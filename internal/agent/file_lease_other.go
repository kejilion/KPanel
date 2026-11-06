//go:build !linux

package agent

import "errors"

var errStableFileUnavailable = errors.New("stable file lease unavailable")

type stableReadLease struct{}

func tryStableReadLease(interface{ Fd() uintptr }, func()) (*stableReadLease, error) {
	return nil, errStableFileUnavailable
}

func (*stableReadLease) valid() bool { return false }
func (*stableReadLease) close()      {}
