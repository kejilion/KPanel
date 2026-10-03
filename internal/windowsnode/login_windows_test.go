//go:build windows

package windowsnode

import (
	"fmt"
	"testing"
	"time"
)

func TestRDPLoginRequiresRemoteInteractiveLogon(t *testing.T) {
	now := time.Now().UTC()
	fixture := func(kind, address string) []byte {
		return []byte(fmt.Sprintf(`<Event><System><EventID>4624</EventID><EventRecordID>1234</EventRecordID><TimeCreated SystemTime="%s"/></System><EventData><Data Name="LogonType">%s</Data><Data Name="TargetUserName">alice</Data><Data Name="IpAddress">%s</Data></EventData></Event>`, now.Format(time.RFC3339Nano), kind, address))
	}
	event, err := ParseLoginXML(fixture("10", "192.0.2.2"), "Security", now)
	if err != nil || event == nil || event.Method != "rdp" || event.Username != "alice" {
		t.Fatalf("event=%+v err=%v", event, err)
	}
	for _, kind := range []string{"2", "3"} {
		event, err := ParseLoginXML(fixture(kind, "192.0.2.2"), "Security", now)
		if err != nil || event != nil {
			t.Fatal("local/network logon leaked into remote-login report")
		}
	}
	if _, err := ParseLoginXML(fixture("10", "-"), "Security", now); err == nil {
		t.Fatal("invalid address accepted")
	}
	if _, err := ParseLoginXML(fixture("10", "192.0.2.2"), "Security", now.Add(48*time.Hour)); err == nil {
		t.Fatal("stale event accepted")
	}
}
