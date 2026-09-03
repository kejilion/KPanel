//go:build !linux

package main

import "os"

func rootOwned(os.FileInfo) bool {
	return false
}
