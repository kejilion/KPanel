//go:build windows

package monitoring

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net"
	"time"
	"unsafe"
)

var icmpDLL = windows.NewLazySystemDLL("iphlpapi.dll")

func platformICMPProbe(ctx context.Context, address string) (time.Duration, error, bool) {
	if err := ctx.Err(); err != nil {
		return 0, err, true
	}
	ip := net.ParseIP(address).To4()
	if ip == nil {
		return 0, errors.New("ICMP latency target must be an IPv4 address"), true
	}
	timeout := operatorProbeTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, context.DeadlineExceeded, true
		}
		timeout = min(timeout, remaining)
	}
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
