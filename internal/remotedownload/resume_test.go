package remotedownload

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDownloadResumeRequiresMatchingStrongValidatorAndExactRange(t *testing.T) {
	for _, test := range []struct {
		name, etag, contentRange string
		status                   int
		wantError                bool
	}{
		{"range", `"version-1"`, "bytes 4-9/10", 206, false},
		{"ignored-range", `"version-1"`, "", 200, false},
		{"changed", `"version-2"`, "bytes 4-9/10", 206, true},
		{"wrong-offset", `"version-1"`, "bytes 3-9/10", 206, true},
		{"unknown-total", `"version-1"`, "bytes 4-9/*", 206, true},
		{"wrong-total", `"version-1"`, "bytes 4-8/9", 206, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := NewClient(Config{})
			client.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("Range") != "bytes=4-" || request.Header.Get("If-Range") != `"version-1"` || request.Header.Get("Accept-Encoding") != "identity" {
					t.Fatalf("invalid resume headers: %v", request.Header)
				}
				header := make(http.Header)
				header.Set("ETag", test.etag)
				if test.contentRange != "" {
					header.Set("Content-Range", test.contentRange)
				}
				length := int64(6)
				if test.status == 200 {
					length = 10
				}
				return &http.Response{StatusCode: test.status, Header: header, ContentLength: length, Body: io.NopCloser(strings.NewReader("body")), Request: request}, nil
			})
			response, err := client.OpenRange(context.Background(), "https://downloads.example.com/file", ResumeRequest{Offset: 4, SizeBytes: 10, ETag: `"version-1"`, FinalURL: "https://downloads.example.com/file"})
			if test.wantError {
				if !errors.Is(err, ErrSourceChanged) {
					t.Fatalf("resume accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
		})
	}
}

func TestDownloadResumeKeepsURLPolicyAndRejectsWeakValidators(t *testing.T) {
	client := NewClient(Config{})
	for _, validator := range []string{"", `W/"version-1"`, "invalid", "\"bad\r\nheader\""} {
		if _, err := client.OpenRange(context.Background(), "https://downloads.example.com/file", ResumeRequest{Offset: 4, SizeBytes: 10, ETag: validator}); !errors.Is(err, ErrSourceChanged) {
			t.Fatalf("weak validator %q accepted: %v", validator, err)
		}
	}
	if _, err := client.OpenRange(context.Background(), "http://localhost/file", ResumeRequest{Offset: 4, SizeBytes: 10, ETag: `"version-1"`}); !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("resume bypassed URL policy: %v", err)
	}
}
