package sheet

import "math"

// Layout is a solved contact-sheet geometry.
type Layout struct {
	Cols, Rows    int
	CellW, CellH  int
	Gutter        int
	Width, Height int
}

// DefaultMaxEdge is where Claude stops downscaling an image (long edge, px).
const DefaultMaxEdge = 1568

// DefaultMaxArea is the other half of the same rule: past ~1.15 megapixels the
// image is scaled down before the model ever sees it, so pixels beyond this are
// paid for in bytes and thrown away. Both caps must hold.
const DefaultMaxArea = 1_150_000

// ChooseCols picks a column count for n cells of the given aspect.
// With the area capped, the sheet costs the same whatever the grid, so the
// dominant term is wasted slots — an empty cell in the last row spends real
// tokens on nothing. Squareness only breaks ties.
func ChooseCols(n int, aspect float64) int {
	if n <= 1 {
		return 1
	}
	if aspect <= 0 {
		aspect = 16.0 / 9.0
	}
	best, bestScore := 1, math.Inf(1)
	for cols := 1; cols <= n; cols++ {
		rows := int(math.Ceil(float64(n) / float64(cols)))
		waste := float64(cols*rows-n) / float64(cols*rows)
		sheetAspect := float64(cols) * aspect / float64(rows)
		score := 3.0*waste + 0.5*math.Abs(math.Log(sheetAspect))
		if score < bestScore-1e-9 {
			best, bestScore = cols, score
		}
	}
	return best
}

// Solve lays out n cells of the given aspect so the finished sheet honours both
// the long-edge cap and the area cap.
func Solve(n int, aspect float64, cols, maxEdge, gutter int) Layout {
	return SolveArea(n, aspect, cols, maxEdge, DefaultMaxArea, gutter)
}

// SolveArea is Solve with an explicit pixel budget.
func SolveArea(n int, aspect float64, cols, maxEdge, maxArea, gutter int) Layout {
	if n < 1 {
		n = 1
	}
	if aspect <= 0 {
		aspect = 16.0 / 9.0
	}
	if cols <= 0 {
		cols = ChooseCols(n, aspect)
	}
	if cols > n {
		cols = n
	}
	if maxEdge <= 0 {
		maxEdge = DefaultMaxEdge
	}
	if maxArea <= 0 {
		maxArea = DefaultMaxArea
	}
	if gutter < 0 {
		gutter = 0
	}
	rows := int(math.Ceil(float64(n) / float64(cols)))

	// width  = cols*cell + (cols+1)*gutter
	// height = rows*cell/aspect + (rows+1)*gutter
	gw := float64(cols+1) * float64(gutter)
	gh := float64(rows+1) * float64(gutter)
	byW := (float64(maxEdge) - gw) / float64(cols)
	byH := ((float64(maxEdge) - gh) / float64(rows)) * aspect
	// area: (cols*c + gw) * (rows*c/aspect + gh) <= maxArea → solve the quadratic
	byArea := solveAreaCell(float64(cols), float64(rows), aspect, gw, gh, float64(maxArea))

	cell := math.Min(math.Min(byW, byH), byArea)
	if cell < 32 {
		cell = 32
	}

	cellW := int(cell)
	if cellW%2 == 1 {
		cellW-- // ffmpeg's scale=W:-2 wants an even width
	}
	cellH := int(math.Round(float64(cellW) / aspect))
	if cellH < 1 {
		cellH = 1
	}

	return Layout{
		Cols:   cols,
		Rows:   rows,
		CellW:  cellW,
		CellH:  cellH,
		Gutter: gutter,
		Width:  cols*cellW + (cols+1)*gutter,
		Height: rows*cellH + (rows+1)*gutter,
	}
}

// solveAreaCell returns the largest cell width whose sheet stays within maxArea.
// (cols*c + gw)(rows*c/a + gh) = maxArea → A·c² + B·c − maxArea = 0.
func solveAreaCell(cols, rows, aspect, gw, gh, maxArea float64) float64 {
	A := cols * rows / aspect
	B := cols*gh + gw*rows/aspect
	C := gw*gh - maxArea
	if A <= 0 {
		return math.Inf(1)
	}
	disc := B*B - 4*A*C
	if disc <= 0 {
		return 32
	}
	return (-B + math.Sqrt(disc)) / (2 * A)
}

// ImageTokens estimates what an image of this size costs a Claude context
// (~w*h/750). Printed on stderr so the agent can choose overview vs close-up.
func ImageTokens(w, h int) int {
	return w * h / 750
}
