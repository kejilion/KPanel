//go:build !linux

package main

import "os"

func readUpdateHealthFile(string) ([]byte, error) { return nil, os.ErrPermission }
