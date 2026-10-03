//go:build windows

package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWindowsICMPUsesUnprivilegedLoopbackAPI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	latency, err, handled := platformICMPProbe(ctx, "127.0.0.1")
	if !handled || err != nil || latency < 0 {
		t.Fatalf("native ICMP: %v %v %v", latency, err, handled)
	}
	if _, err, _ := platformICMPProbe(ctx, "not-an-IP"); err == nil {
		t.Fatal("invalid target accepted")
	}
	cancel()
	if _, err, _ := platformICMPProbe(ctx, "127.0.0.1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error %v", err)
	}
}
