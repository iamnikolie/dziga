package cmd

import (
	"testing"

	"github.com/iamnikolie/dziga/internal/media"
)

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
	info := &media.Info{Duration: 10, FPS: 25}
	got := clampAll([]float64{-5, 2, 100}, info)
	if got[0] != 0 {
		t.Errorf("negative not clamped: %v", got[0])
	}
	if got[2] > 10 {
		t.Errorf("past the end not clamped: %v", got[2])
	}
}

// ffmpeg matches the first frame at or after the seek target, so the real ceiling is
// the last frame's timestamp. On a 2s clip at 6fps that is 1.833: ask for 1.917 and
// ffmpeg exits 0 having written no file at all.
func TestClampAllStopsAtTheLastFrameNotTheDuration(t *testing.T) {
	info := &media.Info{Duration: 2, FPS: 6}
	got := clampAll([]float64{1.9167}, info)
	if got[0] > 1.8334 {
		t.Errorf("clamped to %v, want the last frame at 1.833", got[0])
	}
	if got[0] < 1.8 {
		t.Errorf("clamped to %v — that discards most of the final frame", got[0])
	}
}

func TestLastFrameTimeHandlesUnknownRate(t *testing.T) {
	if got := lastFrameTime(&media.Info{Duration: 5}); got <= 4.5 || got >= 5 {
		t.Errorf("no fps: got %v, want just under the duration", got)
	}
	if got := lastFrameTime(&media.Info{}); got != 0 {
		t.Errorf("unknown duration: got %v, want 0", got)
	}
	if got := lastFrameTime(nil); got != 0 {
		t.Errorf("nil info: got %v", got)
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
