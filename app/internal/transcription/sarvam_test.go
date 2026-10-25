package transcription

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSarvamCreateReadingLanguage(t *testing.T) {
	for _, tc := range []struct{ language, code string }{
		{"english", "en-IN"}, {"English", "en-IN"}, {" en ", "en-IN"}, {"en-IN", "en-IN"},
		{"hindi", "hi-IN"}, {"Hindi", "hi-IN"}, {" hi ", "hi-IN"}, {"hi-IN", "hi-IN"},
		{"", ""}, {"unknown", ""},
	} {
		t.Run(tc.language, func(t *testing.T) {
			client := New("https://api.sarvam.ai", "test-secret", "saaras:v3")
			called := false
			client.Client.Transport = transport(func(r *http.Request) (*http.Response, error) {
				called = true
				if tc.code == "" {
					t.Fatal("invalid language reached provider")
				}
				var input struct {
					Parameters map[string]string `json:"job_parameters"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Fatal(err)
				}
				wantMode := "verbatim"
				if tc.code == "en-IN" {
					wantMode = "transcribe"
				}
				if input.Parameters["language_code"] != tc.code || input.Parameters["mode"] != wantMode || input.Parameters["model"] != "saaras:v3" {
					t.Fatalf("unexpected job parameters: %v", input.Parameters)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"job_id":"job-1"}`))}, nil
			})
			id, err := client.Create(context.Background(), tc.language)
			if tc.code == "" {
				if err == nil || id != "" {
					t.Fatal("invalid language was accepted")
				}
			} else if err != nil || id != "job-1" || !called {
				t.Fatalf("Create = %q, %v; called = %v", id, err, called)
			}
		})
	}
}

func TestSarvamBatchAndCredentialBoundary(t *testing.T) {
	calls := []string{}
	client := New("https://api.sarvam.ai", " test-secret\n", "saaras:v3")
	client.Client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		body := "{}"
		if r.URL.Host == "api.sarvam.ai" {
			if r.Header.Get("api-subscription-key") != "test-secret" {
				t.Fatal("missing API credential")
			}
			switch r.URL.Path {
			case "/speech-to-text/job/v1":
				body = `{"job_id":"job-1"}`
			case "/speech-to-text/job/v1/upload-files":
				body = `{"upload_urls":{"recording.wav":{"file_url":"https://test.blob.core.windows.net/audio?signature=synthetic"}}}`
			case "/speech-to-text/job/v1/job-1/status":
				body = `{"job_state":"Completed","job_details":[{"outputs":[{"file_name":"0.json"}]}]}`
			case "/speech-to-text/job/v1/download-files":
				var input map[string]any
				if e := json.NewDecoder(r.Body).Decode(&input); e != nil {
					t.Fatal(e)
				}
				if input["job_id"] != "job-1" || len(input["files"].([]any)) != 1 {
					t.Fatal("missing batch output file")
				}
				body = `{"download_urls":{"0.json":{"file_url":"https://test.blob.core.windows.net/result?signature=synthetic"}}}`
			}
		} else {
			if r.Header.Get("api-subscription-key") != "" {
				t.Fatal("credential leaked to storage")
			}
			if r.Method == "GET" {
				body = `{"transcript":"हम किताब पढ़ते हैं"}`
			} else if r.Header.Get("x-ms-blob-type") != "BlockBlob" {
				t.Fatal("Azure upload header missing")
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})
	ctx := context.Background()
	id, e := client.Create(ctx, "hi-IN")
	if e != nil || id != "job-1" {
		t.Fatal(id, e)
	}
	if e = client.Upload(ctx, id, "audio/wav", []byte("synthetic")); e != nil {
		t.Fatal(e)
	}
	if e = client.Start(ctx, id); e != nil {
		t.Fatal(e)
	}
	text, done, e := client.Poll(ctx, id)
	if e != nil || !done || text != "हम किताब पढ़ते हैं" {
		t.Fatal(text, done, e)
	}
	if len(calls) != 7 {
		t.Fatal(calls)
	}
}
func TestProviderStorageURLRestrictions(t *testing.T) {
	for _, raw := range []string{"http://test.blob.core.windows.net/a", "https://evil.test/audio", "https://test.blob.core.windows.net.evil.test/a", "https://user@test.blob.core.windows.net/a", "https://127.0.0.1/a"} {
		if signedURL(raw) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestSarvamPlaceholderCredentialDoesNotCallProvider(t *testing.T) {
	for _, key := range []string{"", " ", "dummy", " DUMMY "} {
		client := New("https://api.sarvam.ai", key, "saaras:v3")
		client.Client.Transport = transport(func(r *http.Request) (*http.Response, error) {
			t.Fatal("placeholder credential reached provider")
			return nil, nil
		})
		if _, err := client.Create(context.Background(), "hi-IN"); err == nil {
			t.Fatal("accepted placeholder credential")
		}
	}
}

func TestSarvamS3StorageTransfers(t *testing.T) {
	client := New("https://api.sarvam.ai", "private-key", "saaras:v3")
	client.Client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("api-subscription-key") != "" {
			t.Fatal("API credential leaked to storage")
		}
		if r.Header.Get("x-ms-blob-type") != "" {
			t.Fatal("Azure header sent to S3")
		}
		if r.Method == "PUT" && r.Header.Get("Content-Type") != "audio/webm" {
			t.Fatal("audio content type lost")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"transcript":"test"}`)), Header: http.Header{}}, nil
	})
	for _, host := range []string{"saas-prod-vistaar-bulk-upload.s3.amazonaws.com", "results.s3.ap-south-1.amazonaws.com"} {
		for _, method := range []string{"PUT", "GET"} {
			if _, err := client.transfer(context.Background(), method, "https://"+host+"/synthetic?signature=test", "audio/webm", []byte("synthetic")); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, raw := range []string{"https://evil.amazonaws.com/a", "https://bucket.s3.amazonaws.com.evil.test/a", "https://user@bucket.s3.amazonaws.com/a", "https://bucket.s3.amazonaws.com:443/a", "http://bucket.s3.amazonaws.com/a"} {
		if signedURL(raw) == nil {
			t.Fatalf("accepted untrusted URL %s", raw)
		}
	}
}
