package webbrowser

import (
	"context"
	"testing"
	"time"
)

func TestSnapshotUsesCachedStateWithoutRenderer(t *testing.T) {
	s := newBrowserSession(context.Background())
	defer s.Close()
	// No CDP context: obtaining toolbar state must not enter a renderer at all.
	s.tabs = []*browserTab{{Tab: Tab{ID: "one", Title: "cached", Loading: true}}}
	s.active = "one"
	state := s.Snapshot()
	if state.Active != "one" || len(state.Tabs) != 1 || state.Tabs[0].Title != "cached" {
		t.Fatal(state)
	}
	state.Tabs[0].Title = "changed"
	if s.Snapshot().Tabs[0].Title != "cached" {
		t.Fatal("snapshot aliases workspace state")
	}
	// Avoid asking Close to cancel the deliberately context-free fixture.
	s.tabs = nil
}

func TestHiddenWorkspaceDoesNotPollRenderer(t *testing.T) {
	s := newBrowserSession(context.Background())
	defer s.Close()
	s.visible = false
	s.tabs = []*browserTab{{Tab: Tab{ID: "one"}}}
	done := make(chan struct{})
	go func() { s.RefreshMetadata(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("hidden metadata blocked")
	}
	s.tabs = nil
}
