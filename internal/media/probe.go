package media

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

// Info is what an agent needs to know about a clip before looking at it.
type Info struct {
	Path       string  `json:"path"`
	Container  string  `json:"container,omitempty"`
	Duration   float64 `json:"duration_sec"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Rotation   int     `json:"rotation,omitempty"`
	FPS        float64 `json:"fps"`
	Frames     int     `json:"frames,omitempty"`
	VideoCodec string  `json:"video_codec,omitempty"`
	AudioCodec string  `json:"audio_codec,omitempty"`
	HasAudio   bool    `json:"has_audio"`
	Bitrate    int     `json:"bitrate_bps,omitempty"`
	SizeBytes  int64   `json:"size_bytes,omitempty"`
}

// Aspect is the displayed width/height ratio (rotation applied).
func (i *Info) Aspect() float64 {
	w, h := i.DisplaySize()
	if h == 0 {
		return 16.0 / 9.0
	}
	return float64(w) / float64(h)
}

// DisplaySize returns width/height after applying container rotation.
func (i *Info) DisplaySize() (int, int) {
	if i.Rotation == 90 || i.Rotation == 270 || i.Rotation == -90 {
		return i.Height, i.Width
	}
	return i.Width, i.Height
}

type ffprobeOut struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		RFrameRate   string `json:"r_frame_rate"`
		AvgFrameRate string `json:"avg_frame_rate"`
		NbFrames     string `json:"nb_frames"`
		Duration     string `json:"duration"`
		Tags         struct {
			Rotate string `json:"rotate"`
		} `json:"tags"`
		SideDataList []struct {
			Rotation float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
}

// Probe runs ffprobe and parses the parts that matter.
func Probe(ctx context.Context, path string) (*Info, error) {
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("media.Probe: ffprobe not found on PATH (brew install ffmpeg)")
	}
	out, err := exec.CommandContext(ctx, bin,
		"-v", "error",
		"-show_format", "-show_streams",
		"-of", "json", path).Output()
	if err != nil {
		return nil, fmt.Errorf("media.Probe: ffprobe %s: %w", path, err)
	}
	return ParseProbe(path, out)
}

// ParseProbe turns ffprobe JSON into Info (pure, testable).
func ParseProbe(path string, data []byte) (*Info, error) {
	var p ffprobeOut
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("media.ParseProbe: %w", err)
	}
	info := &Info{Path: path, Container: p.Format.FormatName}
	info.Duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
	info.SizeBytes, _ = strconv.ParseInt(p.Format.Size, 10, 64)
	if br, err := strconv.Atoi(p.Format.BitRate); err == nil {
		info.Bitrate = br
	}

	haveVideo := false
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			if haveVideo {
				continue
			}
			haveVideo = true
			info.VideoCodec = s.CodecName
			info.Width, info.Height = s.Width, s.Height
			info.FPS = parseRate(s.RFrameRate)
			if info.FPS == 0 {
				info.FPS = parseRate(s.AvgFrameRate)
			}
			if n, err := strconv.Atoi(s.NbFrames); err == nil {
				info.Frames = n
			}
			if info.Duration == 0 {
				info.Duration, _ = strconv.ParseFloat(s.Duration, 64)
			}
			if r, err := strconv.Atoi(strings.TrimSpace(s.Tags.Rotate)); err == nil {
				info.Rotation = normalizeRotation(r)
			}
			for _, sd := range s.SideDataList {
				if sd.Rotation != 0 {
					info.Rotation = normalizeRotation(int(math.Round(sd.Rotation)))
				}
			}
		case "audio":
			info.HasAudio = true
			if info.AudioCodec == "" {
				info.AudioCodec = s.CodecName
			}
		}
	}
	if !haveVideo {
		return nil, fmt.Errorf("media.ParseProbe: %s has no video stream", path)
	}
	if info.Frames == 0 && info.FPS > 0 && info.Duration > 0 {
		info.Frames = int(info.Duration * info.FPS)
	}
	return info, nil
}

func normalizeRotation(r int) int {
	r %= 360
	if r < 0 {
		r += 360
	}
	return r
}

// parseRate parses ffprobe's "30000/1001" rational frame rates.
func parseRate(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0/0" {
		return 0
	}
	num, den, ok := strings.Cut(s, "/")
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	if !ok {
		return n
	}
	d, err := strconv.ParseFloat(den, 64)
	if err != nil || d == 0 {
		return 0
	}
	return n / d
}
