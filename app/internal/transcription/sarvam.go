// Package transcription implements Sarvam's asynchronous batch protocol. No other paid provider is supported.
package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Sarvam struct {
	BaseURL, Key, Model string
	Client              *http.Client
}

func New(base, key, model string) *Sarvam {
	return &Sarvam{base, strings.TrimSpace(key), model, &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (s *Sarvam) call(ctx context.Context, method, path string, input any) (map[string]any, error) {
	if s.Key == "" || strings.EqualFold(s.Key, "dummy") {
		return nil, fmt.Errorf("Sarvam credential is not configured")
	}
	var body io.Reader
	if input != nil {
		data, e := json.Marshal(input)
		if e != nil {
			return nil, e
		}
		body = bytes.NewReader(data)
	}
	r, e := http.NewRequestWithContext(ctx, method, s.BaseURL+"/speech-to-text/job/v1"+path, body)
	if e != nil {
		return nil, e
	}
	r.Header.Set("api-subscription-key", s.Key)
	r.Header.Set("Content-Type", "application/json")
	resp, e := s.Client.Do(r)
	if e != nil {
		return nil, fmt.Errorf("Sarvam request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Sarvam HTTP %d", resp.StatusCode)
	}
	var result map[string]any
	e = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result)
	return result, e
}
func (s *Sarvam) Create(ctx context.Context, language string) (string, error) {
	// Use the reading passage's language, never automatic language detection.
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "english", "en", "en-in":
		language = "en-IN"
	case "hindi", "hi", "hi-in":
		language = "hi-IN"
	default:
		return "", fmt.Errorf("unsupported reading language %q", language)
	}
	mode := "verbatim"
	if language == "en-IN" {
		// Saaras v3 verbatim can render English words in Devanagari even
		// with en-IN. Standard transcription preserves English script.
		mode = "transcribe"
	}
	r, e := s.call(ctx, "POST", "", map[string]any{"job_parameters": map[string]any{"model": s.Model, "mode": mode, "language_code": language}})
	if e != nil {
		return "", e
	}
	id, _ := r["job_id"].(string)
	if id == "" || strings.ContainsAny(id, "/?#") {
		return "", fmt.Errorf("invalid provider job ID")
	}
	return id, nil
}

var ErrStorageURL = errors.New("unsupported provider storage URL")
var s3StorageHost = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*\.s3(?:\.[a-z0-9-]+)?\.amazonaws\.com$`)

func signedURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !(strings.HasSuffix(u.Hostname(), ".blob.core.windows.net") || u.Hostname() == "storage.googleapis.com" || s3StorageHost.MatchString(u.Hostname())) {
		return ErrStorageURL
	}
	return nil
}
func (s *Sarvam) transfer(ctx context.Context, method, raw, mediaType string, data []byte) ([]byte, error) {
	if e := signedURL(raw); e != nil {
		return nil, e
	}
	r, e := http.NewRequestWithContext(ctx, method, raw, bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	if method == "PUT" {
		r.Header.Set("Content-Type", mediaType)
		if u, _ := url.Parse(raw); strings.HasSuffix(u.Hostname(), ".blob.core.windows.net") {
			r.Header.Set("x-ms-blob-type", "BlockBlob")
		}
	}
	resp, e := s.Client.Do(r)
	if e != nil {
		return nil, fmt.Errorf("provider storage transfer failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider storage HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}
func fileURL(result map[string]any, field, name string) string {
	files, _ := result[field].(map[string]any)
	item, _ := files[name].(map[string]any)
	raw, _ := item["file_url"].(string)
	return raw
}
func (s *Sarvam) Upload(ctx context.Context, id, mediaType string, data []byte) error {
	extension := map[string]string{"audio/wav": "wav", "audio/webm": "webm", "audio/ogg": "ogg", "audio/mp4": "mp4"}[mediaType]
	name := "recording." + extension
	r, e := s.call(ctx, "POST", "/upload-files", map[string]any{"job_id": id, "files": []string{name}})
	if e != nil {
		return e
	}
	_, e = s.transfer(ctx, "PUT", fileURL(r, "upload_urls", name), mediaType, data)
	return e
}
func (s *Sarvam) Start(ctx context.Context, id string) error {
	_, e := s.call(ctx, "POST", "/"+url.PathEscape(id)+"/start", map[string]any{})
	return e
}
func (s *Sarvam) Poll(ctx context.Context, id string) (string, bool, error) {
	r, e := s.call(ctx, "GET", "/"+url.PathEscape(id)+"/status", nil)
	if e != nil {
		return "", false, e
	}
	state, _ := r["job_state"].(string)
	if state == "Failed" {
		return "", false, fmt.Errorf("provider job failed")
	}
	if state != "Completed" {
		return "", false, nil
	}
	details, _ := r["job_details"].([]any)
	names := []string{}
	for _, entry := range details {
		d, _ := entry.(map[string]any)
		outputs, _ := d["outputs"].([]any)
		for _, o := range outputs {
			item, _ := o.(map[string]any)
			name, _ := item["file_name"].(string)
			if name != "" {
				names = append(names, name)
			}
		}
	}
	if len(names) != 1 {
		return "", false, fmt.Errorf("unexpected provider output count")
	}
	r, e = s.call(ctx, "POST", "/download-files", map[string]any{"job_id": id, "files": names})
	if e != nil {
		return "", false, e
	}
	data, e := s.transfer(ctx, "GET", fileURL(r, "download_urls", names[0]), "", nil)
	if e != nil {
		return "", false, e
	}
	var output struct {
		Transcript string `json:"transcript"`
	}
	if e = json.Unmarshal(data, &output); e != nil {
		return "", false, e
	}
	if len(output.Transcript) > 100000 {
		return "", false, fmt.Errorf("transcript exceeds limit")
	}
	return output.Transcript, true, nil
}

// State permits recovery when the provider accepted start but its response was lost.
func (s *Sarvam) State(ctx context.Context, id string) (string, error) {
	r, e := s.call(ctx, "GET", "/"+url.PathEscape(id)+"/status", nil)
	if e != nil {
		return "", e
	}
	state, _ := r["job_state"].(string)
	return state, nil
}
