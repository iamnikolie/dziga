package media

import (
	"math"
	"testing"
)

func TestParseTime(t *testing.T) {
	cases := map[string]float64{
		"12":       12,
		"12.5":     12.5,
		"12s":      12,
		"2m":       120,
		"1h":       3600,
		"1m30s":    90,
		"1h2m3.5s": 3723.5,
		"1:03":     63,
		"1:03.5":   63.5,
		"01:02:03": 3723,
		"  0:30  ": 30,
	}
	for in, want := range cases {
		got, err := ParseTime(in)
		if err != nil {
			t.Fatalf("ParseTime(%q): %v", in, err)
		}
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("ParseTime(%q) = %v, want %v", in, got, want)
		}
	}
	for _, bad := range []string{"", "abc", "1:2:3:4", "12x", "m"} {
		if _, err := ParseTime(bad); err == nil {
			t.Errorf("ParseTime(%q): expected error", bad)
		}
	}
}

func TestParseTimeList(t *testing.T) {
	got, err := ParseTimeList("3.5, 12 ,1:04")
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{3.5, 12, 64}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-6 {
			t.Errorf("[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if _, err := ParseTimeList(" , "); err == nil {
		t.Error("expected error for empty list")
	}
}

func TestFormatTS(t *testing.T) {
	cases := map[float64]string{
		0:      "0:00.0",
		12.44:  "0:12.4",
		63.5:   "1:03.5",
		3723.5: "1:02:03.5",
	}
	for in, want := range cases {
		if got := FormatTS(in); got != want {
			t.Errorf("FormatTS(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSpreadCentresSamples(t *testing.T) {
	got := Spread(0, 4, 4)
	want := []float64{0.5, 1.5, 2.5, 3.5}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("Spread = %v, want %v", got, want)
		}
	}
	if Spread(0, 0, 3) != nil {
		t.Error("empty window should yield nil")
	}
	if Spread(0, 10, 0) != nil {
		t.Error("n<1 should yield nil")
	}
}
