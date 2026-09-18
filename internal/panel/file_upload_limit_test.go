package panel

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

func TestPanelUploadBodyEnforcesCeilingWithoutContentLength(t *testing.T) {
	recorder := httptest.NewRecorder()
	atLimit := newPanelUploadBody(recorder, io.NopCloser(io.LimitReader(zeroReader{}, filemanager.MaxUploadBytes)))
	if copied, err := io.Copy(io.Discard, atLimit); err != nil || copied != filemanager.MaxUploadBytes || atLimit.exceeded.Load() {
		t.Fatalf("upload at the ceiling = %d, %v, exceeded=%v", copied, err, atLimit.exceeded.Load())
	}
	over := newPanelUploadBody(recorder, io.NopCloser(io.LimitReader(zeroReader{}, filemanager.MaxUploadBytes+1)))
	if _, err := io.Copy(io.Discard, over); err == nil || !over.exceeded.Load() {
		t.Fatalf("upload past the ceiling was accepted: err=%v exceeded=%v", err, over.exceeded.Load())
	}
}
