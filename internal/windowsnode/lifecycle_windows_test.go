//go:build windows

package windowsnode

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestLifecycleMutexRetainsOwnershipAndReleasesAcrossGoroutines(t *testing.T) {
	name := fmt.Sprintf(`Local\KPanelLifecycleTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
	for iteration := 0; iteration < 100; iteration++ {
		release, err := acquireLifecycleMutex(name)
		if err != nil {
			t.Fatal(err)
		}
		// Scheduling must not release the mutex or turn a second acquisition into
		// the recursive acquisition permitted to the owning Windows thread.
		runtime.Gosched()
		if unexpected, err := acquireLifecycleMutex(name); err == nil {
			_ = unexpected()
			_ = release()
			t.Fatal("second lifecycle operation acquired an owned mutex")
		}
		var group sync.WaitGroup
		for worker := 0; worker < 8; worker++ {
			group.Go(func() {
				if err := release(); err != nil {
					t.Errorf("release failed: %v", err)
				}
			})
		}
		group.Wait()
		// The next iteration immediately reacquires the same named mutex; no
		// abandoned owner or ERROR_NOT_OWNER may hide behind a discarded error.
	}
}
