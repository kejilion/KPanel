package webbrowser

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestVNCAttachmentRequiresControlWorkspace(t *testing.T) {
	executable, _ := os.Executable()
	service, err := NewService(executable, strings.Repeat("s", 48), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	request := httptest.NewRequest("GET", "/v1/sessions/"+strings.Repeat("a", 64)+"/vnc", nil)
	request.Header.Set("Authorization", "Bearer "+service.Token)
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("VNC without control session: %d", response.Code)
	}
}

func openTestVNC(t *testing.T, ctx context.Context, base, id string, header http.Header, size ...int) net.Conn {
	t.Helper()
	var ws *websocket.Conn
	var err error
	for attempts := 0; attempts < 40; attempts++ {
		var response *http.Response
		ws, response, err = websocket.Dial(ctx, "ws"+strings.TrimPrefix(base, "http")+"/v1/sessions/"+id+"/vnc", &websocket.DialOptions{HTTPHeader: header})
		if err == nil || response == nil || response.StatusCode != http.StatusConflict {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	ws.SetReadLimit(8 << 20)
	conn := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	t.Cleanup(func() { conn.Close() })
	version := make([]byte, 12)
	if _, err = io.ReadFull(conn, version); err != nil {
		t.Fatal(err)
	}
	if string(version) != "RFB 003.008\n" {
		t.Fatalf("unexpected RFB version %q", version)
	}
	if _, err = conn.Write(version); err != nil {
		t.Fatal(err)
	}
	count := make([]byte, 1)
	if _, err = io.ReadFull(conn, count); err != nil || count[0] == 0 {
		t.Fatalf("invalid security types: %v", err)
	}
	types := make([]byte, int(count[0]))
	if _, err = io.ReadFull(conn, types); err != nil || !bytes.Contains(types, []byte{1}) {
		t.Fatalf("missing local socket security type: %v", err)
	}
	if _, err = conn.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	result := make([]byte, 4)
	if _, err = io.ReadFull(conn, result); err != nil || binary.BigEndian.Uint32(result) != 0 {
		t.Fatalf("RFB security failed: %v", err)
	}
	if _, err = conn.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	init := make([]byte, 24)
	if _, err = io.ReadFull(conn, init); err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint16(init[:2]) < 320 || binary.BigEndian.Uint16(init[2:4]) < 240 {
		t.Fatal("invalid RFB viewport")
	}
	if len(size) == 2 && (int(binary.BigEndian.Uint16(init[:2])) != size[0] || int(binary.BigEndian.Uint16(init[2:4])) != size[1]) {
		t.Fatalf("unexpected RFB viewport: %dx%d, want %dx%d", binary.BigEndian.Uint16(init[:2]), binary.BigEndian.Uint16(init[2:4]), size[0], size[1])
	}
	n := binary.BigEndian.Uint32(init[20:24])
	if n > 4096 {
		t.Fatal("invalid RFB desktop name")
	}
	if _, err = io.CopyN(io.Discard, conn, int64(n)); err != nil {
		t.Fatal(err)
	}
	return conn
}

func testFramebuffer(t *testing.T, conn net.Conn) {
	t.Helper()
	// Request a bounded raw rectangle to exercise the actual pixel transport.
	if _, err := conn.Write([]byte{2, 0, 0, 1, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte{3, 0, 0, 0, 0, 0, 1, 64, 0, 240}); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil || header[0] != 0 || binary.BigEndian.Uint16(header[2:]) == 0 || binary.BigEndian.Uint16(header[2:]) > 128 {
		t.Fatalf("invalid framebuffer update: %v %v", header, err)
	}
	nonzero := false
	for i := 0; i < int(binary.BigEndian.Uint16(header[2:])); i++ {
		rect := make([]byte, 12)
		if _, err := io.ReadFull(conn, rect); err != nil {
			t.Fatal(err)
		}
		x, y := binary.BigEndian.Uint16(rect[0:2]), binary.BigEndian.Uint16(rect[2:4])
		width, height := binary.BigEndian.Uint16(rect[4:6]), binary.BigEndian.Uint16(rect[6:8])
		if width == 0 || height == 0 || int(x)+int(width) > 320 || int(y)+int(height) > 240 || binary.BigEndian.Uint32(rect[8:]) != 0 {
			t.Fatalf("unexpected framebuffer rectangle: %v", rect)
		}
		pixels := make([]byte, int(width)*int(height)*4)
		if _, err := io.ReadFull(conn, pixels); err != nil {
			t.Fatal(err)
		}
		nonzero = nonzero || bytes.Count(pixels, []byte{0}) != len(pixels)
	}
	if !nonzero {
		t.Fatal("framebuffer did not contain page pixels")
	}
}

func TestServiceAuthenticationCapacityAndExpiry(t *testing.T) {
	executable, _ := os.Executable()
	service, err := NewService(executable, strings.Repeat("s", 48), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	for _, header := range []http.Header{{}, {"Authorization": {"Bearer wrong"}}, {"Authorization": {"Bearer " + service.Token}, "Origin": {"https://attacker.invalid"}}} {
		request := httptest.NewRequest("GET", "/healthz", nil)
		request.Header = header
		response := httptest.NewRecorder()
		service.ServeHTTP(response, request)
		if response.Code != 401 {
			t.Fatal("unauthorized service access", response.Code)
		}
	}
	server := httptest.NewServer(service)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func(id string) (*websocket.Conn, *http.Response, error) {
		return websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/sessions/"+strings.Repeat(id, 64), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + service.Token}}})
	}
	first, _, err := dial("a")
	if err != nil {
		t.Fatal(err)
	}
	defer first.CloseNow()
	if conn, response, err := dial("a"); err == nil {
		conn.CloseNow()
		t.Fatal("duplicate attachment")
	} else if response.StatusCode != 409 {
		t.Fatal(response.StatusCode)
	}
	second, _, err := dial("b")
	if err != nil {
		t.Fatal(err)
	}
	defer second.CloseNow()
	if conn, response, err := dial("c"); err == nil {
		conn.CloseNow()
		t.Fatal("unbounded sessions")
	} else if response.StatusCode != 429 {
		t.Fatal(response.StatusCode)
	}
	service.mu.Lock()
	retained := service.sessions[strings.Repeat("a", 64)]
	expired := service.sessions[strings.Repeat("b", 64)]
	expired.attached = false
	expired.expires = time.Now().Add(-time.Second)
	service.mu.Unlock()
	service.expire(time.Now())
	service.mu.Lock()
	count := len(service.sessions)
	service.mu.Unlock()
	if count != 1 || retained.browser.ctx.Err() != nil || expired.browser.ctx.Err() == nil {
		t.Fatal("expiry crossed active-session boundary")
	}
}
