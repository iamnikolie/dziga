package media

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTime accepts the time forms an agent is likely to type:
// "12", "12.5", "12s", "2m", "1h", "1m30s", "1:03", "1:03.5", "01:02:03".
// Returns seconds.
func ParseTime(s string) (float64, error) {
	t := strings.TrimSpace(strings.ToLower(s))
	if t == "" {
		return 0, fmt.Errorf("media.ParseTime: empty")
	}

	if strings.Contains(t, ":") {
		parts := strings.Split(t, ":")
		if len(parts) > 3 {
			return 0, fmt.Errorf("media.ParseTime: %q: too many ':' groups", s)
		}
		var total float64
		for _, p := range parts {
			v, err := strconv.ParseFloat(p, 64)
			if err != nil {
				return 0, fmt.Errorf("media.ParseTime: %q: %w", s, err)
			}
			total = total*60 + v
		}
		return total, nil
	}

	// bare number → seconds
	if v, err := strconv.ParseFloat(t, 64); err == nil {
		return v, nil
	}

	// unit-suffixed: 1h2m3.5s (any subset)
	var total float64
	var num strings.Builder
	seen := false
	for _, r := range t {
		switch {
		case (r >= '0' && r <= '9') || r == '.':
			num.WriteRune(r)
		case r == 'h' || r == 'm' || r == 's':
			v, err := strconv.ParseFloat(num.String(), 64)
			if err != nil {
				return 0, fmt.Errorf("media.ParseTime: %q: %w", s, err)
			}
			switch r {
			case 'h':
				total += v * 3600
			case 'm':
				total += v * 60
			case 's':
				total += v
			}
			num.Reset()
			seen = true
		default:
			return 0, fmt.Errorf("media.ParseTime: %q: unexpected %q", s, r)
		}
	}
	if !seen || num.Len() > 0 {
		return 0, fmt.Errorf("media.ParseTime: %q: not a duration", s)
	}
	return total, nil
}

// ParseTimeList splits a comma-separated list of time expressions.
func ParseTimeList(s string) ([]float64, error) {
	var out []float64
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := ParseTime(p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("media.ParseTimeList: no times in %q", s)
	}
	return out, nil
}

// FormatTS renders seconds as MM:SS.s, or H:MM:SS.s past an hour.
func FormatTS(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	h := int(sec) / 3600
	m := (int(sec) % 3600) / 60
	s := sec - float64(h*3600+m*60)
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%04.1f", h, m, s)
	}
	return fmt.Sprintf("%d:%04.1f", m, s)
}

// FormatOffset renders seconds as the "12.5s" form the Gemini API expects.
func FormatOffset(sec float64) string {
	return strconv.FormatFloat(sec, 'f', -1, 64) + "s"
}

// Spread returns n evenly spaced sample points inside [from, to), each at the
// centre of its slice — the same rule gengoya's poster uses.
func Spread(from, to float64, n int) []float64 {
	if n < 1 || to <= from {
		return nil
	}
	span := to - from
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = from + span*(float64(i)+0.5)/float64(n)
	}
	return out
}
