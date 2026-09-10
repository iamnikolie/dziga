package provider

import (
	"encoding/json"
	"testing"
)

// These pin the request shape verified live against v1beta on 2026-08-23:
// snake_case file_data keys, offsets as "<n>s" strings, fps as a number.
func TestBuildAskRequestShape(t *testing.T) {
	from, to := 2.0, 6.5
	body, err := BuildAskRequest(AskRequest{
		FileURI:     "https://generativelanguage.googleapis.com/v1beta/files/abc",
		Prompt:      "what happens?",
		StartOffset: &from,
		EndOffset:   &to,
		FPS:         2,
		MediaRes:    "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}

	parts := got["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	if len(parts) != 2 {
		t.Fatalf("want file part + text part, got %d", len(parts))
	}
	fp := parts[0].(map[string]any)
	fd := fp["file_data"].(map[string]any)
	if fd["mime_type"] != "video/mp4" {
		t.Errorf("mime_type = %v (default must be video/mp4)", fd["mime_type"])
	}
	if fd["file_uri"] == nil {
		t.Error("file_uri missing")
	}
	vm := fp["video_metadata"].(map[string]any)
	if vm["start_offset"] != "2s" || vm["end_offset"] != "6.5s" {
		t.Errorf("offsets = %v / %v, want the \"<n>s\" string form", vm["start_offset"], vm["end_offset"])
	}
	if _, ok := vm["fps"].(float64); !ok {
		t.Errorf("fps must be a number, got %T", vm["fps"])
	}
	if parts[1].(map[string]any)["text"] != "what happens?" {
		t.Error("prompt part missing")
	}
	gen := got["generationConfig"].(map[string]any)
	if gen["mediaResolution"] != "MEDIA_RESOLUTION_HIGH" {
		t.Errorf("mediaResolution = %v", gen["mediaResolution"])
	}
	if _, ok := gen["responseMimeType"]; ok {
		t.Error("no schema and no --json: response mime must stay unset")
	}
}

func TestBuildAskRequestOmitsEmptyVideoMetadata(t *testing.T) {
	body, _ := BuildAskRequest(AskRequest{FileURI: "u", Prompt: "p"})
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	fp := got["contents"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)
	if _, ok := fp["video_metadata"]; ok {
		t.Error("video_metadata must be omitted when no sampling controls are set")
	}
}

func TestBuildAskRequestSchemaForcesJSON(t *testing.T) {
	body, err := BuildAskRequest(AskRequest{
		FileURI: "u", Prompt: "p",
		Schema: json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(body, &got)
	gen := got["generationConfig"].(map[string]any)
	if gen["responseMimeType"] != "application/json" {
		t.Errorf("responseMimeType = %v", gen["responseMimeType"])
	}
	if gen["responseSchema"] == nil {
		t.Error("responseSchema missing")
	}
}

func TestBuildAskRequestValidates(t *testing.T) {
	if _, err := BuildAskRequest(AskRequest{Prompt: "p"}); err == nil {
		t.Error("expected error without file_uri")
	}
	if _, err := BuildAskRequest(AskRequest{FileURI: "u"}); err == nil {
		t.Error("expected error without prompt")
	}
	if _, err := BuildAskRequest(AskRequest{FileURI: "u", Prompt: "p", Schema: json.RawMessage(`{`)}); err == nil {
		t.Error("expected error on malformed schema")
	}
}

const askResponse = `{
 "candidates":[{"finishReason":"STOP","content":{"parts":[
   {"text":"internal reasoning","thought":true},
   {"text":"A red boat ","thoughtSignature":"xx"},
   {"text":"drifts across a puddle."}
 ]}}],
 "usageMetadata":{"promptTokenCount":376,"candidatesTokenCount":77,"thoughtsTokenCount":104,
   "totalTokenCount":557,
   "promptTokensDetails":[{"modality":"TEXT","tokenCount":12},{"modality":"VIDEO","tokenCount":364}]},
 "modelVersion":"gemini-3.7-flash"
}`

func TestParseAskResponse(t *testing.T) {
	res, err := ParseAskResponse([]byte(askResponse))
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "A red boat drifts across a puddle." {
		t.Errorf("text = %q (thought parts must be dropped, the rest joined)", res.Text)
	}
	if res.Usage.Video != 364 || res.Usage.Text != 12 {
		t.Errorf("modality split = %+v", res.Usage)
	}
	if res.Usage.Output != 77 || res.Usage.Thoughts != 104 || res.Usage.Prompt != 376 {
		t.Errorf("usage = %+v", res.Usage)
	}
	if res.ModelVersion != "gemini-3.7-flash" {
		t.Errorf("modelVersion = %q", res.ModelVersion)
	}
}

func TestParseAskResponseErrors(t *testing.T) {
	if _, err := ParseAskResponse([]byte(`{"promptFeedback":{"blockReason":"SAFETY"}}`)); err == nil {
		t.Error("expected a blocked error")
	}
	if _, err := ParseAskResponse([]byte(`{"candidates":[]}`)); err == nil {
		t.Error("expected an error with no candidates")
	}
	if _, err := ParseAskResponse([]byte(`{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[]}}]}`)); err == nil {
		t.Error("expected an error when the answer is empty and generation was cut short")
	}
	if _, err := ParseAskResponse([]byte(`not json`)); err == nil {
		t.Error("expected a parse error")
	}
}

func TestMediaResolution(t *testing.T) {
	cases := map[string]string{
		"low": "MEDIA_RESOLUTION_LOW", "medium": "MEDIA_RESOLUTION_MEDIUM",
		"high": "MEDIA_RESOLUTION_HIGH", "MEDIA_RESOLUTION_LOW": "MEDIA_RESOLUTION_LOW",
	}
	for in, want := range cases {
		if got := mediaResolution(in); got != want {
			t.Errorf("mediaResolution(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatOffsetIsSecondsString(t *testing.T) {
	if got := formatOffset(6.5); got != "6.5s" {
		t.Errorf("formatOffset = %q", got)
	}
	if got := formatOffset(2); got != "2s" {
		t.Errorf("formatOffset = %q", got)
	}
}
