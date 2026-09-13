package panel

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

type fileTransferEndError struct{}

func (fileTransferEndError) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestFileTransferResultRequiresCompleteTransportAndSingleJSON(t *testing.T) {
	for _, mode := range []string{"complete", "missing-end", "extra-json", "over-limit"} {
		t.Run(mode, func(t *testing.T) {
			var body io.Reader = strings.NewReader(`{"name":"copied.txt","kind":"file"}`)
			switch mode {
			case "missing-end":
				body = io.MultiReader(body, fileTransferEndError{})
			case "extra-json":
				body = io.MultiReader(body, strings.NewReader(`{}`))
			case "over-limit":
				body = io.MultiReader(body, strings.NewReader(strings.Repeat(" ", 1<<20)))
			}
			entry, err := decodeFileTransferEntry(body)
			if mode == "complete" {
				if err != nil || entry.Name != "copied.txt" {
					t.Fatalf("entry=%v err=%v", entry, err)
				}
			} else if err == nil {
				t.Fatal("incomplete or invalid response accepted")
			}
		})
	}
}

func TestLocalFileTransferChecksAgentFinalTrailer(t *testing.T) {
	for _, result := range []string{"ok", "error", ""} {
		t.Run(result, func(t *testing.T) {
			header, trailer := make(http.Header), make(http.Header)
			header.Set(cluster.FileTransferMetadataHeader, base64.RawURLEncoding.EncodeToString([]byte(`{"name":"source","kind":"directory","sizeBytes":0,"resourceVersion":"version"}`)))
			trailer.Set("X-KPanel-Transfer-Result", result)
			server := &Server{agent: &federatedFileStreamAgentStub{response: &http.Response{StatusCode: 200, Header: header, Trailer: trailer, Body: io.NopCloser(strings.NewReader("data"))}}}
			body, _, err := server.openLocalFileTransfer(context.Background(), cluster.FederationFileOpenRequest{Path: "/source", ResourceVersion: "version"}, "request")
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(body)
			body.Close()
			if string(data) != "data" {
				t.Fatalf("data=%q", data)
			}
			if result == "ok" && err != nil {
				t.Fatal(err)
			}
			if result != "ok" && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("invalid export accepted: %v", err)
			}
		})
	}
}
