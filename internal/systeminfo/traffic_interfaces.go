package systeminfo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	networkSelectionSchemaVersion  = 1
	networkSelectionFileLimit      = 4 << 10
	networkContinuitySchemaVersion = 1
	networkContinuityFileLimit     = 4 << 10
	// Persisting the continued counter lets a restart that finds a different
	// interface set continue near the last value instead of jumping.
	networkContinuityPersistEvery = 5 * time.Minute
)

var (
	ErrTrafficSelectionUnavailable = errors.New("network interface selection is not configured")
	ErrTrafficSelectionConflict    = errors.New("network interface selection changed")
)

// TrafficSelectionValidationError reports why a requested selection was refused.
type TrafficSelectionValidationError struct{ Detail string }

func (e *TrafficSelectionValidationError) Error() string { return e.Detail }

type networkSelectionFile struct {
	SchemaVersion int      `json:"schemaVersion"`
	Include       []string `json:"include,omitempty"`
	Exclude       []string `json:"exclude,omitempty"`
}

type networkSelectionStamp struct {
	modTime int64
	size    int64
	present bool
}

// networkContinuity keeps the reported counters continuous when the counted
// interface set changes. Without it, moving the default route to an interface
// with a larger counter (WARP, a failover uplink) adds that counter's history
// to the host's traffic at once. Reboots start over: the center already
// recognizes them by the uptime rollback.
type networkContinuity struct {
	SchemaVersion  int    `json:"schemaVersion"`
	BootID         string `json:"bootId"`
	Scope          string `json:"scope"`
	OffsetReceived int64  `json:"offsetReceived"`
	OffsetSent     int64  `json:"offsetSent"`
	LastReceived   uint64 `json:"lastReceived"`
	LastSent       uint64 `json:"lastSent"`
}

type networkCounter struct {
	name     string
	received uint64
	sent     uint64
}

// LoadTrafficSelection reads a selection file. A missing file is the automatic
// choice. The file must be a regular file of bounded size with no unknown
// fields.
func LoadTrafficSelection(path string) (contract.TrafficInterfaceSelection, []byte, error) {
	empty := contract.TrafficInterfaceSelection{Include: []string{}, Exclude: []string{}}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return empty, nil, nil
	}
	if err != nil {
		return empty, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > networkSelectionFileLimit {
		return empty, nil, errors.New("network interface selection must be a small regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, nil, err
	}
	defer file.Close()
	if opened, err := file.Stat(); err != nil || !os.SameFile(info, opened) {
		return empty, nil, errors.New("network interface selection changed while it was read")
	}
	content, err := io.ReadAll(io.LimitReader(file, networkSelectionFileLimit+1))
	if err != nil {
		return empty, nil, err
	}
	if len(content) > networkSelectionFileLimit {
		return empty, content, errors.New("network interface selection is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var stored networkSelectionFile
	if err := decoder.Decode(&stored); err != nil {
		return empty, content, fmt.Errorf("network interface selection is invalid: %w", err)
	}
	if decoder.More() || stored.SchemaVersion != networkSelectionSchemaVersion {
		return empty, content, errors.New("network interface selection has an unsupported format")
	}
	selection, err := contract.TrafficInterfaceSelection{Include: stored.Include, Exclude: stored.Exclude}.Normalize()
	if err != nil {
		return empty, content, err
	}
	return selection, content, nil
}

// SaveTrafficSelection atomically replaces the selection file. An empty
// selection removes the file, restoring the automatic choice. gid < 0 keeps
// the process group.
func SaveTrafficSelection(path string, selection contract.TrafficInterfaceSelection, mode os.FileMode, gid int) error {
	selection, err := selection.Normalize()
	if err != nil {
		return err
	}
	if len(selection.Include) == 0 && len(selection.Exclude) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return syncDirectory(filepath.Dir(path))
	}
	content, err := json.Marshal(networkSelectionFile{
		SchemaVersion: networkSelectionSchemaVersion, Include: selection.Include, Exclude: selection.Exclude,
	})
	if err != nil {
		return err
	}
	return atomicWriteFile(path, append(content, '\n'), mode, gid)
}

func atomicWriteFile(path string, content []byte, mode os.FileMode, gid int) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if gid >= 0 {
		if err := temporary.Chown(-1, gid); err != nil {
			temporary.Close()
			return err
		}
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	// Some filesystems refuse fsync on a directory; the rename has happened.
	_ = directory.Sync()
	return nil
}

func selectionVersion(content []byte) string {
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func parseNetworkCounters(dev string) []networkCounter {
	var counters []networkCounter
	for _, line := range strings.Split(dev, "\n") {
		name, values, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) < 16 {
			continue
		}
		received, _ := strconv.ParseUint(fields[0], 10, 64)
		sent, _ := strconv.ParseUint(fields[8], 10, 64)
		counters = append(counters, networkCounter{name: strings.TrimSpace(name), received: received, sent: sent})
	}
	return counters
}

// classifyNetworkInterfaces decides which interfaces count toward traffic and
// sums their counters. Without a selection, interfaces carrying a default route
// count so loopback and container/VPN layers are not counted twice; with no
// default route, every non-virtual interface counts. When none of the included
// interfaces exists (renamed, or a PPP link that is down), the automatic choice
// applies rather than silently counting nothing. scope identifies the counted
// set.
func classifyNetworkInterfaces(
	counters []networkCounter,
	defaultRoutes map[string]bool,
	selection contract.TrafficInterfaceSelection,
) (statuses []contract.TrafficInterfaceStatus, received, sent uint64, scope string) {
	present := map[string]bool{}
	for _, counter := range counters {
		present[counter.name] = true
	}
	include := map[string]bool{}
	for _, name := range selection.Include {
		if present[name] {
			include[name] = true
		}
	}
	exclude := map[string]bool{}
	for _, name := range selection.Exclude {
		exclude[name] = true
	}
	var counted []string
	for _, counter := range counters {
		status := contract.TrafficInterfaceStatus{Name: counter.name, ReceivedBytes: counter.received, SentBytes: counter.sent}
		switch {
		case counter.name == "lo":
			status.Reason = contract.TrafficInterfaceLoopback
		case exclude[counter.name]:
			status.Reason = contract.TrafficInterfaceExcluded
		case len(include) > 0 && include[counter.name]:
			status.Counted, status.Reason = true, contract.TrafficInterfaceSelected
		case len(include) > 0:
			status.Reason = contract.TrafficInterfaceNotSelected
		case len(defaultRoutes) > 0 && defaultRoutes[counter.name]:
			status.Counted, status.Reason = true, contract.TrafficInterfaceDefaultRoute
		case len(defaultRoutes) > 0:
			status.Reason = contract.TrafficInterfaceNotDefaultRoute
		case virtualNetworkInterface(counter.name):
			status.Reason = contract.TrafficInterfaceVirtual
		default:
			status.Counted, status.Reason = true, contract.TrafficInterfaceAutomatic
		}
		if status.Counted {
			received += counter.received
			sent += counter.sent
			counted = append(counted, counter.name)
		}
		statuses = append(statuses, status)
	}
	for _, name := range selection.Include {
		if !present[name] {
			statuses = append(statuses, contract.TrafficInterfaceStatus{Name: name, Reason: contract.TrafficInterfaceMissing})
		}
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	sort.Strings(counted)
	digest := sha256.Sum256([]byte(strings.Join(counted, "\n")))
	return statuses, received, sent, hex.EncodeToString(digest[:16])
}

// networkSelectionLocked returns the selection in effect, rereading the file
// only when it changed. A file that cannot be used falls back to the
// automatic choice and keeps the error for the interface snapshot.
func (c *Collector) networkSelectionLocked() (contract.TrafficInterfaceSelection, []byte, error) {
	empty := contract.TrafficInterfaceSelection{Include: []string{}, Exclude: []string{}}
	if c.TrafficSelectionPath == "" {
		return empty, nil, nil
	}
	stamp := networkSelectionStamp{}
	if info, err := os.Lstat(c.TrafficSelectionPath); err == nil {
		stamp = networkSelectionStamp{modTime: info.ModTime().UnixNano(), size: info.Size(), present: true}
	}
	if c.selectionLoaded && stamp == c.selectionStamp {
		return c.selection, c.selectionContent, c.selectionErr
	}
	selection, content, err := LoadTrafficSelection(c.TrafficSelectionPath)
	if err != nil {
		selection = empty
	}
	c.selection, c.selectionContent, c.selectionErr = selection, content, err
	c.selectionStamp, c.selectionLoaded = stamp, true
	return selection, content, err
}

// continueCountersLocked applies the continuity offset for the counted set
// and returns the counters to report.
func (c *Collector) continueCountersLocked(received, sent uint64, scope string) (uint64, uint64) {
	bootID := strings.TrimSpace(c.readOptional("sys/kernel/random/boot_id"))
	persistable := c.TrafficStatePath != "" && bootID != ""
	if !c.continuityLoaded {
		c.continuityLoaded = true
		if persistable {
			if stored, ok := loadNetworkContinuity(c.TrafficStatePath); ok && stored.BootID == bootID {
				c.continuity = stored
			}
		}
	}
	state := &c.continuity
	changed := false
	switch {
	case state.BootID != bootID:
		*state = networkContinuity{SchemaVersion: networkContinuitySchemaVersion, BootID: bootID, Scope: scope}
		changed = true
	case state.Scope != scope:
		if state.Scope != "" {
			// Continue from the last reported value: the newly counted set's
			// earlier traffic was never part of this host's counters.
			state.OffsetReceived = clampedDifference(state.LastReceived, received)
			state.OffsetSent = clampedDifference(state.LastSent, sent)
		}
		state.SchemaVersion, state.Scope = networkContinuitySchemaVersion, scope
		changed = true
	}
	received, sent = applyOffset(received, state.OffsetReceived), applyOffset(sent, state.OffsetSent)
	// A counter that went backwards (an interface recreated under the same
	// name) is saved at once: a stale, higher last value would otherwise let a
	// later restart continue above what the center has already seen.
	changed = changed || received < state.LastReceived || sent < state.LastSent
	state.LastReceived, state.LastSent = received, sent
	if !persistable {
		return received, sent
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	if changed || now.Sub(c.continuityPersisted) >= networkContinuityPersistEvery {
		// Persistence is best effort: a failed write retries at the next sample.
		if content, err := json.Marshal(state); err == nil && atomicWriteFile(c.TrafficStatePath, content, 0o600, -1) == nil {
			c.continuityPersisted = now
		}
	}
	return received, sent
}

func loadNetworkContinuity(path string) (networkContinuity, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > networkContinuityFileLimit {
		return networkContinuity{}, false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return networkContinuity{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var state networkContinuity
	if decoder.Decode(&state) != nil || decoder.More() || state.SchemaVersion != networkContinuitySchemaVersion ||
		state.BootID == "" || state.Scope == "" {
		return networkContinuity{}, false
	}
	return state, true
}

// Counters stay far below 2^63 bytes; the clamp only guards corrupt input.
func clampedDifference(last, current uint64) int64 {
	const limit = uint64(1) << 62
	return int64(min(last, limit)) - int64(min(current, limit))
}

func applyOffset(value uint64, offset int64) uint64 {
	if offset >= 0 {
		return value + uint64(offset)
	}
	if decrease := uint64(-offset); value > decrease {
		return value - decrease
	}
	return 0
}

// TrafficInterfaces lists every interface with whether it counts toward
// traffic, for the panel's interface selection.
func (c *Collector) TrafficInterfaces() (contract.TrafficInterfacesSnapshot, error) {
	c.prepareDefaults()
	if c.TrafficSelectionPath == "" {
		return contract.TrafficInterfacesSnapshot{}, ErrTrafficSelectionUnavailable
	}
	dev := c.readOptional("net/dev")
	if dev == "" {
		return contract.TrafficInterfacesSnapshot{}, errors.New("read network: unavailable /proc/net/dev")
	}
	defaultRoutes := c.defaultRouteInterfaces()
	c.networkMu.Lock()
	defer c.networkMu.Unlock()
	selection, content, selectionErr := c.networkSelectionLocked()
	statuses, _, _, _ := classifyNetworkInterfaces(parseNetworkCounters(dev), defaultRoutes, selection)
	snapshot := contract.TrafficInterfacesSnapshot{
		Selection: selection, Interfaces: statuses, ResourceVersion: selectionVersion(content),
	}
	if snapshot.Interfaces == nil {
		snapshot.Interfaces = []contract.TrafficInterfaceStatus{}
	}
	if selectionErr != nil {
		snapshot.SelectionError = "network interface selection cannot be read; the automatic choice is used"
	}
	return snapshot, nil
}

// ReplaceTrafficInterfaces stores a new selection when the caller saw the
// current one. An empty selection restores the automatic choice.
func (c *Collector) ReplaceTrafficInterfaces(input contract.UpdateTrafficInterfacesInput) (contract.TrafficInterfacesSnapshot, error) {
	c.prepareDefaults()
	if c.TrafficSelectionPath == "" {
		return contract.TrafficInterfacesSnapshot{}, ErrTrafficSelectionUnavailable
	}
	selection, err := contract.TrafficInterfaceSelection{Include: input.Include, Exclude: input.Exclude}.Normalize()
	if err != nil {
		return contract.TrafficInterfacesSnapshot{}, &TrafficSelectionValidationError{Detail: err.Error()}
	}
	c.networkMu.Lock()
	_, content, _ := c.networkSelectionLocked()
	if input.ExpectedResourceVersion != selectionVersion(content) {
		c.networkMu.Unlock()
		return contract.TrafficInterfacesSnapshot{}, ErrTrafficSelectionConflict
	}
	err = SaveTrafficSelection(c.TrafficSelectionPath, selection, 0o600, -1)
	c.selectionLoaded = false
	c.networkMu.Unlock()
	if err != nil {
		return contract.TrafficInterfacesSnapshot{}, err
	}
	return c.TrafficInterfaces()
}
