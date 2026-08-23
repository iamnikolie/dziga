package media

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Frame is one extracted still.
type Frame struct {
	Index int     `json:"index"`
	T     float64 `json:"t_sec"`
	Path  string  `json:"path"`
}

// ExtractOpts controls frame extraction.
type ExtractOpts struct {
	Dir      string // destination directory (created if missing)
	Prefix   string // filename prefix
	Width    int    // scale width, 0 = source resolution
	PNG      bool   // encode PNG instead of JPEG
	Quality  int    // JPEG -q:v, 2 (best) .. 31; 0 → 2
	Parallel int    // concurrent ffmpeg processes; 0 → 4
}

// ExtractFrames writes one still per requested timestamp. Seeking is done with
// -ss before -i (fast input seek, ffmpeg still decodes to the exact frame), one
// process per timestamp so the grid holds the exact moments asked for.
func ExtractFrames(ctx context.Context, src string, times []float64, opts ExtractOpts) ([]Frame, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("media.ExtractFrames: ffmpeg not found on PATH (brew install ffmpeg)")
	}
	if len(times) == 0 {
		return nil, fmt.Errorf("media.ExtractFrames: no timestamps")
	}
	if err := os.MkdirAll(opts.Dir, 0755); err != nil {
		return nil, fmt.Errorf("media.ExtractFrames: %w", err)
	}
	ext := "jpg"
	if opts.PNG {
		ext = "png"
	}
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "frame"
	}
	par := opts.Parallel
	if par <= 0 {
		par = 4
	}
	q := opts.Quality
	if q <= 0 {
		q = 2
	}

	frames := make([]Frame, len(times))
	errs := make([]error, len(times))
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup

	for i, t := range times {
		wg.Add(1)
		go func(i int, t float64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			out := filepath.Join(opts.Dir, fmt.Sprintf("%s-%03d-%s.%s", prefix, i+1, stampSlug(t), ext))
			args := []string{
				"-nostdin", "-y", "-loglevel", "error",
				"-ss", strconv.FormatFloat(t, 'f', 3, 64),
				"-i", src,
				"-frames:v", "1",
			}
			if opts.Width > 0 {
				args = append(args, "-vf", fmt.Sprintf("scale=%d:-2", opts.Width))
			}
			if !opts.PNG {
				args = append(args, "-q:v", strconv.Itoa(q))
			}
			args = append(args, "-update", "1", out)

			if b, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
				errs[i] = fmt.Errorf("t=%.3f: %w: %s", t, err, strings.TrimSpace(string(b)))
				return
			}
			frames[i] = Frame{Index: i + 1, T: t, Path: out}
		}(i, t)
	}
	wg.Wait()

	for _, e := range errs {
		if e != nil {
			return nil, fmt.Errorf("media.ExtractFrames: %w", e)
		}
	}
	return frames, nil
}

// stampSlug renders a timestamp as a filename-safe token: 12.457 → "0m12s457".
func stampSlug(t float64) string {
	if t < 0 {
		t = 0
	}
	ms := int(t*1000 + 0.5)
	h := ms / 3600000
	m := (ms % 3600000) / 60000
	s := (ms % 60000) / 1000
	milli := ms % 1000
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds%03d", h, m, s, milli)
	}
	return fmt.Sprintf("%dm%02ds%03d", m, s, milli)
}
