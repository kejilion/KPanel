//go:build !linux

package selfupdate

func acquireFileLock(string) (func(), error) {
	return func() {}, nil
}
