package cmd

import "testing"

func TestSceneTimesLandJustAfterTheCut(t *testing.T) {
	got := sceneTimes([]float64{3.2, 8.0, 3.25}, 0, 10)
	// the opening frame is always included; near-duplicate cuts collapse
	if len(got) != 3 {
		t.Fatalf("got %v, want [~0, ~3.25, ~8.05]", got)
	}
	if got[0] <= 0 || got[0] > 0.1 {
		t.Errorf("first sample = %v, want just after the start", got[0])
	}
	if got[1] < 3.2 || got[1] > 3.3 {
		t.Errorf("second sample = %v, want just after the 3.2 cut", got[1])
	}
}

func TestSceneTimesIgnoresCutsOutsideTheWindow(t *testing.T) {
	got := sceneTimes([]float64{1, 5, 30}, 4, 10)
	for _, v := range got {
		if v < 4 || v > 10 {
			t.Errorf("sample %v outside window [4,10]", v)
		}
	}
}

func TestSubsampleKeepsEndpoints(t *testing.T) {
	in := []float64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	got := subsample(in, 4)
	if len(got) != 4 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0] != 0 || got[3] != 9 {
		t.Errorf("endpoints = %v %v, want 0 and 9", got[0], got[3])
	}
	if len(subsample(in, 20)) != len(in) {
		t.Error("n above len must return the input untouched")
	}
}

func TestClampAllStaysInsideTheClip(t *testing.T) {
	got := clampAll([]float64{-5, 2, 100}, 10)
	if got[0] != 0 {
		t.Errorf("negative not clamped: %v", got[0])
	}
	if got[2] > 10 {
		t.Errorf("past the end not clamped: %v", got[2])
	}
}

func TestSanitizeStem(t *testing.T) {
	cases := map[string]string{
		"My Clip (final)": "my-clip-final",
		"reel_2026-08-23": "reel-2026-08-23",
		"":                "clip",
		"////":            "clip",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanHelpers(t *testing.T) {
	if got := humanBytes(2 * 1024 * 1024); got != "2.0MB" {
		t.Errorf("humanBytes = %q", got)
	}
	if got := humanTokens(1600); got != "1.6k" {
		t.Errorf("humanTokens = %q", got)
	}
	if got := humanTokens(940); got != "940" {
		t.Errorf("humanTokens = %q", got)
	}
}
