package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type blockedAgentTerminal struct {
	entered chan struct{}
	done    chan struct{}
	once    sync.Once
}

func (p *blockedAgentTerminal) Read([]byte) (int, error) { <-p.done; return 0, io.EOF }
func (p *blockedAgentTerminal) Write([]byte) (int, error) {
	close(p.entered)
	<-p.done
	return 0, io.ErrClosedPipe
}
func (p *blockedAgentTerminal) Close() error                { p.once.Do(func() { close(p.done) }); return nil }
func (p *blockedAgentTerminal) Kill() error                 { return p.Close() }
func (p *blockedAgentTerminal) Wait() error                 { <-p.done; return nil }
func (p *blockedAgentTerminal) Resize(uint16, uint16) error { return nil }

func TestTerminalCloseAPIBypassesBlockedInputMutationLock(t *testing.T) {
	s := testServer(t)
	p := &blockedAgentTerminal{entered: make(chan struct{}), done: make(chan struct{})}
	s.terminals = terminal.New(terminal.Config{Starter: func(uint16, uint16) (terminal.Process, error) { return p, nil }})
	defer s.terminals.CloseAll()
	opened, err := s.terminals.Open("owner", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	request := func(action string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(http.MethodPost, "/v1/terminals/"+opened.ID+"/"+action, bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	inputDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		inputDone <- request("input-sequenced", map[string]any{"owner": "owner", "frame": terminal.InputFrame{Stream: "00000000000000000000000000000001", Seq: 1, Data: []byte("block")}})
	}()
	<-p.entered
	closeDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { closeDone <- request("close", map[string]string{"owner": "owner"}) }()
	select {
	case response := <-closeDone:
		if response.Code != 200 {
			t.Fatalf("close: %d %s", response.Code, response.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("Agent mutation mutex prevented terminal close")
	}
	if response := <-inputDone; response.Code != 409 || !strings.Contains(response.Body.String(), "terminal_input_uncertain") {
		t.Fatalf("partial write incorrectly acknowledged: %d %s", response.Code, response.Body.String())
	}
}
