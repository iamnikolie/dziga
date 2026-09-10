package sheet

import (
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Labels are drawn here rather than by ffmpeg's drawtext on purpose: drawtext
// needs a libfreetype-enabled build, and Homebrew's ffmpeg 8.x ships without it.
// A pure-Go bitmap face scaled by an integer factor is legible and always there.

// LabelScale picks an integer upscale for the 7x13 base face from cell width.
func LabelScale(cellW int) int {
	s := cellW / 160
	if s < 1 {
		s = 1
	}
	if s > 4 {
		s = 4
	}
	return s
}

// DrawLabel composites white text on a translucent black bar at (x, y) of dst.
// Returns the drawn size.
func DrawLabel(dst *image.RGBA, x, y int, text string, scale int) (int, int) {
	if scale < 1 {
		scale = 1
	}
	face := basicfont.Face7x13
	tw := font.MeasureString(face, text).Ceil()
	ascent := face.Ascent
	th := face.Height

	padX, padY := 3, 2
	boxW, boxH := tw+padX*2, th+padY*2

	small := image.NewRGBA(image.Rect(0, 0, boxW, boxH))
	draw.Draw(small, small.Bounds(), image.NewUniform(color.RGBA{0, 0, 0, 205}), image.Point{}, draw.Src)
	d := &font.Drawer{
		Dst:  small,
		Src:  image.NewUniform(color.RGBA{255, 255, 255, 255}),
		Face: face,
		Dot:  fixed.P(padX, padY+ascent),
	}
	d.DrawString(text)

	big := upscale(small, scale)
	r := image.Rect(x, y, x+big.Bounds().Dx(), y+big.Bounds().Dy())
	draw.Draw(dst, r, big, image.Point{}, draw.Over)
	return r.Dx(), r.Dy()
}

// upscale does nearest-neighbour integer magnification (crisp for bitmap fonts).
func upscale(src *image.RGBA, f int) *image.RGBA {
	if f == 1 {
		return src
	}
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx()*f, b.Dy()*f))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := src.RGBAAt(b.Min.X+x, b.Min.Y+y)
			for dy := 0; dy < f; dy++ {
				for dx := 0; dx < f; dx++ {
					out.SetRGBA(x*f+dx, y*f+dy, c)
				}
			}
		}
	}
	return out
}
