package cluster

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
	"time"
)

func TestLightFileTransferPollBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		relay := newLightFileRelay(time.Now)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		item, session, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
		defer response.Body.Close()
		chunk := make([]byte, 16<<10)
		started := time.Now()
		for batch := 0; batch < 32; batch++ {
			events := []FileRelayEvent{
				{RequestID: session.id, Kind: "data", Offset: int64(batch * 2 * len(chunk)), Data: chunk},
				{RequestID: session.id, Kind: "data", Offset: int64((batch*2 + 1) * len(chunk)), Data: chunk},
			}
			if _, err := relay.poll(ctx, item.id, []string{session.id}, events); err != nil {
				t.Fatal(err)
			}
			if _, err := io.CopyN(io.Discard, response.Body, int64(2*len(chunk))); err != nil {
				t.Fatal(err)
			}
		}
		elapsed := time.Since(started)
		t.Logf("1 MiB / 32 polls: protocol wait=%s (excludes network, disk and cryptography)", elapsed)
		if elapsed > 4*time.Second {
			t.Fatalf("file transfer spends %s waiting between ready batches", elapsed)
		}
		if elapsed < 3*time.Second {
			t.Fatal("file transfer can burst past the existing 600 requests/minute budget")
		}
	})
}

func TestLightFileLostDataAcknowledgementIsIdempotent(t *testing.T) {
	relay := newLightFileRelay(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, session, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
	defer response.Body.Close()
	for attempt := 0; attempt < 2; attempt++ {
		for i, data := range []string{"first", "chunk"} {
			if err := session.push(ctx, int64(i*5), []byte(data)); err != nil {
				t.Fatalf("delivery %d chunk %d: %v", attempt, i, err)
			}
		}
	}
	if err := session.push(ctx, 10, []byte("last")); err != nil {
		t.Fatal(err)
	}
	session.finish(nil)
	content, err := io.ReadAll(response.Body)
	if err != nil || string(content) != "firstchunklast" {
		t.Fatalf("retry changed bytes: %q, %v", content, err)
	}
}

func TestLightFileReplayStillRejectsChangedOrUnorderedData(t *testing.T) {
	for _, input := range []struct {
		name   string
		offset int64
		data   string
	}{
		{"changed", 0, "other"},
		{"partial", 2, "rst"},
		{"future", 10, "next"},
	} {
		t.Run(input.name, func(t *testing.T) {
			relay := newLightFileRelay(time.Now)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, session, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
			defer response.Body.Close()
			if err := session.push(ctx, 0, []byte("first")); err != nil {
				t.Fatal(err)
			}
			if err := session.push(ctx, input.offset, []byte(input.data)); !errors.Is(err, ErrFileRelayUnavailable) {
				t.Fatalf("invalid data accepted: %v", err)
			}
		})
	}
}
