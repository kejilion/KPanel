package cluster

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/desktopcredentials"
)

func TestDesktopManagedCredentialsStayOnSingleClaimedNoiseLease(t *testing.T) {
	f, node, key, peer := desktopFixture(t)
	record, _ := f.service.light.Host(node.NodeID)
	snapshot := *record.LastSnapshot
	snapshot.NodeCapabilities = append(snapshot.NodeCapabilities, "desktop-managed")
	if _, err := f.service.light.UpdateReport(node.NodeID, snapshot, "1.24.0", f.service.now()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	relay, _ := NewTerminalRelayClient(f.server.Client())
	started, cleaned := make(chan string, 4), make(chan struct{}, 4)
	secret := desktopcredentials.Credentials{Username: "managed-test", Domain: "TESTBOX", Password: "not-a-real-password"}
	done := make(chan error, 1)
	go func() {
		done <- relay.RunManagedDesktopStream(ctx, f.server.URL, node.NodeID, node.TargetNodeID, key, peer,
			func(ctx context.Context, stream io.ReadWriteCloser, nonce string) error {
				started <- nonce
				_, err := io.Copy(stream, stream)
				return err
			},
			func(context.Context) (desktopcredentials.Credentials, func() error, error) {
				return secret, func() error { cleaned <- struct{}{}; return nil }, nil
			}, nil)
	}()
	t.Cleanup(func() { cancel(); <-done })
	waitDesktop(t, f, node.NodeID)
	nonce := strings.Repeat("a", 64)
	stream, got, err := f.service.PrepareManagedDesktop(ctx, node.NodeID, nonce)
	if err != nil || got != secret {
		t.Fatal("credential preparation failed", err)
	}
	defer stream.Close()
	if _, err := stream.Write([]byte("too early")); err == nil {
		t.Fatal("unclaimed stream writable")
	}
	select {
	case <-started:
		t.Fatal("desktop started before browser claim")
	default:
	}
	if err := stream.Claim(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Claim(); err == nil {
		t.Fatal("repeated claim accepted")
	}
	select {
	case got := <-started:
		if got != nonce {
			t.Fatal("nonce mismatch")
		}
	case <-ctx.Done():
		t.Fatal("claim never started RDP")
	}
	if _, err := stream.Write([]byte("RDP")); err != nil {
		t.Fatal(err)
	}
	value := make([]byte, 3)
	if _, err := io.ReadFull(stream, value); err != nil || string(value) != "RDP" {
		t.Fatal("claimed stream failed", err)
	}
	stream.Close()
	select {
	case <-cleaned:
	case <-ctx.Done():
		t.Fatal("lease not cleaned after close")
	}

	// A new lease that never reaches a browser must also revoke its credential.
	unclaimed, _, err := f.service.PrepareManagedDesktop(ctx, node.NodeID, nonce)
	// The callback signal precedes worker teardown and gate release. Admission
	// may correctly reject that brief overlap; do not depend on goroutine order.
	for err == ErrRateLimited && ctx.Err() == nil {
		time.Sleep(time.Millisecond)
		unclaimed, _, err = f.service.PrepareManagedDesktop(ctx, node.NodeID, nonce)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.SetDesktopAllowed(node.NodeID, false); err != nil {
		t.Fatal(err)
	}
	unclaimed.Close()
	select {
	case <-cleaned:
	case <-ctx.Done():
		t.Fatal("unclaimed credential survived policy revoke")
	}
	select {
	case <-started:
		t.Fatal("unclaimed desktop started")
	default:
	}
	if _, _, err := f.service.PrepareManagedDesktop(ctx, node.NodeID, nonce); err == nil {
		t.Fatal("revoked policy returned credential")
	}
}

func TestDesktopManagedRefusesUnsupportedAndInvalidCredentials(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsupported", true: "invalid-credential"}[invalid], func(t *testing.T) {
			f, node, key, peer := desktopFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if invalid {
				r, _ := f.service.light.Host(node.NodeID)
				snapshot := *r.LastSnapshot
				snapshot.NodeCapabilities = append(snapshot.NodeCapabilities, "desktop-managed")
				f.service.light.UpdateReport(node.NodeID, snapshot, "1.24.0", f.service.now())
			}
			relay, _ := NewTerminalRelayClient(f.server.Client())
			cleaned := make(chan struct{}, 1)
			done := make(chan error, 1)
			go func() {
				done <- relay.RunManagedDesktopStream(ctx, f.server.URL, node.NodeID, node.TargetNodeID, key, peer,
					func(context.Context, io.ReadWriteCloser, string) error {
						t.Error("invalid lease started RDP")
						return nil
					},
					func(context.Context) (desktopcredentials.Credentials, func() error, error) {
						return desktopcredentials.Credentials{}, func() error { cleaned <- struct{}{}; return nil }, nil
					}, nil)
			}()
			t.Cleanup(func() { cancel(); <-done })
			waitDesktop(t, f, node.NodeID)
			if _, _, err := f.service.PrepareManagedDesktop(ctx, node.NodeID, strings.Repeat("b", 64)); err == nil {
				t.Fatal("invalid managed credential accepted")
			}
			if invalid {
				select {
				case <-cleaned:
				case <-ctx.Done():
					t.Fatal("failed preparation omitted cleanup")
				}
			}
		})
	}
}
