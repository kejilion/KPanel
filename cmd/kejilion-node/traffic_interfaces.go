package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

// The selection sits beside node.json and, like it, is readable by the
// unprivileged telemetry service through the kejilion-node group.
const defaultTrafficSelectionPath = "/etc/kejilion-node/traffic-interfaces.json"

const trafficInterfacesUsage = "usage: kejilion-node interfaces [show | auto | include NAME... | exclude NAME...]"

// runTrafficInterfaces shows or changes which interfaces count toward this
// node's traffic. Changes apply at the next report without a restart.
func runTrafficInterfaces(arguments []string) error {
	if runtime.GOOS != "linux" {
		return errors.New("traffic interface selection requires Linux /proc network counters")
	}
	gid := -1
	if len(arguments) > 0 && arguments[0] != "show" {
		if runtime.GOOS == "linux" && os.Geteuid() != 0 {
			return errors.New("changing traffic interfaces requires root")
		}
		if !secureMigrationDirectory(filepath.Dir(defaultTrafficSelectionPath)) {
			return errors.New("lightweight node configuration directory is missing or unsafe")
		}
		var err error
		if gid, err = telemetryGroupID(); err != nil {
			return err
		}
	}
	return applyTrafficInterfaces(arguments, "/proc", defaultTrafficSelectionPath, gid, os.Stdout)
}

func applyTrafficInterfaces(arguments []string, procRoot, path string, gid int, out io.Writer) error {
	command := "show"
	if len(arguments) > 0 {
		command, arguments = arguments[0], arguments[1:]
	}
	var selection contract.TrafficInterfaceSelection
	switch command {
	case "show":
		if len(arguments) != 0 {
			return errors.New(trafficInterfacesUsage)
		}
	case "auto":
		if len(arguments) != 0 {
			return errors.New(trafficInterfacesUsage)
		}
	case "include", "exclude":
		if len(arguments) == 0 {
			return errors.New(trafficInterfacesUsage)
		}
		if command == "include" {
			selection.Include = arguments
		} else {
			selection.Exclude = arguments
		}
	default:
		return errors.New(trafficInterfacesUsage)
	}
	if command != "show" {
		mode := os.FileMode(0o640)
		if gid < 0 {
			mode = 0o600
		}
		if err := systeminfo.SaveTrafficSelection(path, selection, mode, gid); err != nil {
			return err
		}
	}
	collector := &systeminfo.Collector{ProcRoot: procRoot, TrafficSelectionPath: path}
	snapshot, err := collector.TrafficInterfaces()
	if err != nil {
		return err
	}
	return writeTrafficInterfaces(out, snapshot)
}

func telemetryGroupID() (int, error) {
	if runtime.GOOS != "linux" {
		return -1, nil
	}
	group, err := user.LookupGroup("kejilion-node")
	if err != nil {
		return -1, errors.New("telemetry service group is unavailable")
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil || gid <= 0 {
		return -1, errors.New("telemetry service group is unsafe")
	}
	return gid, nil
}

func writeTrafficInterfaces(out io.Writer, snapshot contract.TrafficInterfacesSnapshot) error {
	selection := snapshot.Selection
	switch {
	case len(selection.Include) > 0:
		fmt.Fprintf(out, "Counted interfaces: %s\n", strings.Join(selection.Include, " "))
	case len(selection.Exclude) > 0:
		fmt.Fprintf(out, "Counted interfaces: automatic, excluding %s\n", strings.Join(selection.Exclude, " "))
	default:
		fmt.Fprintln(out, "Counted interfaces: automatic (interfaces with a default route)")
	}
	if snapshot.SelectionError != "" {
		fmt.Fprintf(out, "Warning: %s\n", snapshot.SelectionError)
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "INTERFACE\tCOUNTED\tREASON\tRECEIVED\tSENT")
	for _, entry := range snapshot.Interfaces {
		counted := "no"
		if entry.Counted {
			counted = "yes"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", entry.Name, counted, entry.Reason,
			formatTrafficBytes(entry.ReceivedBytes), formatTrafficBytes(entry.SentBytes))
	}
	return table.Flush()
}

func formatTrafficBytes(value uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	size, unit := float64(value), 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d B", value)
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}
