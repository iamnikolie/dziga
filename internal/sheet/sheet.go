package sheet

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"

	_ "image/jpeg"
	_ "image/png"
)

// Cell is one frame placed on the sheet.
type Cell struct {
	Path  string // decoded from disk
	Label string // burned into the top-left corner
}

// Opts controls sheet rendering.
type Opts struct {
	MaxEdge int  // long-edge cap of the finished sheet (0 → 1568)
	Cols    int  // 0 → auto from aspect
	Gutter  int  // pixels between cells (0 → 4)
	PNG     bool // encode PNG instead of JPEG
	Quality int  // JPEG quality (0 → 88)
}

// Result describes the written sheet.
type Result struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	Cells  int    `json:"cells"`
	Bytes  int64  `json:"bytes"`
	Tokens int    `json:"approx_image_tokens"`
}

// Compose tiles the cells into one labelled contact sheet at outPath.
// aspect is the source video's display aspect; the caller has already scaled the
// extracted frames to the layout's cell width.
func Compose(cells []Cell, aspect float64, outPath string, opts Opts) (*Result, error) {
	if len(cells) == 0 {
		return nil, fmt.Errorf("sheet.Compose: no cells")
	}
	gutter := opts.Gutter
	if gutter == 0 {
		gutter = 4
	}
	lay := Solve(len(cells), aspect, opts.Cols, opts.MaxEdge, gutter)

	canvas := image.NewRGBA(image.Rect(0, 0, lay.Width, lay.Height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{18, 18, 20, 255}), image.Point{}, draw.Src)

	scale := LabelScale(lay.CellW)
	for i, c := range cells {
		img, err := decode(c.Path)
		if err != nil {
			return nil, fmt.Errorf("sheet.Compose: %w", err)
		}
		col := i % lay.Cols
		row := i / lay.Cols
		x := gutter + col*(lay.CellW+gutter)
		y := gutter + row*(lay.CellH+gutter)
		cellRect := image.Rect(x, y, x+lay.CellW, y+lay.CellH)

		// centre the frame in its cell; ffmpeg's even-height rounding can be a
		// pixel off, and a mixed-aspect source should not shear the grid.
		b := img.Bounds()
		originX := x + (lay.CellW-b.Dx())/2
		originY := y + (lay.CellH-b.Dy())/2
		dst := image.Rect(originX, originY, originX+b.Dx(), originY+b.Dy()).Intersect(cellRect)
		if dst.Empty() {
			continue
		}
		// draw.Draw's source point is the pixel that lands on dst.Min
		sp := b.Min.Add(dst.Min.Sub(image.Pt(originX, originY)))
		draw.Draw(canvas, dst, img, sp, draw.Src)

		if c.Label != "" {
			DrawLabel(canvas, x+2, y+2, c.Label, scale)
		}
	}

	f, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("sheet.Compose: %w", err)
	}
	defer f.Close()

	if opts.PNG {
		err = png.Encode(f, canvas)
	} else {
		q := opts.Quality
		if q == 0 {
			q = 88
		}
		err = jpeg.Encode(f, canvas, &jpeg.Options{Quality: q})
	}
	if err != nil {
		return nil, fmt.Errorf("sheet.Compose: encode: %w", err)
	}

	st, err := os.Stat(outPath)
	if err != nil {
		return nil, fmt.Errorf("sheet.Compose: %w", err)
	}
	return &Result{
		Path:   outPath,
		Width:  lay.Width,
		Height: lay.Height,
		Cols:   lay.Cols,
		Rows:   lay.Rows,
		Cells:  len(cells),
		Bytes:  st.Size(),
		Tokens: ImageTokens(lay.Width, lay.Height),
	}, nil
}

func decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return img, nil
}
