//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/version"
	"github.com/kejilion/kejilion-panel/internal/windowsnode"
	"golang.org/x/sys/windows"
)

func startPlatformHealthPublisher(ctx context.Context) func() {
	child, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			health := contract.LightNodeHealth{ObservedAt: time.Now().UTC(), RuntimeVersion: version.Version, Update: contract.LightNodeUpdateHealth{State: "missing"}, Services: contract.LightNodeServicesHealth{Timer: windowsTaskHealth(), Telemetry: unknownServiceHealth(), Terminal: unknownServiceHealth(), File: unknownServiceHealth(), SSHLogin: unknownServiceHealth()}}
			content, err := json.Marshal(health)
			if err == nil {
				_ = windowsnode.WriteAtomic(windowsnode.RuntimePath("task-health.json"), content, windowsnode.TelemetryRead)
			}
			if !waitContext(child, 20*time.Second) {
				return
			}
		}
	}()
	return func() { cancel(); <-done }
}
func windowsTaskHealth() contract.LightNodeServiceHealth {
	state := unknownServiceHealth()
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return state
	}
	path := filepath.Join(system, "Tasks", "KPanel", "KejilionNodeUpdate")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		state.LoadState = "not-found"
		return state
	}
	if err != nil {
		return state
	}
	defer file.Close()
	if windowsnode.ValidateFile(file, windowsnode.ProgramRead) != nil {
		return state
	}
	content, err := io.ReadAll(io.LimitReader(file, 16385))
	if err != nil || len(content) > 16384 {
		return state
	}
	// Task Scheduler persists XML as UTF-16LE on some supported releases.
	if len(content) >= 2 && content[0] == 0xff && content[1] == 0xfe {
		units := make([]uint16, (len(content)-2)/2)
		for i := range units {
			units[i] = uint16(content[2+i*2]) | uint16(content[3+i*2])<<8
		}
		content = []byte(windows.UTF16ToString(units))
	}
	var task struct {
		Settings struct {
			Enabled *bool `xml:"Enabled"`
		} `xml:"Settings"`
		Actions struct {
			Exec struct {
				Command   string `xml:"Command"`
				Arguments string `xml:"Arguments"`
			} `xml:"Exec"`
		} `xml:"Actions"`
	}
	// Avoid encoding declarations contradicting the decoded UTF-8 input.
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	if decoder.Decode(&task) != nil {
		return state
	}
	if !strings.EqualFold(task.Actions.Exec.Command, filepath.Join(windowsnode.InstallDir(), "kejilion-node-bootstrap.exe")) || task.Actions.Exec.Arguments != "update" {
		return state
	}
	state.LoadState = "loaded"
	state.UnitFileState = "enabled"
	if task.Settings.Enabled != nil && !*task.Settings.Enabled {
		state.UnitFileState = "disabled"
		state.ActiveState = "inactive"
		state.SubState = "dead"
		return state
	}
	scheduler := windowsServiceHealth("Schedule")
	if scheduler.ActiveState == "active" {
		state.ActiveState = "active"
		state.SubState = "waiting"
	} else {
		state.ActiveState = scheduler.ActiveState
		state.SubState = scheduler.SubState
	}
	return state
}
