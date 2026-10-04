//go:build !windows

package terminal

func platformStarter(uint16, uint16) (Process, error, bool) { return nil, nil, false }

func Available() error { _, err := trustedTerminalExecutable("sh"); return err }
