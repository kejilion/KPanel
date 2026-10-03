//go:build !windows

package desktopbridge

func localEndpoint() (endpoint, error) { return endpoint{}, ErrUnsupported }
