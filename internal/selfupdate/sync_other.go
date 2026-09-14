//go:build !linux

package selfupdate

func syncDirectory(string) error { return nil }
