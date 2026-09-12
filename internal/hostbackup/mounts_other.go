//go:build !linux

package hostbackup

// Non-Linux hosts exercise the archive protocol with isolated fixtures only.
func noNestedMounts(string) error { return nil }
