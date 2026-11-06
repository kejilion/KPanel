//go:build windows

package monitoring

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net"
	"net/netip"
	"strconv"
	"time"
	"unsafe"
)

var icmpDLL = windows.NewLazySystemDLL("iphlpapi.dll")

func platformICMPProbe(ctx context.Context, address string) (time.Duration, error, bool) {
	if err := ctx.Err(); err != nil {
		return 0, err, true
	}
	parsed, parseErr := netip.ParseAddr(address)
	if parseErr != nil {
		return 0, errors.New("ICMP latency target must be an IP address"), true
	}
	ip := net.IP(parsed.AsSlice())
	timeout := operatorProbeTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, context.DeadlineExceeded, true
		}
		timeout = min(timeout, remaining)
	}
	if ip.To4() == nil {
		return platformICMP6Probe(ctx, ip, parsed.Zone(), timeout)
	}
	ip = ip.To4()
	create := icmpDLL.NewProc("IcmpCreateFile")
	send := icmpDLL.NewProc("IcmpSendEcho2")
	closeHandle := icmpDLL.NewProc("IcmpCloseHandle")
	h, _, err := create.Call()
	if h == uintptr(windows.InvalidHandle) {
		return 0, err, true
	}
	defer closeHandle.Call(h)
	payload := []byte("kpanel")
	// ICMP_ECHO_REPLY layout varies with pointer width; the documented minimum
	// is sizeof(reply)+payload+8. 256 bytes bounds and covers both supported ABIs.
	buffer := make([]byte, 256)
	started := time.Now()
	count, _, err := send.Call(h, 0, 0, 0, uintptr(binary.LittleEndian.Uint32(ip)), uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(max(1, timeout.Milliseconds())))
	// Synchronous calls own their buffers until completion. Cancellation never
	// frees a buffer still being written by an overlapped Windows API operation;
	// worst-case cancellation latency is the existing 1.5-second probe budget.
	if e := ctx.Err(); e != nil {
		return 0, e, true
	}
	if count == 0 {
		return 0, err, true
	}
	status := binary.LittleEndian.Uint32(buffer[4:8])
	if status != 0 {
		return 0, fmt.Errorf("ICMP reply status %d", status), true
	}
	if binary.LittleEndian.Uint32(buffer[:4]) != binary.LittleEndian.Uint32(ip) {
		return 0, errors.New("ICMP reply address mismatch"), true
	}
	return time.Since(started), nil, true
}

func platformICMP6Probe(ctx context.Context, ip net.IP, zone string, timeout time.Duration) (time.Duration, error, bool) {
	var scopeID uint32
	if zone != "" {
		if numeric, err := strconv.ParseUint(zone, 10, 32); err == nil {
			scopeID = uint32(numeric)
		} else {
			iface, err := net.InterfaceByName(zone)
			if err != nil {
				return 0, err, true
			}
			scopeID = uint32(iface.Index)
		}
	}
	create := icmpDLL.NewProc("Icmp6CreateFile")
	send := icmpDLL.NewProc("Icmp6SendEcho2")
	closeHandle := icmpDLL.NewProc("IcmpCloseHandle")
	h, _, err := create.Call()
	if h == uintptr(windows.InvalidHandle) {
		return 0, err, true
	}
	defer closeHandle.Call(h)
	source := windows.RawSockaddrInet6{Family: windows.AF_INET6}
	destination := windows.RawSockaddrInet6{Family: windows.AF_INET6, Scope_id: scopeID}
	copy(destination.Addr[:], ip.To16())
	payload := []byte("kpanel")
	// Synchronous Icmp6SendEcho2 needs ICMPV6_ECHO_REPLY + payload + 8
	// ICMP error bytes + IO_STATUS_BLOCK; 256 bytes covers both Windows ABIs.
	buffer := make([]byte, 256)
	started := time.Now()
	count, _, err := send.Call(h, 0, 0, 0, uintptr(unsafe.Pointer(&source)), uintptr(unsafe.Pointer(&destination)), uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(max(1, timeout.Milliseconds())))
	if e := ctx.Err(); e != nil {
		return 0, e, true
	}
	if count == 0 {
		return 0, err, true
	}
	// IPV6_ADDRESS_EX's address starts at byte 6; the reply status is at
	// byte 28 after the address/scope structure's alignment padding.
	if status := binary.LittleEndian.Uint32(buffer[28:32]); status != 0 {
		return 0, fmt.Errorf("ICMPv6 reply status %d", status), true
	}
	if !net.IP(buffer[6:22]).Equal(ip) {
		return 0, errors.New("ICMPv6 reply address mismatch"), true
	}
	return time.Since(started), nil, true
}
