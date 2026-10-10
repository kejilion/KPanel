//go:build !linux && !darwin && !freebsd

package bittorrent

// The supported server platforms perform a free-space preflight. Other hosts
// retain the hard logical size bound and propagate write failures.
func checkSpace(_ string, _ int64) error { return nil }
