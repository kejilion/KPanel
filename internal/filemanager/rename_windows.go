//go:build windows

package filemanager

func isCrossDeviceError(error) bool {
	return false
}
