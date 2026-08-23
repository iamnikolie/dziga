package media

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
)

var ptsRE = regexp.MustCompile(`pts_time:([0-9]+\.?[0-9]*)`)

// DetectScenes returns the timestamps of detected cuts. The frames are scaled
// down to `scaleW` before the detector runs — a cut is a global change, so full
// resolution buys nothing and costs a lot of decode time.
func DetectScenes(ctx context.Context, path string, threshold float64, scaleW int) ([]float64, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("media.DetectScenes: ffmpeg not found on PATH (brew install ffmpeg)")
	}
	if scaleW <= 0 {
		scaleW = 320
	}
	filter := fmt.Sprintf("scale=%d:-2,select='gt(scene,%s)',showinfo",
		scaleW, strconv.FormatFloat(threshold, 'f', -1, 64))

	cmd := exec.CommandContext(ctx, bin,
		"-nostdin", "-hide_banner", "-loglevel", "info",
		"-i", path,
		"-an", "-sn",
		"-filter:v", filter,
		"-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("media.DetectScenes: %w: %s", err, tail(stderr.String(), 400))
	}
	return ParseSceneOutput(stderr.String()), nil
}

// ParseSceneOutput pulls pts_time values out of ffmpeg's showinfo lines (pure).
func ParseSceneOutput(s string) []float64 {
	m := ptsRE.FindAllStringSubmatch(s, -1)
	out := make([]float64, 0, len(m))
	seen := map[float64]bool{}
	for _, g := range m {
		v, err := strconv.ParseFloat(g[1], 64)
		if err != nil || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Float64s(out)
	return out
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
