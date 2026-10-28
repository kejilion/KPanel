package webbrowser

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/coder/websocket"
)

func TestRealBrowserEnterSubmitsAndInsertsNewline(t *testing.T) {
	executable := os.Getenv("KPANEL_TEST_CHROME")
	if executable == "" {
		t.Skip("set KPANEL_TEST_CHROME")
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>Input</title><form onsubmit="event.preventDefault();document.title='Submitted:'+this.query.value"><input name="query" style="position:absolute;left:20px;top:20px;width:300px;height:50px"></form><textarea style="position:absolute;left:20px;top:100px;width:300px;height:100px"></textarea>`))
	}))
	defer app.Close()
	s := newBrowserSession(context.Background())
	defer s.Close()
	execute := func(c Command) {
		t.Helper()
		if err := s.Execute(c, executable, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}); err != nil {
			t.Fatal(err)
		}
	}
	execute(Command{Type: "new", URL: app.URL})
	deadline := time.Now().Add(8 * time.Second)
	for (s.Snapshot().Tabs[0].Title != "Input" || s.Snapshot().Tabs[0].Loading) && time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
	}
	click := func(y float64) {
		execute(Command{Type: "mouse", Action: "mousePressed", Button: "left", X: 60, Y: y})
		execute(Command{Type: "mouse", Action: "mouseReleased", Button: "left", X: 60, Y: y})
	}
	enter := func() {
		execute(Command{Type: "key", Action: "keyDown", Key: "Enter", Code: "Enter"})
		execute(Command{Type: "key", Action: "keyUp", Key: "Enter", Code: "Enter"})
	}
	click(40)
	execute(Command{Type: "text", Text: "query"})
	enter()
	deadline = time.Now().Add(2 * time.Second)
	for s.Snapshot().Tabs[0].Title != "Submitted:query" && time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
	}
	if s.Snapshot().Tabs[0].Title != "Submitted:query" {
		state, _ := chromedp.Run(s.selected(s.active).ctx, chromedp.Evaluate[string](`JSON.stringify({title:document.title,active:document.activeElement?.outerHTML,value:document.querySelector('input')?.value,ready:document.readyState})`))
		t.Fatalf("Enter did not submit the form: %s", state)
	}
	click(130)
	execute(Command{Type: "text", Text: "first"})
	enter()
	execute(Command{Type: "text", Text: "second"})
	value, err := chromedp.Run(s.selected(s.active).ctx, chromedp.Evaluate[string](`document.querySelector('textarea').value`))
	if err != nil || value != "first\nsecond" {
		t.Fatalf("Enter did not insert one newline: %q %v", value, err)
	}
}

func TestRealBrowserWindowManagerFailureEndsSession(t *testing.T) {
	executable := os.Getenv("KPANEL_TEST_CHROME")
	if executable == "" {
		t.Skip("set KPANEL_TEST_CHROME")
	}
	s := newBrowserSession(context.Background())
	defer s.Close()
	if err := s.Execute(Command{Type: "new"}, executable, nil); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	manager := s.display.wm.Process
	s.mu.Unlock()
	if err := manager.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-s.notices:
		if event.Code != "browser_disconnected" {
			t.Fatalf("unexpected notice: %v", event)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("window manager failure did not end the session")
	}
	if state := s.Snapshot(); len(state.Tabs) != 0 {
		t.Fatal("window manager failure retained browser tabs")
	}
}

func TestRealBrowserActivationFailureEndsSession(t *testing.T) {
	executable := os.Getenv("KPANEL_TEST_CHROME")
	if executable == "" {
		t.Skip("set KPANEL_TEST_CHROME")
	}
	s := newBrowserSession(context.Background())
	defer s.Close()
	if err := s.Execute(Command{Type: "new"}, executable, nil); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	tab := s.selected(s.active)
	tab.cancel()
	tab.streaming = false
	err := s.stream()
	retained := s.root != nil || len(s.tabs) != 0
	s.mu.Unlock()
	if err == nil || retained {
		t.Fatalf("failed activation retained workspace: err=%v retained=%v", err, retained)
	}
}

func TestRealBrowserWheelAndViewport(t *testing.T) {
	executable := os.Getenv("KPANEL_TEST_CHROME")
	if executable == "" {
		t.Skip("set KPANEL_TEST_CHROME")
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<title>Scroll</title><style>body{margin:0;width:3000px;height:4000px;background:linear-gradient(red,blue)}</style>`))
	}))
	defer app.Close()
	s := newBrowserSession(context.Background())
	defer s.Close()
	ranges := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	execute := func(c Command) {
		t.Helper()
		if err := s.Execute(c, executable, ranges); err != nil {
			t.Fatal(err)
		}
	}
	execute(Command{Type: "resize", Width: 1088, Height: 560})
	execute(Command{Type: "new", URL: app.URL})
	deadline := time.Now().Add(8 * time.Second)
	for s.Snapshot().Tabs[0].Loading && time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
	}
	first := s.active
	tab := s.selected(first)
	for _, delta := range [][2]float64{{0, 400}, {300, 0}, {0, -200}, {-150, 0}} {
		before, err := chromedp.Run(tab.ctx, chromedp.Evaluate[[2]float64](`[scrollX,scrollY]`))
		if err != nil {
			t.Fatal(err)
		}
		var after [2]float64

		execute(Command{Type: "mouse", Action: "mouseWheel", Button: "none", X: 200, Y: 200, DeltaX: delta[0], DeltaY: delta[1]})
		deadline = time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			after, err = chromedp.Run(tab.ctx, chromedp.Evaluate[[2]float64](`[scrollX,scrollY]`))
			if err != nil {
				t.Fatal(err)
			}
			if after != before {
				break
			}
			time.Sleep(30 * time.Millisecond)
		}
		if after == before {
			t.Fatalf("wheel %v did not move page: %v", delta, after)
		}
	}
	execute(Command{Type: "new", URL: app.URL})
	execute(Command{Type: "resize", Width: 1200, Height: 700})
	deadline = time.Now().Add(8 * time.Second)
	for s.Snapshot().Tabs[1].Loading && time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
	}
	execute(Command{Type: "resize", Width: 1400, Height: 800})
	execute(Command{Type: "select", Tab: first})
	size, err := chromedp.Run(tab.ctx, chromedp.Evaluate[[2]int](`[innerWidth,innerHeight]`))
	if err != nil {
		t.Fatal(err)
	}
	if size != [2]int{1400, 800} {
		t.Fatalf("selected tab kept stale viewport: %v", size)
	}
	capture, err := chromedp.Run(tab.ctx, chromedp.CaptureScreenshot())
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.DecodeConfig(bytes.NewReader(capture))
	if err != nil || image.Width != 1400 || image.Height != 800 {
		t.Fatalf("invalid selected tab viewport: %v %v", image, err)
	}
}

func TestRealBrowserNavigationCanStopAndPopupSelfClose(t *testing.T) {
	executable := os.Getenv("KPANEL_TEST_CHROME")
	if executable == "" {
		t.Skip("set KPANEL_TEST_CHROME")
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/hung":
			<-r.Context().Done()
		case "/slow":
			_, _ = w.Write([]byte(`<title>Slow</title><img src="/hung">`))
			w.(http.Flusher).Flush()
		case "/popup":
			_, _ = w.Write([]byte(`<title>Popup</title><script>setTimeout(()=>window.close(),1000)</script>`))
		default:
			_, _ = w.Write([]byte(`<title>Main</title><button style="position:absolute;left:0;top:0;width:200px;height:70px" onclick="window.open('/popup')">Popup</button>`))
		}
	}))
	defer app.Close()
	s := newBrowserSession(context.Background())
	defer s.Close()
	ranges := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	execute := func(c Command) {
		t.Helper()
		if err := s.Execute(c, executable, ranges); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(match func(Event) bool) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			if match(s.Snapshot()) {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatal("browser condition not reached")
	}
	execute(Command{Type: "new", URL: app.URL + "/slow"})
	wait(func(e Event) bool { return len(e.Tabs) == 1 && e.Tabs[0].Title == "Slow" && e.Tabs[0].Loading })
	started := time.Now()
	execute(Command{Type: "stop"})
	if time.Since(started) > 3*time.Second {
		t.Fatal("stop was blocked by navigation")
	}
	execute(Command{Type: "navigate", URL: app.URL})
	wait(func(e Event) bool { return len(e.Tabs) == 1 && e.Tabs[0].Title == "Main" && !e.Tabs[0].Loading })
	execute(Command{Type: "mouse", Action: "mousePressed", Button: "left", X: 50, Y: 30})
	execute(Command{Type: "mouse", Action: "mouseReleased", Button: "left", X: 50, Y: 30})
	wait(func(e Event) bool { return len(e.Tabs) == 2 })
	wait(func(e Event) bool { return len(e.Tabs) == 1 && e.Active == e.Tabs[0].ID })
	execute(Command{Type: "new", URL: app.URL})
	wait(func(e Event) bool { return len(e.Tabs) == 2 && e.Tabs[1].Title == "Main" })
	active := s.Snapshot().Active
	s.mu.Lock()
	var activeContext context.Context
	for _, tab := range s.tabs {
		if tab.ID == active {
			activeContext = tab.ctx
		}
	}
	s.mu.Unlock()
	if activeContext == nil {
		t.Fatal("missing active tab")
	}
	capture, err := chromedp.Run(activeContext, chromedp.CaptureScreenshot())
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(capture))
	if err != nil || image.Bounds().Dx() < 320 {
		t.Fatal("invalid new tab capture", err)
	}
}

func TestRealBrowserRoundTrip(t *testing.T) {
	executable := os.Getenv("KPANEL_TEST_CHROME")
	if executable == "" {
		t.Skip("set KPANEL_TEST_CHROME to an isolated test browser executable")
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		color := "17"
		if r.URL.Path == "/second" {
			color = "61"
		}
		_, _ = w.Write([]byte(`<!doctype html><title>Fixture</title><style>body{margin:0;background:rgb(` + color + `,` + color + `,` + color + `)}input{position:absolute;left:20px;top:20px;width:300px;height:40px}</style><input oninput="document.title=this.value"><a style="position:absolute;top:100px" href="/next">Next</a>`))
	}))
	defer app.Close()
	service, err := NewService(executable, strings.Repeat("t", 48), []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := httptest.NewServer(service)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/sessions/"+strings.Repeat("a", 64), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + service.Token}}})
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(MaxEventBytes)
	sendCommand := func(c Command) {
		t.Helper()
		data, _ := json.Marshal(c)
		if ws.Write(ctx, websocket.MessageText, data) != nil {
			t.Fatal("write failed")
		}
	}
	readUntil := func(match func(Event) bool) Event {
		t.Helper()
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var event Event
			if json.Unmarshal(data, &event) != nil {
				t.Fatal("invalid event")
			}
			if event.Type == "error" {
				t.Fatal(event.Code)
			}
			if match(event) {
				return event
			}
		}
	}
	readUntil(func(e Event) bool { return e.Type == "state" })
	sendCommand(Command{Type: "new", URL: app.URL})
	state := readUntil(func(e Event) bool {
		return e.Type == "state" && len(e.Tabs) > 0 && e.Tabs[0].Title == "Fixture" && !e.Tabs[0].Loading
	})
	vnc := openTestVNC(t, ctx, server.URL, strings.Repeat("a", 64), http.Header{"Authorization": {"Bearer " + service.Token}})
	testFramebuffer(t, vnc)
	waitTestGray(t, vnc, 17)
	first := state.Active
	sendCommand(Command{Type: "new", URL: app.URL + "/second"})
	secondState := readUntil(func(e Event) bool {
		return e.Type == "state" && len(e.Tabs) == 2 && e.Active != first && !e.Tabs[1].Loading
	})
	waitTestGray(t, vnc, 61)
	sendCommand(Command{Type: "select", Tab: first})
	readUntil(func(e Event) bool { return e.Type == "state" && e.Active == first })
	waitTestGray(t, vnc, 17)
	sendCommand(Command{Type: "close", Tab: secondState.Active})
	readUntil(func(e Event) bool { return e.Type == "state" && len(e.Tabs) == 1 && e.Active == first })
	// Raw VNC keyboard and pointer messages must never control the browser.
	for _, packet := range [][]byte{{5, 1, 0, 60, 0, 40}, {5, 0, 0, 60, 0, 40}, {4, 1, 0, 0, 0, 0, 0, 97}, {4, 0, 0, 0, 0, 0, 0, 97}} {
		if _, err := vnc.Write(packet); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	service.mu.Lock()
	actual := service.sessions[strings.Repeat("a", 64)].browser
	service.mu.Unlock()
	actual.mu.Lock()
	activeContext := actual.selected(actual.active).ctx
	actual.mu.Unlock()
	focus, err := chromedp.Run(activeContext, chromedp.Evaluate[string](`document.activeElement.tagName`))
	if err != nil || focus != "BODY" {
		t.Fatalf("VNC pointer or keyboard changed browser focus: %q %v", focus, err)
	}
	id := state.Active
	sendCommand(Command{Type: "mouse", Tab: id, Action: "mousePressed", Button: "left", X: 60, Y: 40})
	sendCommand(Command{Type: "mouse", Tab: id, Action: "mouseReleased", Button: "left", X: 60, Y: 40})
	sendCommand(Command{Type: "text", Tab: id, Text: "中文测试"})
	readUntil(func(e Event) bool { return e.Type == "state" && len(e.Tabs) > 0 && e.Tabs[0].Title == "中文测试" })
	sendCommand(Command{Type: "navigate", Tab: id, URL: app.URL + "/next"})
	readUntil(func(e Event) bool {
		return e.Type == "state" && len(e.Tabs) > 0 && strings.HasSuffix(e.Tabs[0].URL, "/next") && e.Tabs[0].Back
	})
	sendCommand(Command{Type: "back", Tab: id})
	readUntil(func(e Event) bool {
		return e.Type == "state" && len(e.Tabs) > 0 && !strings.HasSuffix(e.Tabs[0].URL, "/next") && e.Tabs[0].Forward
	})
	sendCommand(Command{Type: "resize", Width: 640, Height: 480})
	readUntil(func(e Event) bool { return e.Type == "state" })
	sendCommand(Command{Type: "close", Tab: id})
	readUntil(func(e Event) bool { return e.Type == "state" && len(e.Tabs) == 0 })
	service.mu.Lock()
	entry := service.sessions[strings.Repeat("a", 64)]
	service.mu.Unlock()
	entry.browser.mu.Lock()
	running := entry.browser.root != nil
	entry.browser.mu.Unlock()
	if running {
		t.Fatal("last tab retained browser process")
	}
	for i := 0; i < 8; i++ {
		sendCommand(Command{Type: "new", URL: app.URL})
	}
	state = readUntil(func(e Event) bool { return e.Type == "state" && len(e.Tabs) == 8 })
	if err := entry.browser.Execute(Command{Type: "new", URL: app.URL}, executable, nil); err == nil || err.Error() != "tab_limit" {
		t.Fatal("ninth tab did not respect the eight-tab limit", err)
	}
	sendCommand(Command{Type: "select", Tab: state.Tabs[0].ID})
	readUntil(func(e Event) bool { return e.Type == "state" && e.Active == state.Tabs[0].ID })
	sendCommand(Command{Type: "end"})
}
