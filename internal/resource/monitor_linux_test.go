//go:build linux

package resource

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMonitorReadsCgroupAndProcessUsage(t *testing.T) {
	root := t.TempDir()
	proc, cgroup := filepath.Join(root, "proc"), filepath.Join(root, "cgroup")
	writeFixture(t, filepath.Join(proc, "self", "cgroup"), "0::/user.slice/gripi.service\n")
	directory := filepath.Join(cgroup, "user.slice", "gripi.service")
	writeFixture(t, filepath.Join(directory, "memory.current"), "637181952\n")
	writeFixture(t, filepath.Join(directory, "memory.stat"), "anon 400000000\ninactive_file 134217728\n")
	writeFixture(t, filepath.Join(directory, "cpu.stat"), "usage_usec 1234567\n")
	writeFixture(t, filepath.Join(directory, "cgroup.procs"), "100\n101\n102\n")
	writeProcessFixture(t, proc, 100, "gripi", 371124)
	writeProcessFixture(t, proc, 101, "pi", 183184)
	writeProcessFixture(t, proc, 102, "pi-rpc", 182668)
	monitor := &Monitor{ProcRoot: proc, CgroupRoot: cgroup, PID: 100}

	snapshot, err := monitor.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.MemoryBytes != 637181952 || snapshot.WorkingSetBytes != 502964224 || snapshot.CPUUsageUsec != 1234567 {
		t.Fatalf("cgroup snapshot = %+v", snapshot)
	}
	if snapshot.GatewayRSSBytes != 371124*1024 || snapshot.PiRSSBytes != (183184+182668)*1024 || snapshot.PiProcessCount != 2 {
		t.Fatalf("process snapshot = %+v", snapshot)
	}
}

func TestMonitorBoundsCgroupProcessInput(t *testing.T) {
	for name, processes := range map[string]string{
		"bytes": strings.Repeat("1", maxCgroupProcessBytes+1),
		"count": strings.Repeat("123456789\n", maxCgroupProcessCount+1),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			proc, cgroup := filepath.Join(root, "proc"), filepath.Join(root, "cgroup")
			writeFixture(t, filepath.Join(proc, "self", "cgroup"), "0::/service\n")
			directory := filepath.Join(cgroup, "service")
			writeFixture(t, filepath.Join(directory, "memory.current"), "10\n")
			writeFixture(t, filepath.Join(directory, "memory.stat"), "inactive_file 0\n")
			writeFixture(t, filepath.Join(directory, "cpu.stat"), "usage_usec 3\n")
			writeFixture(t, filepath.Join(directory, "cgroup.procs"), processes)

			if snapshot, err := (&Monitor{ProcRoot: proc, CgroupRoot: cgroup, PID: 100}).Snapshot(); err == nil || snapshot != nil {
				t.Fatalf("snapshot = %+v, error = %v", snapshot, err)
			}
		})
	}
}

func TestMonitorRejectsAProcessWhoseIdentityOrMembershipChanges(t *testing.T) {
	root := t.TempDir()
	proc, cgroup := filepath.Join(root, "proc"), filepath.Join(root, "cgroup")
	writeFixture(t, filepath.Join(proc, "self", "cgroup"), "0::/service\n")
	directory := filepath.Join(cgroup, "service")
	writeFixture(t, filepath.Join(directory, "memory.current"), "10\n")
	writeFixture(t, filepath.Join(directory, "memory.stat"), "inactive_file 0\n")
	writeFixture(t, filepath.Join(directory, "cpu.stat"), "usage_usec 3\n")
	writeFixture(t, filepath.Join(directory, "cgroup.procs"), "101\n102\n")
	writeProcessFixture(t, proc, 101, "pi", 10)
	writeProcessFixture(t, proc, 102, "pi", 20)
	writeFixture(t, filepath.Join(proc, "101", "cgroup"), "0::/other\n")
	writeFixture(t, filepath.Join(proc, "102", "cgroup"), "0::/service\n")
	statReads := 0
	monitor := &Monitor{ProcRoot: proc, CgroupRoot: cgroup, PID: 100}
	monitor.ReadFile = func(path string) ([]byte, error) {
		if path == filepath.Join(proc, "102", "stat") {
			statReads++
			return []byte(processStat(102, "pi", map[bool]string{true: "before", false: "after"}[statReads == 1])), nil
		}
		return os.ReadFile(path)
	}

	snapshot, err := monitor.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PiProcessCount != 0 || snapshot.PiRSSBytes != 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestMonitorReportsUnsupportedCgroupsAndToleratesExitedPi(t *testing.T) {
	root := t.TempDir()
	proc, cgroup := filepath.Join(root, "proc"), filepath.Join(root, "cgroup")
	writeFixture(t, filepath.Join(proc, "self", "cgroup"), "2:memory:/legacy\n")
	monitor := &Monitor{ProcRoot: proc, CgroupRoot: cgroup, PID: 100}
	if snapshot, err := monitor.Snapshot(); err != nil || snapshot != nil {
		t.Fatalf("legacy snapshot = %+v, %v", snapshot, err)
	}
	writeFixture(t, filepath.Join(proc, "self", "cgroup"), "0::/service\n")
	directory := filepath.Join(cgroup, "service")
	writeFixture(t, filepath.Join(directory, "memory.current"), "10\n")
	writeFixture(t, filepath.Join(directory, "memory.stat"), "inactive_file 20\n")
	writeFixture(t, filepath.Join(directory, "cpu.stat"), "usage_usec 3\n")
	writeFixture(t, filepath.Join(directory, "cgroup.procs"), "999\n")
	snapshot, err := monitor.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.WorkingSetBytes != 0 || snapshot.PiProcessCount != 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func writeFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}
func writeProcessFixture(t *testing.T, root string, pid int, name string, rss int) {
	t.Helper()
	directory := filepath.Join(root, fmtInt(pid))
	writeFixture(t, filepath.Join(directory, "comm"), name+"\n")
	writeFixture(t, filepath.Join(directory, "status"), "Name:\t"+name+"\nVmRSS:\t"+fmtInt(rss)+" kB\n")
	writeFixture(t, filepath.Join(directory, "stat"), processStat(pid, name, "100"))
	writeFixture(t, filepath.Join(directory, "cgroup"), "0::/user.slice/gripi.service\n")
}
func processStat(pid int, name, start string) string {
	return fmtInt(pid) + " (" + name + ") S 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 " + start + " 0\n"
}
func fmtInt(value int) string { return strconv.Itoa(value) }
