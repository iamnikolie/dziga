package sheet

import "testing"

func TestChooseColsPrefersFullGrids(t *testing.T) {
	cases := []struct {
		n      int
		aspect float64
		want   int
	}{
		{16, 16.0 / 9.0, 4}, // landscape 4x4, no empty slots
		{16, 9.0 / 16.0, 4}, // vertical reel also 4x4
		{9, 16.0 / 9.0, 3},  // 3x3 beats a squarer 2x5 that wastes a slot
		{6, 16.0 / 9.0, 2},  // 2x3
		{1, 16.0 / 9.0, 1},
	}
	for _, c := range cases {
		if got := ChooseCols(c.n, c.aspect); got != c.want {
			t.Errorf("ChooseCols(%d, %.2f) = %d, want %d", c.n, c.aspect, got, c.want)
		}
	}
}

func TestSolveRespectsBothCaps(t *testing.T) {
	for _, aspect := range []float64{16.0 / 9.0, 9.0 / 16.0, 1.0} {
		for _, n := range []int{1, 4, 9, 16, 25} {
			lay := Solve(n, aspect, 0, DefaultMaxEdge, 4)
			long := lay.Width
			if lay.Height > long {
				long = lay.Height
			}
			if long > DefaultMaxEdge {
				t.Errorf("n=%d aspect=%.2f: long edge %d > %d", n, aspect, long, DefaultMaxEdge)
			}
			if area := lay.Width * lay.Height; area > DefaultMaxArea+50_000 {
				t.Errorf("n=%d aspect=%.2f: area %d over budget", n, aspect, area)
			}
			if lay.Cols*lay.Rows < n {
				t.Errorf("n=%d: grid %dx%d cannot hold all cells", n, lay.Cols, lay.Rows)
			}
			if lay.CellW%2 != 0 {
				t.Errorf("n=%d: cell width %d must be even for ffmpeg scale=W:-2", n, lay.CellW)
			}
		}
	}
}

func TestSolveHonoursForcedCols(t *testing.T) {
	lay := Solve(9, 1.0, 3, DefaultMaxEdge, 4)
	if lay.Cols != 3 || lay.Rows != 3 {
		t.Fatalf("grid = %dx%d, want 3x3", lay.Cols, lay.Rows)
	}
}

func TestImageTokens(t *testing.T) {
	if got := ImageTokens(1500, 1000); got != 2000 {
		t.Errorf("ImageTokens = %d, want 2000", got)
	}
}
