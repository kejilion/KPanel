package contract

import "strings"

const (
	SSHLoginEventIDMaxBytes  = 160
	SSHLoginUsernameMaxBytes = 128
	SSHLoginAddressMaxBytes  = 253
	SSHLoginMethodMaxBytes   = 64
)

// ValidSSHLoginEvent validates the intentionally small event surface before
// it crosses a federation boundary or is rendered in an external message.
func ValidSSHLoginEvent(value SSHLoginEvent) bool {
	return !value.OccurredAt.IsZero() &&
		validSSHLoginText(value.ID, SSHLoginEventIDMaxBytes) &&
		validSSHLoginText(value.Username, SSHLoginUsernameMaxBytes) &&
		validSSHLoginText(value.RemoteAddress, SSHLoginAddressMaxBytes) &&
		validSSHLoginText(value.Method, SSHLoginMethodMaxBytes)
}

func validSSHLoginText(value string, maxBytes int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxBytes || strings.ContainsAny(value, "\r\n\t\x00") {
		return false
	}
	for _, character := range value {
		if character < ' ' || character == '\u007f' {
			return false
		}
	}
	return true
}
