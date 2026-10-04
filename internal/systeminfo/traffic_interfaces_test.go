package systeminfo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type trafficFixture struct {
	t        *testing.T
	procRoot string
	stateDir string
}

func newTrafficFixture(t *testing.T) *trafficFixture {
	t.Helper()
	fixture := &trafficFixture{t: t, procRoot: t.TempDir(), stateDir: t.TempDir()}
	for _, directory := range []string{"net", "sys/kernel/random"} {
		if err := os.MkdirAll(filepath.Join(fixture.procRoot, filepath.FromSlash(directory)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fixture.boot("boot-a")
	return fixture
}

func (f *trafficFixture) write(name, content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.procRoot, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *trafficFixture) boot(id string) { f.write("sys/kernel/random/boot_id", id+"\n") }

// counters writes /proc/net/dev; each value is "received/sent".
func (f *trafficFixture) counters(values map[string][2]uint64) {
	dev := "Inter-| Receive | Transmit\n face |bytes packets|bytes packets\n"
	for name, value := range values {
		dev += fmt.Sprintf(" %s: %d 0 0 0 0 0 0 0 %d 0 0 0 0 0 0 0\n", name, value[0], value[1])
	}
	f.write("net/dev", dev)
}

func (f *trafficFixture) routes(names ...string) {
	route := "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n"
	for _, name := range names {
		route += name + " 00000000 0100000A 0003 0 0 100 00000000 0 0 0\n"
	}
	f.write("net/route", route)
}

func (f *trafficFixture) collector(persist bool) *Collector {
	collector := &Collector{
		ProcRoot:             f.procRoot,
		TrafficSelectionPath: filepath.Join(f.stateDir, "traffic-interfaces.json"),
	}
	if persist {
		collector.TrafficStatePath = filepath.Join(f.stateDir, "traffic-continuity.json")
	}
	return collector
}

func readTraffic(t *testing.T, collector *Collector) (uint64, uint64) {
	t.Helper()
	var summary contract.NetworkSummary
	if err := collector.readNetwork(&summary); err != nil {
		t.Fatalf("readNetwork() error = %v", err)
	}
	return summary.ReceivedBytes, summary.SentBytes
}

func expectTraffic(t *testing.T, collector *Collector, received, sent uint64) {
	t.Helper()
	if gotReceived, gotSent := readTraffic(t, collector); gotReceived != received || gotSent != sent {
		t.Fatalf("traffic = %d/%d, want %d/%d", gotReceived, gotSent, received, sent)
	}
}

func TestClassifyTrafficInterfacesExplainsEachInterface(t *testing.T) {
	counters := []networkCounter{
		{name: "lo", received: 1, sent: 1},
		{name: "eth0", received: 100, sent: 200},
		{name: "warp", received: 1000, sent: 2000},
		{name: "docker0", received: 10, sent: 20},
	}
	cases := []struct {
		name      string
		routes    map[string]bool
		selection contract.TrafficInterfaceSelection
		received  uint64
		reasons   map[string]string
	}{
		{
			name: "default routes", routes: map[string]bool{"eth0": true}, received: 100,
			reasons: map[string]string{
				"lo": contract.TrafficInterfaceLoopback, "eth0": contract.TrafficInterfaceDefaultRoute,
				"warp": contract.TrafficInterfaceNotDefaultRoute, "docker0": contract.TrafficInterfaceNotDefaultRoute,
			},
		},
		{
			name: "no default route", received: 1100,
			reasons: map[string]string{
				"lo": contract.TrafficInterfaceLoopback, "eth0": contract.TrafficInterfaceAutomatic,
				"warp": contract.TrafficInterfaceAutomatic, "docker0": contract.TrafficInterfaceVirtual,
			},
		},
		{
			name: "include", routes: map[string]bool{"warp": true}, received: 100,
			selection: contract.TrafficInterfaceSelection{Include: []string{"eth0", "ppp0"}},
			reasons: map[string]string{
				"lo": contract.TrafficInterfaceLoopback, "eth0": contract.TrafficInterfaceSelected,
				"warp": contract.TrafficInterfaceNotSelected, "docker0": contract.TrafficInterfaceNotSelected,
				"ppp0": contract.TrafficInterfaceMissing,
			},
		},
		{
			name: "exclude", routes: map[string]bool{"eth0": true, "warp": true}, received: 100,
			selection: contract.TrafficInterfaceSelection{Exclude: []string{"warp"}},
			reasons: map[string]string{
				"lo": contract.TrafficInterfaceLoopback, "eth0": contract.TrafficInterfaceDefaultRoute,
				"warp": contract.TrafficInterfaceExcluded, "docker0": contract.TrafficInterfaceNotDefaultRoute,
			},
		},
		{
			name: "every included interface missing", routes: map[string]bool{"eth0": true}, received: 100,
			selection: contract.TrafficInterfaceSelection{Include: []string{"ppp0"}},
			reasons: map[string]string{
				"lo": contract.TrafficInterfaceLoopback, "eth0": contract.TrafficInterfaceDefaultRoute,
				"warp": contract.TrafficInterfaceNotDefaultRoute, "docker0": contract.TrafficInterfaceNotDefaultRoute,
				"ppp0": contract.TrafficInterfaceMissing,
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			statuses, received, _, _ := classifyNetworkInterfaces(counters, test.routes, test.selection)
			if received != test.received {
				t.Fatalf("received = %d, want %d", received, test.received)
			}
			reasons := map[string]string{}
			for index, status := range statuses {
				reasons[status.Name] = status.Reason
				if index > 0 && statuses[index-1].Name >= status.Name {
					t.Fatalf("statuses are not sorted: %#v", statuses)
				}
			}
			if !reflect.DeepEqual(reasons, test.reasons) {
				t.Fatalf("reasons = %#v, want %#v", reasons, test.reasons)
			}
		})
	}
}

func TestTrafficScopeIdentifiesCountedSetOnly(t *testing.T) {
	routes := map[string]bool{"eth0": true}
	_, _, _, first := classifyNetworkInterfaces([]networkCounter{{name: "eth0", received: 1}}, routes, contract.TrafficInterfaceSelection{})
	_, _, _, grown := classifyNetworkInterfaces([]networkCounter{{name: "eth0", received: 9}, {name: "docker0"}}, routes, contract.TrafficInterfaceSelection{})
	_, _, _, changed := classifyNetworkInterfaces([]networkCounter{{name: "eth0"}, {name: "wg0"}}, map[string]bool{"eth0": true, "wg0": true}, contract.TrafficInterfaceSelection{})
	if first != grown || first == changed {
		t.Fatalf("scope = %q/%q/%q, want only a counted-set change to alter it", first, grown, changed)
	}
}

func TestReadNetworkContinuesWhenTheCountedSetChanges(t *testing.T) {
	fixture := newTrafficFixture(t)
	collector := fixture.collector(false)
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1000, 2000}, "warp": {50000, 60000}})
	expectTraffic(t, collector, 1000, 2000)

	// WARP takes a default route: its earlier counter is not this period's traffic.
	fixture.routes("eth0", "warp")
	fixture.counters(map[string][2]uint64{"eth0": {1100, 2100}, "warp": {50000, 60000}})
	expectTraffic(t, collector, 1000, 2000)
	fixture.counters(map[string][2]uint64{"eth0": {1150, 2150}, "warp": {50070, 60080}})
	expectTraffic(t, collector, 1120, 2130)

	// Dropping WARP continues from the last value instead of rolling back.
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1200, 2200}, "warp": {50100, 60100}})
	expectTraffic(t, collector, 1120, 2130)
	fixture.counters(map[string][2]uint64{"eth0": {1300, 2250}, "warp": {50100, 60100}})
	expectTraffic(t, collector, 1220, 2180)
}

func TestReadNetworkFollowsTheSelectionFile(t *testing.T) {
	fixture := newTrafficFixture(t)
	collector := fixture.collector(false)
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1000, 2000}, "eth1": {500, 700}})
	expectTraffic(t, collector, 1000, 2000)

	selection := contract.TrafficInterfaceSelection{Include: []string{"eth0", "eth1"}}
	if err := SaveTrafficSelection(collector.TrafficSelectionPath, selection, 0o600, -1); err != nil {
		t.Fatal(err)
	}
	// Adding eth1 continues from the last value, then counts both.
	fixture.counters(map[string][2]uint64{"eth0": {1010, 2010}, "eth1": {520, 730}})
	expectTraffic(t, collector, 1000, 2000)
	fixture.counters(map[string][2]uint64{"eth0": {1020, 2020}, "eth1": {530, 740}})
	expectTraffic(t, collector, 1020, 2020)
}

func TestTrafficContinuitySurvivesRestartAndResetsOnReboot(t *testing.T) {
	fixture := newTrafficFixture(t)
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1000, 2000}, "warp": {500, 600}})
	first := fixture.collector(true)
	expectTraffic(t, first, 1000, 2000)
	fixture.routes("eth0", "warp")
	expectTraffic(t, first, 1000, 2000)

	// Same boot, same counted set: the stored offset still applies.
	fixture.counters(map[string][2]uint64{"eth0": {1100, 2100}, "warp": {500, 600}})
	expectTraffic(t, fixture.collector(true), 1100, 2100)

	// Same boot, set changed while stopped: continue from the stored value.
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1200, 2200}, "warp": {500, 600}})
	expectTraffic(t, fixture.collector(true), 1100, 2100)

	// A reboot starts over; the center recognizes it by the uptime rollback.
	fixture.boot("boot-b")
	fixture.counters(map[string][2]uint64{"eth0": {10, 20}, "warp": {1, 2}})
	expectTraffic(t, fixture.collector(true), 10, 20)
}

func TestTrafficContinuityIgnoresAnotherBootsState(t *testing.T) {
	fixture := newTrafficFixture(t)
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1000, 2000}})
	path := filepath.Join(fixture.stateDir, "traffic-continuity.json")
	stale, err := json.Marshal(networkContinuity{
		SchemaVersion: networkContinuitySchemaVersion, BootID: "boot-old", Scope: "other",
		OffsetReceived: 900, OffsetSent: 900, LastReceived: 5000, LastSent: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	expectTraffic(t, fixture.collector(true), 1000, 2000)
	for _, content := range []string{`{"schemaVersion":1,"bootId":"boot-a","scope":"x","extra":1}`, `{"schemaVersion":2}`, `not json`} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok := loadNetworkContinuity(path); ok {
			t.Fatalf("loadNetworkContinuity(%q) accepted invalid state", content)
		}
	}
}

func TestTrafficContinuityClampsAtZero(t *testing.T) {
	fixture := newTrafficFixture(t)
	collector := fixture.collector(false)
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {100, 100}, "wg0": {5000, 5000}})
	expectTraffic(t, collector, 100, 100)
	fixture.routes("eth0", "wg0")
	expectTraffic(t, collector, 100, 100)
	// wg0 was recreated: its counter restarts below the carried offset.
	fixture.counters(map[string][2]uint64{"eth0": {110, 110}, "wg0": {0, 0}})
	expectTraffic(t, collector, 0, 0)
}

func TestTrafficContinuityPersistsCounterRollback(t *testing.T) {
	fixture := newTrafficFixture(t)
	now := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	collector := fixture.collector(true)
	collector.Now = func() time.Time { return now }
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1000, 2000}})
	expectTraffic(t, collector, 1000, 2000)
	fixture.counters(map[string][2]uint64{"eth0": {10, 20}})
	expectTraffic(t, collector, 10, 20)
	stored, ok := loadNetworkContinuity(collector.TrafficStatePath)
	if !ok || stored.LastReceived != 10 || stored.LastSent != 20 {
		t.Fatalf("stored continuity = %#v, %v; want the rolled-back counters saved at once", stored, ok)
	}
}

func TestLoadTrafficSelectionValidatesTheFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "traffic-interfaces.json")
	selection, content, err := LoadTrafficSelection(path)
	if err != nil || content != nil || len(selection.Include) != 0 || selection.Include == nil {
		t.Fatalf("missing file = %#v, %q, %v; want the automatic choice", selection, content, err)
	}
	for _, invalid := range []string{
		`{"schemaVersion":1,"include":["eth0"],"unknown":true}`,
		`{"schemaVersion":2,"include":["eth0"]}`,
		`{"schemaVersion":1,"include":["eth0"]} {}`,
		`{"schemaVersion":1,"include":["eth0"],"exclude":["eth0"]}`,
		`{"schemaVersion":1,"include":["lo"]}`,
		`{"schemaVersion":1,"include":["../etc"]}`,
		`{"schemaVersion":1,"include":["` + strings.Repeat("a", 16) + `"]}`,
		strings.Repeat(" ", networkSelectionFileLimit+1),
	} {
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadTrafficSelection(path); err == nil {
			t.Fatalf("LoadTrafficSelection(%.60q) accepted an invalid file", invalid)
		}
	}
	target := filepath.Join(directory, "target.json")
	if err := os.WriteFile(target, []byte(`{"schemaVersion":1,"include":["eth0"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err == nil {
		if _, _, err := LoadTrafficSelection(path); err == nil {
			t.Fatal("LoadTrafficSelection() followed a symbolic link")
		}
	}
}

func TestTrafficInterfacesFallsBackWhenTheSelectionIsUnreadable(t *testing.T) {
	fixture := newTrafficFixture(t)
	collector := fixture.collector(false)
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1000, 2000}, "eth1": {5, 5}})
	if err := os.WriteFile(collector.TrafficSelectionPath, []byte(`{"schemaVersion":1,"include":["eth1"],"x":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	expectTraffic(t, collector, 1000, 2000)
	snapshot, err := collector.TrafficInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SelectionError == "" || len(snapshot.Selection.Include) != 0 {
		t.Fatalf("snapshot = %#v, want the automatic choice with a selection error", snapshot)
	}
}

func TestReplaceTrafficInterfacesChecksTheVersion(t *testing.T) {
	fixture := newTrafficFixture(t)
	collector := fixture.collector(true)
	fixture.routes("eth0")
	fixture.counters(map[string][2]uint64{"eth0": {1000, 2000}, "eth1": {300, 400}})

	if _, err := (&Collector{ProcRoot: fixture.procRoot}).TrafficInterfaces(); !errors.Is(err, ErrTrafficSelectionUnavailable) {
		t.Fatalf("TrafficInterfaces() without a path error = %v", err)
	}
	initial, err := collector.TrafficInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if initial.ResourceVersion == "" || initial.Selection.Include == nil || initial.Selection.Exclude == nil {
		t.Fatalf("initial snapshot = %#v", initial)
	}
	_, err = collector.ReplaceTrafficInterfaces(contract.UpdateTrafficInterfacesInput{Include: []string{"eth0", "eth0 "}, ExpectedResourceVersion: initial.ResourceVersion})
	if err != nil {
		t.Fatalf("ReplaceTrafficInterfaces() error = %v", err)
	}
	updated, err := collector.TrafficInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.Selection.Include, []string{"eth0"}) || updated.ResourceVersion == initial.ResourceVersion {
		t.Fatalf("updated snapshot = %#v", updated)
	}
	if _, err := collector.ReplaceTrafficInterfaces(contract.UpdateTrafficInterfacesInput{ExpectedResourceVersion: initial.ResourceVersion}); !errors.Is(err, ErrTrafficSelectionConflict) {
		t.Fatalf("stale version error = %v, want conflict", err)
	}
	var validation *TrafficSelectionValidationError
	if _, err := collector.ReplaceTrafficInterfaces(contract.UpdateTrafficInterfacesInput{Include: []string{"a/b"}, ExpectedResourceVersion: updated.ResourceVersion}); !errors.As(err, &validation) {
		t.Fatalf("invalid name error = %v, want validation", err)
	}
	restored, err := collector.ReplaceTrafficInterfaces(contract.UpdateTrafficInterfacesInput{ExpectedResourceVersion: updated.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if restored.ResourceVersion != initial.ResourceVersion {
		t.Fatalf("automatic choice version = %q, want %q", restored.ResourceVersion, initial.ResourceVersion)
	}
	if _, err := os.Stat(collector.TrafficSelectionPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("automatic choice left the selection file: %v", err)
	}
}

func TestTrafficInterfaceSelectionNormalize(t *testing.T) {
	selection, err := contract.TrafficInterfaceSelection{Include: []string{" eth0", "eth0", "ens3"}}.Normalize()
	if err != nil || !reflect.DeepEqual(selection.Include, []string{"eth0", "ens3"}) || selection.Exclude == nil {
		t.Fatalf("Normalize() = %#v, %v", selection, err)
	}
	tooMany := make([]string, contract.MaxTrafficInterfaceSelection+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("eth%d", index)
	}
	for _, invalid := range []contract.TrafficInterfaceSelection{
		{Include: tooMany},
		{Include: []string{""}},
		{Include: []string{"eth 0"}},
		{Include: []string{"eth0:1"}},
		{Include: []string{".."}},
		{Exclude: []string{"lo"}},
		{Include: []string{"eth0"}, Exclude: []string{"eth0"}},
	} {
		if _, err := invalid.Normalize(); err == nil {
			t.Fatalf("Normalize(%#v) accepted an invalid selection", invalid)
		}
	}
}
