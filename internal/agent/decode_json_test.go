package agent

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRequiresJSONMediaType(t *testing.T) {
	for contentType, wantErr := range map[string]bool{
		"":                                true,
		"text/plain":                      true,
		"application/json":                false,
		"application/json; charset=utf-8": false,
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/system/actions", strings.NewReader(`{"action":"x"}`))
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		var target struct {
			Action string `json:"action"`
		}
		err := decodeJSON(httptest.NewRecorder(), request, &target)
		if wantErr != errors.Is(err, errJSONContentType) || !wantErr && err != nil {
			t.Errorf("Content-Type %q: err=%v", contentType, err)
		}
	}
}
