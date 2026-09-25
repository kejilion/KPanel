//go:build windows

package desktopwallpapers

// Windows development builds have no O_NOFOLLOW; the regular-file check still applies.
const noFollow = 0
