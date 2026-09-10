package media

import "testing"

const showinfo = `
[Parsed_showinfo_2 @ 0x600] n:0 pts:0 pts_time:0 duration:1 fmt:yuv420p
[Parsed_showinfo_2 @ 0x600] n:1 pts:76800 pts_time:3.2 duration:1 fmt:yuv420p
[Parsed_showinfo_2 @ 0x600] n:2 pts:194000 pts_time:8.083 duration:1 fmt:yuv420p
[Parsed_showinfo_2 @ 0x600] n:2 pts:194000 pts_time:8.083 duration:1 fmt:yuv420p
`

func TestParseSceneOutput(t *testing.T) {
	got := ParseSceneOutput(showinfo)
	want := []float64{0, 3.2, 8.083}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v (duplicates must collapse)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if len(ParseSceneOutput("no timestamps here")) != 0 {
		t.Error("expected no cuts")
	}
}
