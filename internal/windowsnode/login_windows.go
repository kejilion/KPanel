//go:build windows

package windowsnode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unsafe"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"golang.org/x/sys/windows"
)

var eventDLL = windows.NewLazySystemDLL("wevtapi.dll")
var eventQuery = eventDLL.NewProc("EvtQuery")
var eventNext = eventDLL.NewProc("EvtNext")
var eventRender = eventDLL.NewProc("EvtRender")
var eventClose = eventDLL.NewProc("EvtClose")
var windowsAccepted = regexp.MustCompile(`(?i)\bAccepted (password|publickey) for ([^\s]+) from ([^\s]+)`)

func LatestLogin(ctx context.Context) (*contract.SSHLoginEvent, error) {
	var latest *contract.SSHLoginEvent
	var failures []error
	for _, source := range []struct{ channel, query string }{
		{"Security", "*[System[(EventID=4624) and TimeCreated[timediff(@SystemTime)<=86400000]] and EventData[Data[@Name='LogonType']='10']]"},
		{"OpenSSH/Operational", "*[System[TimeCreated[timediff(@SystemTime)<=86400000]]]"},
	} {
		channel, _ := windows.UTF16PtrFromString(source.channel)
		query, _ := windows.UTF16PtrFromString(source.query)
		result, _, err := eventQuery.Call(0, uintptr(unsafe.Pointer(channel)), uintptr(unsafe.Pointer(query)), 0x201)
		if result == 0 {
			failures = append(failures, err)
			continue
		}
		func() {
			defer eventClose.Call(result)
			handles := [16]uintptr{}
			var count uint32
			ok, _, err := eventNext.Call(result, 16, uintptr(unsafe.Pointer(&handles[0])), 0, 0, uintptr(unsafe.Pointer(&count)))
			if ok == 0 {
				if !errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
					failures = append(failures, err)
				}
				return
			}
			for i := uint32(0); i < count && i < 16; i++ {
				func() {
					defer eventClose.Call(handles[i])
					if ctx.Err() != nil {
						return
					}
					buffer := make([]uint16, 16384)
					var used, properties uint32
					ok, _, _ := eventRender.Call(0, handles[i], 1, uintptr(len(buffer)*2), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&used)), uintptr(unsafe.Pointer(&properties)))
					if ok == 0 || used > uint32(len(buffer)*2) {
						return
					}
					event, err := ParseLoginXML([]byte(windows.UTF16ToString(buffer)), source.channel, time.Now())
					if err == nil && event != nil && (latest == nil || event.OccurredAt.After(latest.OccurredAt)) {
						latest = event
					}
				}()
			}
		}()
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if latest != nil {
		return latest, nil
	}
	if len(failures) == 2 {
		return nil, errors.New("Windows remote login event channels unavailable")
	}
	return nil, nil
}
func ParseLoginXML(data []byte, channel string, now time.Time) (*contract.SSHLoginEvent, error) {
	if len(data) > 32768 {
		return nil, errors.New("event exceeds limit")
	}
	var value struct {
		System struct {
			ID     int    `xml:"EventID"`
			Record string `xml:"EventRecordID"`
			Time   struct {
				At string `xml:"SystemTime,attr"`
			} `xml:"TimeCreated"`
		} `xml:"System"`
		Data []struct {
			Name  string `xml:"Name,attr"`
			Value string `xml:",chardata"`
		} `xml:"EventData>Data"`
	}
	if xml.Unmarshal(data, &value) != nil || len(value.Data) > 256 {
		return nil, errors.New("invalid event XML")
	}
	at, err := time.Parse(time.RFC3339Nano, value.System.Time.At)
	if err != nil || at.After(now.Add(time.Minute)) || at.Before(now.Add(-24*time.Hour)) {
		return nil, errors.New("event time outside observation window")
	}
	fields := map[string]string{}
	for _, item := range value.Data {
		if _, ok := fields[item.Name]; ok {
			return nil, errors.New("duplicate event field")
		}
		fields[item.Name] = item.Value
	}
	event := &contract.SSHLoginEvent{OccurredAt: at}
	switch channel {
	case "Security":
		if value.System.ID != 4624 || fields["LogonType"] != "10" {
			return nil, nil
		}
		event.Method = "rdp"
		event.Username = fields["TargetUserName"]
		event.RemoteAddress = fields["IpAddress"]
	case "OpenSSH/Operational":
		for _, item := range value.Data {
			match := windowsAccepted.FindStringSubmatch(item.Value)
			if match == nil {
				continue
			}
			event.Method = "ssh-" + strings.ToLower(match[1])
			event.Username = match[2]
			event.RemoteAddress = match[3]
			break
		}
		if event.Method == "" {
			return nil, nil
		}
	default:
		return nil, errors.New("unknown event channel")
	}
	if _, err := netip.ParseAddr(event.RemoteAddress); err != nil {
		return nil, errors.New("invalid remote login address")
	}
	digest := sha256.Sum256([]byte(channel + ":" + value.System.Record + ":" + at.Format(time.RFC3339Nano)))
	event.ID = "windows:" + hex.EncodeToString(digest[:])
	if !contract.ValidSSHLoginEvent(*event) {
		return nil, errors.New("invalid remote login event")
	}
	return event, nil
}
