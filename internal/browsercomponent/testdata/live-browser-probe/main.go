package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/coder/websocket"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	token, err := os.ReadFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	ws, _, err := websocket.Dial(ctx, "ws://"+os.Args[1]+":8092/v1/sessions/"+strings.Repeat("a", 64), &websocket.DialOptions{HTTPClient: &http.Client{Transport: &http.Transport{Proxy: nil}}, HTTPHeader: http.Header{"Authorization": {"Bearer " + string(token)}}})
	if err != nil {
		panic(err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(4 << 20)
	_, _, err = ws.Read(ctx)
	if err != nil {
		panic(err)
	}
	_ = ws.Write(ctx, websocket.MessageText, []byte(`{"type":"visible","visible":true}`))
	_ = ws.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","width":1280,"height":720}`))
	err = ws.Write(ctx, websocket.MessageText, []byte(`{"type":"new","url":"https://example.com"}`))
	if err != nil {
		panic(err)
	}
	title := false
	for !title {
		_, data, err := ws.Read(ctx)
		if err != nil {
			panic(err)
		}
		var event struct {
			Type  string
			Image string
			Code  string
			Tabs  []struct{ Title string }
		}
		if json.Unmarshal(data, &event) != nil {
			panic("invalid browser event")
		}
		if event.Type == "error" {
			panic("browser error: " + event.Code)
		}
		for _, tab := range event.Tabs {
			if tab.Title != "" {
				fmt.Println("probe title=" + tab.Title)
			}
			if tab.Title == "Example Domain" {
				title = true
			}
		}
	}
	vnc, _, err := websocket.Dial(ctx, "ws://"+os.Args[1]+":8092/v1/sessions/"+strings.Repeat("a", 64)+"/vnc", &websocket.DialOptions{HTTPClient: &http.Client{Transport: &http.Transport{Proxy: nil}}, HTTPHeader: http.Header{"Authorization": {"Bearer " + string(token)}}})
	if err != nil {
		panic(err)
	}
	defer vnc.CloseNow()
	vnc.SetReadLimit(8 << 20)
	stream := websocket.NetConn(ctx, vnc, websocket.MessageBinary)
	defer stream.Close()
	serverVersion := make([]byte, 12)
	if _, err = io.ReadFull(stream, serverVersion); err != nil || string(serverVersion) != "RFB 003.008\n" {
		panic("invalid RFB handshake")
	}
	if _, err = stream.Write(serverVersion); err != nil {
		panic(err)
	}
	securityCount := make([]byte, 1)
	if _, err = io.ReadFull(stream, securityCount); err != nil || securityCount[0] == 0 {
		panic("missing RFB security types")
	}
	securityTypes := make([]byte, int(securityCount[0]))
	if _, err = io.ReadFull(stream, securityTypes); err != nil || !bytes.Contains(securityTypes, []byte{1}) {
		panic("RFB private socket security negotiation failed")
	}
	if _, err = stream.Write([]byte{1}); err != nil {
		panic(err)
	}
	securityResult := make([]byte, 4)
	if _, err = io.ReadFull(stream, securityResult); err != nil || binary.BigEndian.Uint32(securityResult) != 0 {
		panic("RFB security negotiation failed")
	}
	if _, err = stream.Write([]byte{1}); err != nil {
		panic(err)
	}
	serverInit := make([]byte, 24)
	if _, err = io.ReadFull(stream, serverInit); err != nil || binary.BigEndian.Uint16(serverInit[:2]) < 320 || binary.BigEndian.Uint16(serverInit[2:4]) < 240 {
		panic("invalid RFB viewport")
	}
	bytesPerPixel := int(serverInit[4] / 8)
	if bytesPerPixel != 2 && bytesPerPixel != 4 {
		panic("unsupported RFB pixel format")
	}
	nameLength := binary.BigEndian.Uint32(serverInit[20:24])
	if nameLength > 4096 {
		panic("invalid RFB desktop name")
	}
	if _, err = io.CopyN(io.Discard, stream, int64(nameLength)); err != nil {
		panic(err)
	}
	if _, err = stream.Write([]byte{2, 0, 0, 1, 0, 0, 0, 0}); err != nil {
		panic(err)
	}
	if _, err = stream.Write([]byte{3, 0, 0, 0, 0, 0, 1, 64, 0, 240}); err != nil {
		panic(err)
	}
	update := make([]byte, 4)
	if _, err = io.ReadFull(stream, update); err != nil || update[0] != 0 {
		panic("missing RFB framebuffer update")
	}
	rectangles := binary.BigEndian.Uint16(update[2:4])
	if rectangles == 0 || rectangles > 128 {
		panic("invalid RFB framebuffer rectangles")
	}
	nonzero := false
	for i := 0; i < int(rectangles); i++ {
		rect := make([]byte, 12)
		if _, err = io.ReadFull(stream, rect); err != nil {
			panic(err)
		}
		x, y := binary.BigEndian.Uint16(rect[:2]), binary.BigEndian.Uint16(rect[2:4])
		width, height := binary.BigEndian.Uint16(rect[4:6]), binary.BigEndian.Uint16(rect[6:8])
		if width == 0 || height == 0 || int(x)+int(width) > 320 || int(y)+int(height) > 240 || binary.BigEndian.Uint32(rect[8:]) != 0 {
			panic("invalid RFB framebuffer rectangle")
		}
		pixels := make([]byte, int(width)*int(height)*bytesPerPixel)
		if _, err = io.ReadFull(stream, pixels); err != nil {
			panic(err)
		}
		nonzero = nonzero || bytes.Count(pixels, []byte{0}) != len(pixels)
	}
	if !nonzero {
		panic("RFB framebuffer did not contain page pixels")
	}
	_ = ws.Write(ctx, websocket.MessageText, []byte(`{"type":"end"}`))
	fmt.Println("live-browser=pass title=Example_Domain framebuffer=received authenticated_websocket=pass")
}
