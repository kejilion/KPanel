package webbrowser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

type browserTab struct {
	Tab
	ctx              context.Context
	cancel           context.CancelFunc
	streaming        bool
	width, height    int64
	navigationCancel context.CancelFunc
	navigationID     uint64
	navigating       bool
}
type browserSession struct {
	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
	allocatorCancel context.CancelFunc
	root            context.Context
	rootCancel      context.CancelFunc
	proxy           *Egress
	proxyServer     *http.Server
	display         *visualDisplay
	tabs            []*browserTab
	active          string
	width, height   int64
	visible         bool
	nextID          int
	dirty           chan struct{}
	notices         chan Event
}

func call[P, R any](ctx context.Context, command cdp.Command[P, R], params P) (R, error) {
	return chromedp.Run(ctx, func(ctx context.Context, t *chromedp.Target) (R, error) { return cdp.Call(ctx, t, command, params) })
}
func send[P any](ctx context.Context, command cdp.Command[P, cdp.Empty], params P) error {
	_, err := call(ctx, command, params)
	if err != nil {
		return fmt.Errorf("%s: %w", command.Method, err)
	}
	return err
}

// Chromium requires both wheel deltas, including a zero on the unused axis.
// The generated optional fields omit zero values, so override their JSON tags.
type wheelEventParams struct {
	input.DispatchMouseEventParams
	DeltaX float64 `json:"deltaX"`
	DeltaY float64 `json:"deltaY"`
}

func browserCall[P, R any](ctx context.Context, command cdp.Command[P, R], params P) (R, error) {
	return cdp.Call(ctx, chromedp.FromContext(ctx).Browser, command, params)
}
func newBrowserSession(parent context.Context) *browserSession {
	ctx, cancel := context.WithCancel(parent)
	return &browserSession{ctx: ctx, cancel: cancel, width: 1280, height: 720, visible: true, dirty: make(chan struct{}, 1), notices: make(chan Event, 8)}
}
func (s *browserSession) signal() {
	select {
	case s.dirty <- struct{}{}:
	default:
	}
}
func (s *browserSession) notice(e Event) {
	select {
	case s.notices <- e:
	default:
	}
}

func (s *browserSession) Close() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopBrowser()
}

func (s *browserSession) stopBrowser() {
	if s.rootCancel != nil {
		s.rootCancel()
	}
	if s.allocatorCancel != nil {
		s.allocatorCancel()
	}
	if s.proxyServer != nil {
		_ = s.proxyServer.Close()
	}
	if s.proxy != nil {
		s.proxy.Close()
	}
	if s.display != nil {
		s.display.Close()
	}
	s.root = nil
	s.rootCancel = nil
	s.allocatorCancel = nil
	s.display = nil
	s.tabs = nil
	s.active = ""
}
func (s *browserSession) start(executable string, private []netip.Prefix) error {
	if s.root != nil {
		return nil
	}
	display, err := startVisualDisplay(s.ctx, s.width, s.height)
	if err != nil {
		return errors.New("browser_start_failed")
	}
	s.display = display
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		s.stopBrowser()
		return errors.New("browser_start_failed")
	}
	s.proxy = NewEgress(private)
	s.proxyServer = &http.Server{Handler: s.proxy, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 30 * time.Minute, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 64 << 10}
	go func() { _ = s.proxyServer.Serve(listener) }()
	options := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	// A private kiosk window exposes only the page viewport to the framebuffer;
	// the KPanel toolbar and the structured input channel remain authoritative.
	options = append(options, chromedp.Flag("headless", false), chromedp.Flag("kiosk", true), chromedp.Flag("window-size", fmt.Sprintf("%d,%d", s.width, s.height)), chromedp.Flag("force-device-scale-factor", "1"), chromedp.Env("DISPLAY="+display.display, "XAUTHORITY="+display.auth))
	options = append(options, chromedp.Flag("enable-automation", false), chromedp.Flag("disable-infobars", true))
	options = append(options, chromedp.ExecPath(executable), chromedp.ProxyServer("http://"+listener.Addr().String()), chromedp.Flag("no-sandbox", false), chromedp.Flag("proxy-bypass-list", "<-loopback>"), chromedp.Flag("disable-quic", true), chromedp.Flag("force-webrtc-ip-handling-policy", "disable_non_proxied_udp"), chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1"), chromedp.Flag("disable-features", "Translate,BlinkGenPropertyTrees,MediaRouter"), chromedp.Flag("disable-popup-blocking", false), chromedp.Flag("disk-cache-size", "33554432"), chromedp.Flag("media-cache-size", "16777216"))
	allocator, stopAllocator := chromedp.NewExecAllocator(s.ctx, options...)
	s.allocatorCancel = stopAllocator
	root, stopRoot := chromedp.NewContext(allocator)
	// The initial context owns the process; a per-command timeout would kill it.
	if err := chromedp.Do(root); err != nil {
		stopRoot()
		stopAllocator()
		s.stopBrowser()
		return errors.New("browser_start_failed")
	}
	s.root, s.rootCancel = root, stopRoot
	ctx, cancel := context.WithTimeout(root, 5*time.Second)
	defer cancel()
	enabled := true
	if _, err := browserCall(ctx, browser.SetDownloadBehavior, browser.SetDownloadBehaviorParams{Behavior: browser.SetDownloadBehaviorBehaviorDeny, EventsEnabled: &enabled}); err != nil {
		s.stopBrowser()
		return errors.New("browser_start_failed")
	}
	s.listenBrowser()
	return nil
}

func (s *browserSession) listenBrowser() {
	root := s.root
	destroyed := chromedp.BrowserEvents(root, target.TargetDestroyed)
	go func() {
		for event, err := range destroyed {
			if err != nil {
				return
			}
			s.mu.Lock()
			if s.root != root {
				s.mu.Unlock()
				return
			}
			for i, tab := range s.tabs {
				if chromedp.FromContext(tab.ctx).Target.TargetID != event.TargetID {
					continue
				}
				s.tabs = append(s.tabs[:i], s.tabs[i+1:]...)
				tab.cancel()
				if s.active == tab.ID {
					s.active = ""
					if len(s.tabs) > 0 {
						s.active = s.tabs[len(s.tabs)-1].ID
					}
				}
				if len(s.tabs) == 0 {
					s.stopBrowser()
				} else {
					_ = s.stream()
				}
				break
			}
			s.mu.Unlock()
			s.signal()
		}
	}()
	created := chromedp.BrowserEvents(root, target.TargetCreated)
	go func() {
		for event, err := range created {
			if err != nil {
				return
			}
			info := event.TargetInfo
			if info.Type != "page" || info.OpenerID == "" {
				continue
			}
			s.mu.Lock()
			if len(s.tabs) >= MaxTabs || s.root != root {
				ctx, cancel := context.WithTimeout(root, 2*time.Second)
				_, _ = browserCall(ctx, target.CloseTarget, target.CloseTargetParams{TargetID: info.TargetID})
				cancel()
				s.mu.Unlock()
				s.notice(Event{Type: "error", Code: "tab_limit"})
				continue
			}
			s.nextID++
			ctx, cancel := chromedp.NewContext(root, chromedp.WithTargetID(info.TargetID))
			tab := &browserTab{Tab: Tab{ID: fmt.Sprintf("tab-%d", s.nextID), URL: info.URL, Title: info.Title}, ctx: ctx, cancel: cancel}
			if chromedp.Do(ctx) != nil {
				cancel()
				s.mu.Unlock()
				continue
			}
			s.listen(tab)
			s.tabs = append(s.tabs, tab)
			s.active = tab.ID
			_ = s.stream()
			s.mu.Unlock()
			s.signal()
		}
	}()
	downloads := chromedp.BrowserEvents(root, browser.DownloadWillBegin)
	go func() {
		for _, err := range downloads {
			if err != nil {
				return
			}
			s.notice(Event{Type: "error", Code: "download_unavailable"})
		}
	}()
}
func (s *browserSession) selected(id string) *browserTab {
	if id == "" {
		id = s.active
	}
	for _, tab := range s.tabs {
		if tab.ID == id {
			return tab
		}
	}
	return nil
}
func (s *browserSession) listen(tab *browserTab) {
	dialogs := chromedp.Events(tab.ctx, page.JavascriptDialogOpening)
	go func() {
		for event, err := range dialogs {
			if err != nil {
				return
			}
			text := event.Message
			if len(text) > 2048 {
				text = text[:2048]
			}
			s.notice(Event{Type: "dialog", Tab: tab.ID, Code: string(event.Type), Text: text})
		}
	}()
	files := chromedp.Events(tab.ctx, page.FileChooserOpened)
	go func() {
		for _, err := range files {
			if err != nil {
				return
			}
			s.notice(Event{Type: "error", Tab: tab.ID, Code: "upload_unavailable"})
		}
	}()
}
func (s *browserSession) stream() error {
	if s.visible && s.display != nil && (s.display.width != s.width || s.display.height != s.height) {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		err := s.display.resize(ctx, s.width, s.height)
		cancel()
		if err != nil {
			s.stopBrowser()
			return errors.New("browser_disconnected")
		}
	}
	for _, tab := range s.tabs {
		want := s.visible && tab.ID == s.active
		if want && (tab.width != s.width || tab.height != s.height) {
			ctx, cancel := context.WithTimeout(tab.ctx, 5*time.Second)
			err := send(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: s.width, Height: s.height, DeviceScaleFactor: 1})
			cancel()
			if err != nil {
				return errors.New("browser_disconnected")
			}
			tab.width, tab.height = s.width, s.height
		}
		if tab.streaming == want {
			continue
		}
		ctx, cancel := context.WithTimeout(tab.ctx, 5*time.Second)
		if want {
			id := chromedp.FromContext(tab.ctx).Target.TargetID
			window, err := browserCall(ctx, browser.GetWindowForTarget, browser.GetWindowForTargetParams{TargetID: id})
			if err == nil {
				_, err = browserCall(ctx, browser.SetWindowBounds, browser.SetWindowBoundsParams{WindowID: window.WindowID, Bounds: &browser.Bounds{WindowState: browser.WindowStateFullscreen}})
			}
			if err == nil {
				_, err = browserCall(ctx, target.ActivateTarget, target.ActivateTargetParams{TargetID: id})
			}
			if err != nil {
				cancel()
				return errors.New("browser_disconnected")
			}
		}
		cancel()
		tab.streaming = want
	}
	return s.ctx.Err()
}

func (s *browserSession) Execute(c Command, executable string, private []netip.Prefix) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.Type != "mouse" || (c.Action != "mouseMoved" && c.Action != "mouseWheel") {
		defer s.signal()
	}
	if s.root != nil && s.root.Err() != nil {
		s.stopBrowser()
	}
	if c.Type == "visible" {
		s.visible = c.Visible
		if s.root == nil {
			return nil
		}
		return s.stream()
	}
	if c.Type == "resize" {
		s.width, s.height = c.Width, c.Height
		if s.root == nil {
			return nil
		}
	}
	if c.Type == "new" {
		if len(s.tabs) >= MaxTabs {
			return errors.New("tab_limit")
		}
		if err := s.start(executable, private); err != nil {
			return err
		}
		s.nextID++
		ctx, cancel := chromedp.NewContext(s.root)
		tab := &browserTab{Tab: Tab{ID: fmt.Sprintf("tab-%d", s.nextID), URL: "about:blank"}, ctx: ctx, cancel: cancel}
		if err := chromedp.Do(ctx); err != nil {
			cancel()
			return errors.New("browser_start_failed")
		}
		s.listen(tab)
		run, stop := context.WithTimeout(ctx, 5*time.Second)
		err := send(run, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: s.width, Height: s.height, DeviceScaleFactor: 1})
		if err == nil {
			err = send(run, page.SetInterceptFileChooserDialog, page.SetInterceptFileChooserDialogParams{Enabled: true})
		}
		stop()
		if err != nil {
			cancel()
			return errors.New("browser_start_failed")
		}
		tab.width, tab.height = s.width, s.height
		s.tabs = append(s.tabs, tab)
		s.active = tab.ID
		if c.URL != "" {
			c.Type = "navigate"
			c.Tab = tab.ID
		} else {
			return s.stream()
		}
	}
	tab := s.selected(c.Tab)
	if tab == nil {
		return errors.New("tab_unavailable")
	}
	ctx, stop := context.WithTimeout(tab.ctx, 10*time.Second)
	defer stop()
	var err error
	switch c.Type {
	case "navigate":
		if _, err := WebURL(c.URL); err != nil {
			return err
		}
		tab.Loading = true
		// Start from the active blank page before navigation can detach it.
		// Navigation waits outside the command channel, so Stop/Close remain usable.
		if err = s.stream(); err != nil {
			return err
		}
		if tab.navigationCancel != nil {
			tab.navigationCancel()
		}
		tab.navigationID++
		generation := tab.navigationID
		navigation, cancel := context.WithTimeout(tab.ctx, 30*time.Second)
		tab.navigationCancel = cancel
		tab.navigating = true
		tab.URL = c.URL
		go func() {
			err := chromedp.Do(navigation, chromedp.Navigate(c.URL))
			if err == nil {
				// Force the first compositor frame before reporting navigation complete;
				// headed Chromium can otherwise accept input before hit testing is ready.
				paint, done := context.WithTimeout(navigation, time.Second)
				_, _ = chromedp.Run(paint, chromedp.CaptureScreenshot())
				done()
			}
			cancel()
			s.mu.Lock()
			if tab.navigationID == generation {
				tab.navigationCancel = nil
				tab.navigating = false
				tab.Loading = false
				if err != nil && !errors.Is(err, context.Canceled) && tab.ctx.Err() == nil {
					s.notice(Event{Type: "error", Code: "browser_action_failed"})
				}
			}
			s.mu.Unlock()
			s.signal()
		}()
		return nil
	case "select":
		s.active = tab.ID
	case "close":
		tab.cancel()
		for i, t := range s.tabs {
			if t == tab {
				s.tabs = append(s.tabs[:i], s.tabs[i+1:]...)
				break
			}
		}
		if s.active == tab.ID {
			s.active = ""
			if len(s.tabs) > 0 {
				s.active = s.tabs[len(s.tabs)-1].ID
			}
		}
		if len(s.tabs) == 0 {
			s.stopBrowser()
			return nil
		}
	case "back", "forward":
		history, e := call(ctx, page.GetNavigationHistory, cdp.Empty{})
		err = e
		if e == nil {
			index := history.CurrentIndex
			if c.Type == "back" {
				index--
			} else {
				index++
			}
			if index >= 0 && index < int64(len(history.Entries)) {
				err = send(ctx, page.NavigateToHistoryEntry, page.NavigateToHistoryEntryParams{EntryID: history.Entries[index].ID})
			}
		}
	case "reload":
		err = send(ctx, page.Reload, page.ReloadParams{})
	case "stop":
		if tab.navigationCancel != nil {
			tab.navigationCancel()
		}
		err = send(ctx, page.StopLoading, cdp.Empty{})
		tab.Loading = false
	case "resize":
		// stream synchronizes the active target without restarting capture.
	case "text":
		err = send(ctx, input.InsertText, input.InsertTextParams{Text: c.Text})
	case "key":
		vk := map[string]int64{"Enter": 13, "Tab": 9, "Backspace": 8, "Delete": 46, "Escape": 27, "ArrowLeft": 37, "ArrowUp": 38, "ArrowRight": 39, "ArrowDown": 40, "Home": 36, "End": 35, "PageUp": 33, "PageDown": 34}[c.Key]
		if len(c.Key) == 1 && c.Key[0] >= 'a' && c.Key[0] <= 'z' {
			vk = int64(c.Key[0] - 'a' + 'A')
		}
		p := input.DispatchKeyEventParams{Type: input.DispatchKeyEventType(c.Action), Key: c.Key, Code: c.Code, Modifiers: c.Modifiers, WindowsVirtualKeyCode: vk}
		// Enter's character event drives native form submission and newline insertion.
		if c.Action == "keyDown" && c.Key == "Enter" && c.Modifiers&7 == 0 {
			p.Text = "\r"
			p.UnmodifiedText = "\r"
		}
		err = send(ctx, input.DispatchKeyEvent, p)
	case "mouse":
		if c.X < 0 || c.Y < 0 || c.X > float64(s.width) || c.Y > float64(s.height) {
			return errors.New("invalid_coordinates")
		}
		p := input.DispatchMouseEventParams{Type: input.DispatchMouseEventType(c.Action), X: c.X, Y: c.Y, Modifiers: c.Modifiers, DeltaX: c.DeltaX, DeltaY: c.DeltaY, Button: input.MouseButton(c.Button)}
		if c.Action == "mousePressed" || c.Action == "mouseReleased" {
			p.ClickCount = 1
		}
		if c.Action == "mouseWheel" {
			err = send(ctx, cdp.Command[wheelEventParams, cdp.Empty]{Method: input.CommandDispatchMouseEvent}, wheelEventParams{DispatchMouseEventParams: p, DeltaX: c.DeltaX, DeltaY: c.DeltaY})
		} else {
			err = send(ctx, input.DispatchMouseEvent, p)
		}
	case "dialog":
		err = send(ctx, page.HandleJavaScriptDialog, page.HandleJavaScriptDialogParams{Accept: c.Accept, PromptText: c.Text})
	default:
		return errors.New("invalid_command")
	}
	if err != nil {
		return errors.New("browser_action_failed")
	}
	return s.stream()
}
func (s *browserSession) Snapshot() Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := Event{Type: "state", Active: s.active, Tabs: make([]Tab, 0, len(s.tabs))}
	for _, tab := range s.tabs {
		ctx, cancel := context.WithTimeout(tab.ctx, 2*time.Second)
		history, err := call(ctx, page.GetNavigationHistory, cdp.Empty{})
		if err == nil && history.CurrentIndex >= 0 && history.CurrentIndex < int64(len(history.Entries)) {
			index := history.CurrentIndex
			entry := history.Entries[index]
			if !tab.navigating {
				tab.URL = entry.URL
			}
			tab.Title = entry.Title
			if len(tab.Title) > 160 {
				tab.Title = tab.Title[:160]
			}
			if len(tab.URL) > 4096 {
				tab.URL = tab.URL[:4096]
			}
			tab.Back = index > 0
			tab.Forward = index+1 < int64(len(history.Entries))
		}
		if title, e := chromedp.Run(ctx, chromedp.Title()); e == nil && len(title) <= 160 {
			tab.Title = title
		}
		if !tab.navigating {
			if loading, e := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.readyState !== 'complete'`)); e == nil {
				tab.Loading = loading
			}
		}
		cancel()
		event.Tabs = append(event.Tabs, tab.Tab)
	}
	return event
}
