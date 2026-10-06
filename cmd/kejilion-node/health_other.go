//go:build !linux

package main

import "os"

func readUpdateHealthFile(string) ([]byte, error)          { return nil, os.ErrPermission }
func readTrustedRuntimeFile(string, int64) ([]byte, error) { return nil, os.ErrPermission }
