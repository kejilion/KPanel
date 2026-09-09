package filemanager

import (
	"context"
	"sync"
)

// Preserve serialized mutations while letting queued archive jobs stop promptly.
type cancellableMutex struct {
	once sync.Once
	gate chan struct{}
}

func (m *cancellableMutex) LockContext(ctx context.Context) error {
	m.once.Do(func() { m.gate = make(chan struct{}, 1) })
	select {
	case m.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *cancellableMutex) Lock()   { _ = m.LockContext(context.Background()) }
func (m *cancellableMutex) Unlock() { <-m.gate }
