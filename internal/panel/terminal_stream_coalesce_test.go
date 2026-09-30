package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// jobChunkAgentStub serves fixed task terminal chunks keyed by offset and
// records every query, so tests can see whether a follow-up read happened.
type jobChunkAgentStub struct {
	terminalAgentStub
	mu      sync.Mutex
	chunks  map[string]jobTerminalChunk
	queries []url.Values
}

func (s *jobChunkAgentStub) Get(ctx context.Context, path, rawQuery, requestID string) (AgentResponse, error) {
	if !strings.HasPrefix(path, "/v1/app-jobs/") {
		return s.terminalAgentStub.Get(ctx, path, rawQuery, requestID)
	}
	values, _ := url.ParseQuery(rawQuery)
	s.mu.Lock()
	s.queries = append(s.queries, values)
	chunk := s.chunks[values.Get("offset")]
	s.mu.Unlock()
	body, _ := json.Marshal(chunk)
	return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}, nil
}

func encodedJobData(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}

func TestCoalesceJobTerminalMergesFollowingOutput(t *testing.T) {
	server, _ := newTestServer(t)
	stub := &jobChunkAgentStub{chunks: map[string]jobTerminalChunk{
		"5": {DataBase64: encodedJobData(" world"), NextOffset: 11, InputOpen: false},
	}}
	server.agent = stub
	first := jobTerminalChunk{DataBase64: encodedJobData("hello"), NextOffset: 5, InputOpen: true}

	merged := server.coalesceJobTerminal(context.Background(), "/v1/app-jobs/", "job", first)

	data, _ := base64.StdEncoding.DecodeString(merged.DataBase64)
	if string(data) != "hello world" || merged.NextOffset != 11 || merged.InputOpen {
		t.Fatalf("merged chunk = %#v (%q)", merged, data)
	}
	if len(stub.queries) != 1 || stub.queries[0].Get("wait") != "0" || stub.queries[0].Get("offset") != "5" {
		t.Fatalf("follow-up read = %#v, want one non-blocking read at the next offset", stub.queries)
	}
}

func TestCoalesceJobTerminalKeepsChunkWhenFollowUpDoesNotContinueIt(t *testing.T) {
	cases := map[string]jobTerminalChunk{
		"rotated log":         {DataBase64: encodedJobData("later"), NextOffset: 4096, Truncated: true},
		"discontinuous bytes": {DataBase64: encodedJobData("gap"), NextOffset: 42},
	}
	for name, follow := range cases {
		t.Run(name, func(t *testing.T) {
			server, _ := newTestServer(t)
			server.agent = &jobChunkAgentStub{chunks: map[string]jobTerminalChunk{"5": follow}}
			first := jobTerminalChunk{DataBase64: encodedJobData("hello"), NextOffset: 5, InputOpen: true}
			if got := server.coalesceJobTerminal(context.Background(), "/v1/app-jobs/", "job", first); got != first {
				t.Fatalf("chunk = %#v, want the original %#v", got, first)
			}
		})
	}
}

func TestCoalesceJobTerminalSkipsLargeChunks(t *testing.T) {
	server, _ := newTestServer(t)
	stub := &jobChunkAgentStub{chunks: map[string]jobTerminalChunk{}}
	server.agent = stub
	large := jobTerminalChunk{DataBase64: encodedJobData(strings.Repeat("x", terminalStreamCoalesceBytes)), NextOffset: terminalStreamCoalesceBytes}
	if got := server.coalesceJobTerminal(context.Background(), "/v1/app-jobs/", "job", large); got != large || len(stub.queries) != 0 {
		t.Fatalf("large chunk was delayed or re-read: %#v queries=%d", got, len(stub.queries))
	}
}
