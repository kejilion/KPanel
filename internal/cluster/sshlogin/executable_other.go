//go:build !linux

package sshlogin

func trustedLogExecutable(string) bool { return false }
