package cmd

import (
	"context"
	"fmt"

	"github.com/langgerone/dziga/internal/media"
)

// sampleOpts is the shared "which moments do we look at" surface of sheet/frames.
type sampleOpts struct {
	n         int
	from, to  string
	at        string
	every     string
	useScenes bool
	threshold float64
}

// resolveTimes turns the flags into a concrete, sorted list of timestamps and a
// one-word description of how they were chosen (for the status line).
func resolveTimes(ctx context.Context, src *Source, info *media.Info, o sampleOpts) ([]float64, string, error) {
	from, to := 0.0, info.Duration
	if o.from != "" {
		v, err := media.ParseTime(o.from)
		if err != nil {
			return nil, "", err
		}
		from = v
	}
	if o.to != "" {
		v, err := media.ParseTime(o.to)
		if err != nil {
			return nil, "", err
		}
		to = v
	}
	if to <= 0 || to > info.Duration {
		to = info.Duration
	}
	if to <= from {
		return nil, "", fmt.Errorf("empty window: from=%.3f to=%.3f", from, to)
	}

	switch {
	case o.at != "":
		times, err := media.ParseTimeList(o.at)
		if err != nil {
			return nil, "", err
		}
		return clampAll(times, info), "at", nil

	case o.every != "":
		step, err := media.ParseTime(o.every)
		if err != nil {
			return nil, "", err
		}
		if step <= 0 {
			return nil, "", fmt.Errorf("--every must be > 0")
		}
		var times []float64
		for t := from; t < to; t += step {
			times = append(times, t)
		}
		if len(times) == 0 {
			times = []float64{from}
		}
		return clampAll(times, info), "every", nil

	case o.useScenes:
		cuts, err := media.DetectScenes(ctx, src.Path, o.threshold, 320)
		if err != nil {
			return nil, "", err
		}
		times := sceneTimes(cuts, from, to)
		if len(times) == 0 {
			// a single-shot clip is a legitimate answer, not a failure
			statusf("scenes: no cuts above threshold %.2f — falling back to even sampling", o.threshold)
			return clampAll(media.Spread(from, to, o.n), info), "even", nil
		}
		if o.n > 0 && len(times) > o.n {
			times = subsample(times, o.n)
		}
		return clampAll(times, info), "scenes", nil

	default:
		if o.n < 1 {
			return nil, "", fmt.Errorf("--n must be >= 1")
		}
		return clampAll(media.Spread(from, to, o.n), info), "even", nil
	}
}

// sceneTimes places a sample just *after* each cut (the first frame of the new
// shot) and always includes the opening frame.
func sceneTimes(cuts []float64, from, to float64) []float64 {
	const eps = 0.05
	out := []float64{}
	if from < to {
		out = append(out, from+eps)
	}
	for _, c := range cuts {
		t := c + eps
		if t <= from || t >= to {
			continue
		}
		if len(out) > 0 && t-out[len(out)-1] < 0.2 {
			continue
		}
		out = append(out, t)
	}
	return out
}

// subsample keeps n evenly spread entries of a longer list, endpoints included.
func subsample(in []float64, n int) []float64 {
	if n >= len(in) || n < 1 {
		return in
	}
	if n == 1 {
		return in[:1]
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		idx := int(float64(i) * float64(len(in)-1) / float64(n-1))
		out[i] = in[idx]
	}
	return out
}

// clampAll keeps sample points inside the clip. The ceiling is the *last frame's*
// presentation time, not the duration: ffmpeg seeks to the first frame at or after
// the target, so on a 2s clip at 6fps anything past 1.833 matches no frame at all
// and ffmpeg exits 0 having written nothing.
func clampAll(times []float64, info *media.Info) []float64 {
	last := lastFrameTime(info)
	out := make([]float64, 0, len(times))
	for _, t := range times {
		if t < 0 {
			t = 0
		}
		if last > 0 && t > last {
			t = last
		}
		out = append(out, t)
	}
	return out
}

// lastFrameTime is the presentation time of the final frame, or 0 when unknown.
func lastFrameTime(info *media.Info) float64 {
	if info == nil || info.Duration <= 0 {
		return 0
	}
	step := 0.05
	if info.FPS > 0 {
		step = 1 / info.FPS
	}
	last := info.Duration - step
	if last < 0 {
		return 0
	}
	return last
}
