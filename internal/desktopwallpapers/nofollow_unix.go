//go:build !windows

package desktopwallpapers

import "syscall"

// noFollow refuses to open a stored image through a symbolic link.
const noFollow = syscall.O_NOFOLLOW
