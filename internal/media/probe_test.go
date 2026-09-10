package media

import "testing"

const probeJSON = `{
  "streams": [
    {"codec_type":"video","codec_name":"h264","width":1080,"height":1920,
     "r_frame_rate":"30000/1001","nb_frames":"900",
     "side_data_list":[{"rotation":-90}]},
    {"codec_type":"audio","codec_name":"aac"}
  ],
  "format": {"format_name":"mov,mp4","duration":"30.030000","size":"5242880","bit_rate":"1400000"}
}`

func TestParseProbe(t *testing.T) {
	info, err := ParseProbe("/tmp/a.mp4", []byte(probeJSON))
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 1080 || info.Height != 1920 {
		t.Errorf("size = %dx%d", info.Width, info.Height)
	}
	if info.Rotation != 270 {
		t.Errorf("rotation = %d, want 270 (-90 normalised)", info.Rotation)
	}
	w, h := info.DisplaySize()
	if w != 1920 || h != 1080 {
		t.Errorf("display size = %dx%d, want 1920x1080 (rotation applied)", w, h)
	}
	if !info.HasAudio || info.AudioCodec != "aac" {
		t.Errorf("audio = %v %q", info.HasAudio, info.AudioCodec)
	}
	if info.FPS < 29.9 || info.FPS > 30.0 {
		t.Errorf("fps = %v", info.FPS)
	}
	if info.Duration != 30.03 || info.SizeBytes != 5242880 {
		t.Errorf("duration/size = %v %v", info.Duration, info.SizeBytes)
	}
}

func TestParseProbeNoVideo(t *testing.T) {
	_, err := ParseProbe("/tmp/a.m4a", []byte(`{"streams":[{"codec_type":"audio","codec_name":"aac"}],"format":{}}`))
	if err == nil {
		t.Fatal("expected an error for a file with no video stream")
	}
}

func TestParseRate(t *testing.T) {
	if got := parseRate("30000/1001"); got < 29.9 || got > 30 {
		t.Errorf("parseRate rational = %v", got)
	}
	if got := parseRate("25"); got != 25 {
		t.Errorf("parseRate plain = %v", got)
	}
	for _, bad := range []string{"", "0/0", "x/y", "1/0"} {
		if got := parseRate(bad); got != 0 {
			t.Errorf("parseRate(%q) = %v, want 0", bad, got)
		}
	}
}
