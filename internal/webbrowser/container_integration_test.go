package webbrowser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Run inside the real service container, with its normal entrypoint and a
// read-only test binary mount. The loopback fixture is granted only by the test.
func TestContainerServiceRoundTrip(t *testing.T) {
	base := os.Getenv("KPANEL_TEST_SERVICE")
	if base == "" {
		t.Skip("set KPANEL_TEST_SERVICE to qualify an actual browser container")
	}
	tokenBytes, err := os.ReadFile("/run/secrets/browser-token")
	if err != nil {
		t.Fatal("test token file unavailable")
	}
	header := http.Header{"Authorization": {"Bearer " + strings.TrimSpace(string(tokenBytes))}}
	response, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("unauthenticated health endpoint accepted")
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<title>Container fixture</title><style>body{margin:0;height:4000px;background:linear-gradient(#eee,#29a)}input{position:absolute;left:20px;top:20px;width:300px;height:50px}</style><form onsubmit="event.preventDefault();document.title='Submitted:'+this.q.value"><input name="q" oninput="document.title='Input:'+this.value"></form><script>onscroll=()=>document.title='Scrolled:'+scrollY</script>`))
	}))
	defer app.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(base, "http")+"/v1/sessions/"+strings.Repeat("b", 64), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(MaxEventBytes)
	send := func(c Command) {
		t.Helper()
		data, _ := json.Marshal(c)
		if err := ws.Write(ctx, websocket.MessageText, data); err != nil {
			t.Fatal(err)
		}
	}
	defer send(Command{Type: "end"})
	wait := func(match func(Event) bool) Event {
		t.Helper()
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var event Event
			if err := json.Unmarshal(data, &event); err != nil {
				t.Fatal(err)
			}
			if match(event) {
				return event
			}
			if event.Type == "error" {
				t.Fatal(event.Code)
			}
		}
	}
	title := func(expected string) func(Event) bool {
		return func(e Event) bool {
			return e.Type == "state" && len(e.Tabs) > 0 && strings.HasPrefix(e.Tabs[0].Title, expected)
		}
	}
	send(Command{Type: "new", URL: app.URL})
	wait(title("Container fixture"))
	vnc := openTestVNC(t, ctx, base, strings.Repeat("b", 64), header)
	testFramebuffer(t, vnc)
	send(Command{Type: "mouse", Action: "mousePressed", Button: "left", X: 60, Y: 40})
	send(Command{Type: "mouse", Action: "mouseReleased", Button: "left", X: 60, Y: 40})
	send(Command{Type: "text", Text: "中文测试"})
	wait(title("Input:中文测试"))
	send(Command{Type: "key", Action: "keyDown", Key: "Enter", Code: "Enter"})
	send(Command{Type: "key", Action: "keyUp", Key: "Enter", Code: "Enter"})
	wait(title("Submitted:中文测试"))
	send(Command{Type: "mouse", Action: "mouseWheel", Button: "none", X: 500, Y: 300, DeltaY: 400})
	wait(title("Scrolled:"))
	for _, size := range [][2]int64{{1600, 1000}, {640, 480}, {1280, 720}} {
		t.Logf("waiting for viewport %dx%d", size[0], size[1])
		send(Command{Type: "resize", Width: size[0], Height: size[1]})
		wait(func(e Event) bool { return e.Type == "state" })
		if err := vnc.Close(); err != nil {
			t.Fatal(err)
		}
		vnc = openTestVNC(t, ctx, base, strings.Repeat("b", 64), header, int(size[0]), int(size[1]))
		testFramebuffer(t, vnc)
		t.Logf("received viewport %dx%d", size[0], size[1])
	}
	for i := 1; i < MaxTabs; i++ {
		send(Command{Type: "new", URL: app.URL})
	}
	wait(func(e Event) bool { return e.Type == "state" && len(e.Tabs) == MaxTabs })
	send(Command{Type: "new", URL: app.URL})
	wait(func(e Event) bool { return e.Type == "error" && e.Code == "tab_limit" })
	t.Log("container entrypoint, authenticated WS, VNC negotiation, Chinese, Enter, wheel, resize and eight-tab limit passed")
}
