package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/iamnikolie/dziga/internal/client"
)

const geminiBase = "https://generativelanguage.googleapis.com/v1beta"
const geminiUploadBase = "https://generativelanguage.googleapis.com/upload/v1beta/files"

// Gemini is the only engine that reads video natively today.
type Gemini struct {
	APIKey string
	HTTP   *client.Client
}

// NewGemini constructs the provider.
func NewGemini(key string, httpClient *client.Client) *Gemini {
	if httpClient == nil {
		httpClient = client.New()
	}
	return &Gemini{APIKey: key, HTTP: httpClient}
}

func (g *Gemini) headers(extra map[string]string) map[string]string {
	h := map[string]string{"x-goog-api-key": g.APIKey}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// File mirrors the Files API resource.
type File struct {
	Name           string    `json:"name"`
	URI            string    `json:"uri"`
	MimeType       string    `json:"mimeType"`
	SizeBytes      string    `json:"sizeBytes"`
	State          string    `json:"state"`
	ExpirationTime time.Time `json:"expirationTime"`
}

type fileEnvelope struct {
	File File `json:"file"`
}

// Upload performs the three-step resumable upload the Files API requires.
// (A plain POST of the bytes is not accepted; the start call replies with the
// real upload URL in the X-Goog-Upload-URL header.)
func (g *Gemini) Upload(ctx context.Context, path, mime, displayName string) (*File, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("provider.Upload: %w", err)
	}
	size := st.Size()

	start, err := json.Marshal(map[string]any{"file": map[string]any{"display_name": displayName}})
	if err != nil {
		return nil, fmt.Errorf("provider.Upload: %w", err)
	}
	resp, err := g.HTTP.Do(ctx, "POST", geminiUploadBase, start, g.headers(map[string]string{
		"X-Goog-Upload-Protocol":              "resumable",
		"X-Goog-Upload-Command":               "start",
		"X-Goog-Upload-Header-Content-Length": strconv.FormatInt(size, 10),
		"X-Goog-Upload-Header-Content-Type":   mime,
		"Content-Type":                        "application/json",
	}))
	if err != nil {
		return nil, fmt.Errorf("provider.Upload: start: %w", err)
	}
	uploadURL := resp.Header.Get("X-Goog-Upload-URL")
	if uploadURL == "" {
		return nil, fmt.Errorf("provider.Upload: start: no X-Goog-Upload-URL in response")
	}

	body := func() (io.ReadCloser, int64, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		return f, size, nil
	}
	resp, err = g.HTTP.DoBody(ctx, "POST", uploadURL, body, g.headers(map[string]string{
		"X-Goog-Upload-Offset":  "0",
		"X-Goog-Upload-Command": "upload, finalize",
	}))
	if err != nil {
		return nil, fmt.Errorf("provider.Upload: finalize: %w", err)
	}

	var env fileEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("provider.Upload: parse: %w", err)
	}
	if env.File.Name == "" {
		return nil, fmt.Errorf("provider.Upload: empty file resource in response")
	}
	return &env.File, nil
}

// GetFile fetches a file resource ("files/abc123").
func (g *Gemini) GetFile(ctx context.Context, name string) (*File, error) {
	resp, err := g.HTTP.Do(ctx, "GET", geminiBase+"/"+name, nil, g.headers(nil))
	if err != nil {
		return nil, fmt.Errorf("provider.GetFile: %w", err)
	}
	var f File
	if err := json.Unmarshal(resp.Body, &f); err != nil {
		return nil, fmt.Errorf("provider.GetFile: parse: %w", err)
	}
	return &f, nil
}

// WaitActive polls until the uploaded file leaves PROCESSING.
func (g *Gemini) WaitActive(ctx context.Context, f *File, poll time.Duration) (*File, error) {
	if poll <= 0 {
		poll = 2 * time.Second
	}
	cur := f
	for cur.State == "PROCESSING" || cur.State == "" {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("provider.WaitActive: %w", ctx.Err())
		case <-time.After(poll):
		}
		var err error
		cur, err = g.GetFile(ctx, f.Name)
		if err != nil {
			return nil, err
		}
	}
	if cur.State != "ACTIVE" {
		return nil, fmt.Errorf("provider.WaitActive: file %s ended in state %s", cur.Name, cur.State)
	}
	return cur, nil
}

// AskRequest is one question about one uploaded clip.
type AskRequest struct {
	FileURI     string
	MimeType    string
	Prompt      string
	StartOffset *float64
	EndOffset   *float64
	FPS         float64
	MediaRes    string          // low|medium|high (empty = model default)
	Schema      json.RawMessage // when set, forces JSON output
	JSONOut     bool
	Temperature *float64
	MaxTokens   int
}

// BuildAskRequest builds the generateContent body (pure, unit-tested).
func BuildAskRequest(r AskRequest) ([]byte, error) {
	if r.FileURI == "" {
		return nil, fmt.Errorf("provider.BuildAskRequest: file_uri required")
	}
	if r.Prompt == "" {
		return nil, fmt.Errorf("provider.BuildAskRequest: prompt required")
	}
	mime := r.MimeType
	if mime == "" {
		mime = "video/mp4"
	}

	filePart := map[string]any{
		"file_data": map[string]any{"mime_type": mime, "file_uri": r.FileURI},
	}
	vm := map[string]any{}
	if r.StartOffset != nil {
		vm["start_offset"] = formatOffset(*r.StartOffset)
	}
	if r.EndOffset != nil {
		vm["end_offset"] = formatOffset(*r.EndOffset)
	}
	if r.FPS > 0 {
		vm["fps"] = r.FPS
	}
	if len(vm) > 0 {
		filePart["video_metadata"] = vm
	}

	body := map[string]any{
		"contents": []any{map[string]any{
			"parts": []any{filePart, map[string]any{"text": r.Prompt}},
		}},
	}

	gen := map[string]any{}
	if r.Schema != nil {
		var schema any
		if err := json.Unmarshal(r.Schema, &schema); err != nil {
			return nil, fmt.Errorf("provider.BuildAskRequest: schema: %w", err)
		}
		gen["responseMimeType"] = "application/json"
		gen["responseSchema"] = schema
	} else if r.JSONOut {
		gen["responseMimeType"] = "application/json"
	}
	if r.MediaRes != "" {
		gen["mediaResolution"] = mediaResolution(r.MediaRes)
	}
	if r.Temperature != nil {
		gen["temperature"] = *r.Temperature
	}
	if r.MaxTokens > 0 {
		gen["maxOutputTokens"] = r.MaxTokens
	}
	if len(gen) > 0 {
		body["generationConfig"] = gen
	}
	return json.Marshal(body)
}

func mediaResolution(s string) string {
	switch s {
	case "low", "LOW":
		return "MEDIA_RESOLUTION_LOW"
	case "medium", "MEDIUM":
		return "MEDIA_RESOLUTION_MEDIUM"
	case "high", "HIGH":
		return "MEDIA_RESOLUTION_HIGH"
	}
	return s
}

func formatOffset(sec float64) string {
	return strconv.FormatFloat(sec, 'f', -1, 64) + "s"
}

// Usage is the token accounting Gemini reports back.
type Usage struct {
	Prompt   int `json:"prompt_tokens"`
	Video    int `json:"video_tokens"`
	Text     int `json:"text_tokens"`
	Output   int `json:"output_tokens"`
	Thoughts int `json:"thought_tokens"`
	Total    int `json:"total_tokens"`
}

type geminiResponse struct {
	Candidates []struct {
		FinishReason string `json:"finishReason"`
		Content      struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
		PromptTokensDetails  []struct {
			Modality   string `json:"modality"`
			TokenCount int    `json:"tokenCount"`
		} `json:"promptTokensDetails"`
	} `json:"usageMetadata"`
	ModelVersion string `json:"modelVersion"`
}

// AskResult is a parsed answer.
type AskResult struct {
	Text         string
	Usage        Usage
	FinishReason string
	ModelVersion string
}

// ParseAskResponse extracts the answer text and usage (pure, unit-tested).
// Gemini 3.x returns reasoning parts alongside the answer — parts flagged
// `thought` are dropped, everything else is concatenated.
func ParseAskResponse(b []byte) (*AskResult, error) {
	var r geminiResponse
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("provider.ParseAskResponse: %w", err)
	}
	if r.PromptFeedback.BlockReason != "" {
		return nil, fmt.Errorf("provider.ParseAskResponse: blocked: %s", r.PromptFeedback.BlockReason)
	}
	if len(r.Candidates) == 0 {
		return nil, fmt.Errorf("provider.ParseAskResponse: no candidates")
	}
	c := r.Candidates[0]

	text := ""
	for _, p := range c.Content.Parts {
		if p.Thought || p.Text == "" {
			continue
		}
		text += p.Text
	}

	u := Usage{
		Prompt:   r.UsageMetadata.PromptTokenCount,
		Output:   r.UsageMetadata.CandidatesTokenCount,
		Thoughts: r.UsageMetadata.ThoughtsTokenCount,
		Total:    r.UsageMetadata.TotalTokenCount,
	}
	for _, d := range r.UsageMetadata.PromptTokensDetails {
		switch d.Modality {
		case "VIDEO":
			u.Video = d.TokenCount
		case "TEXT":
			u.Text = d.TokenCount
		}
	}

	if text == "" && c.FinishReason != "" && c.FinishReason != "STOP" {
		return nil, fmt.Errorf("provider.ParseAskResponse: empty answer (finishReason=%s)", c.FinishReason)
	}
	return &AskResult{Text: text, Usage: u, FinishReason: c.FinishReason, ModelVersion: r.ModelVersion}, nil
}

// Ask runs generateContent against an already-uploaded file.
func (g *Gemini) Ask(ctx context.Context, modelID string, r AskRequest) (*AskResult, error) {
	body, err := BuildAskRequest(r)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/models/%s:generateContent", geminiBase, modelID)
	resp, err := g.HTTP.Do(ctx, "POST", url, body, g.headers(map[string]string{"Content-Type": "application/json"}))
	if err != nil {
		return nil, fmt.Errorf("provider.Ask: %w", err)
	}
	return ParseAskResponse(resp.Body)
}
