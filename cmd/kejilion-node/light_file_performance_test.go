package main

import (
	"context"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestLightFileDrainKeepsBackpressureBounded(t *testing.T) {
	session := &lightNodeFileSession{events: make(chan cluster.FileRelayEvent, lightFileEventLimit)}
	for i := 0; i < lightFileEventLimit; i++ {
		session.pending = append(session.pending, cluster.FileRelayEvent{Kind: "data"})
		session.events <- cluster.FileRelayEvent{Kind: "data"}
	}
	session.drain()
	if len(session.pending) != lightFileEventLimit || len(session.events) != lightFileEventLimit {
		t.Fatalf("drain bypassed producer backpressure: pending=%d queued=%d", len(session.pending), len(session.events))
	}
}

func TestLightFileLargeDownloadDoesNotStarveDirectoryResponse(t *testing.T) {
	largeID, smallID := strings.Repeat("a", 32), strings.Repeat("b", 32)
	large := &lightNodeFileSession{requestID: largeID, events: make(chan cluster.FileRelayEvent, lightFileEventLimit)}
	small := &lightNodeFileSession{requestID: smallID, events: make(chan cluster.FileRelayEvent, lightFileEventLimit)}
	for i := 0; i < lightFileEventLimit; i++ {
		large.events <- cluster.FileRelayEvent{RequestID: largeID, Kind: "data", Offset: int64(i * 1024), Data: make([]byte, 1024)}
	}
	small.events <- cluster.FileRelayEvent{RequestID: smallID, Kind: "response", Status: 200}
	small.events <- cluster.FileRelayEvent{RequestID: smallID, Kind: "data", Data: []byte("directory")}
	small.events <- cluster.FileRelayEvent{RequestID: smallID, Kind: "end"}
	control := &lightFileControl{sessions: map[string]*lightNodeFileSession{largeID: large, smallID: small}}
	var smallKinds []string
	for batch := 0; batch < 3; batch++ {
		events, err := control.collectEvents()
		if err != nil || !control.pollPayloadFits(events, control.requestIDs()) {
			t.Fatalf("invalid batch: %v", err)
		}
		for _, event := range events {
			if event.RequestID == smallID {
				smallKinds = append(smallKinds, event.Kind)
			}
		}
		if batch == 0 && len(smallKinds) == 0 {
			t.Fatal("directory response starved behind a large download")
		}
		control.acceptRelayResponse(cluster.FileRelayPollResponse{})
	}
	if strings.Join(smallKinds, ",") != "response,data,end" {
		t.Fatalf("directory response order = %v", smallKinds)
	}
}

func TestLightFileCollectionPreservesByteOrderAcrossBatches(t *testing.T) {
	id := strings.Repeat("a", 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session := &lightNodeFileSession{requestID: id, ctx: ctx, cancel: cancel, events: make(chan cluster.FileRelayEvent, lightFileEventLimit)}
	for i := 0; i < 8; i++ {
		session.events <- cluster.FileRelayEvent{RequestID: id, Kind: "data", Offset: int64(i * lightFileEventChunkBytes), Data: make([]byte, lightFileEventChunkBytes)}
	}
	session.events <- cluster.FileRelayEvent{RequestID: id, Kind: "end"}
	control := &lightFileControl{sessions: map[string]*lightNodeFileSession{id: session}}
	next := int64(0)
	ended := false
	for batch := 0; batch < 10 && !ended; batch++ {
		events, _ := control.collectEvents()
		if !control.pollPayloadFits(events, control.requestIDs()) {
			t.Fatal("batch exceeds wire limit")
		}
		for _, event := range events {
			if event.Kind == "end" {
				ended = true
				continue
			}
			if ended || event.Offset != next {
				t.Fatalf("out of order data: offset=%d want=%d ended=%v", event.Offset, next, ended)
			}
			next += int64(len(event.Data))
		}
		control.acceptRelayResponse(cluster.FileRelayPollResponse{})
	}
	if !ended || next != 8*lightFileEventChunkBytes {
		t.Fatalf("incomplete stream: ended=%v bytes=%d", ended, next)
	}
}
