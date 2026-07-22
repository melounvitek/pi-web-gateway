//go:build linux

package resource

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxCgroupProcessBytes = 64 * 1024
	maxCgroupProcessCount = 4096
)

type Monitor struct {
	ProcRoot   string
	CgroupRoot string
	PID        int
	ReadFile   func(string) ([]byte, error)
}

func NewMonitor() *Monitor {
	return &Monitor{ProcRoot: "/proc", CgroupRoot: "/sys/fs/cgroup", PID: os.Getpid()}
}

func (monitor *Monitor) Snapshot() (*Snapshot, error) {
	path, err := unifiedCgroupPath(filepath.Join(monitor.ProcRoot, "self", "cgroup"))
	if err != nil || path == "" || path == "/" {
		return nil, err
	}
	root, err := filepath.Abs(monitor.CgroupRoot)
	if err != nil {
		return nil, err
	}
	directory, err := filepath.Abs(filepath.Join(root, strings.TrimPrefix(path, "/")))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, directory)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("cgroup path escapes root")
	}
	memory, err := integerFile(filepath.Join(directory, "memory.current"))
	if err != nil {
		return nil, err
	}
	inactive, err := statValue(filepath.Join(directory, "memory.stat"), "inactive_file")
	if err != nil {
		return nil, err
	}
	cpu, err := statValue(filepath.Join(directory, "cpu.stat"), "usage_usec")
	if err != nil {
		return nil, err
	}
	gatewayRSS, _ := processRSS(monitor.ProcRoot, monitor.PID)
	snapshot := &Snapshot{MemoryBytes: memory, WorkingSetBytes: max(memory-inactive, 0), InactiveFileBytes: inactive, CPUUsageUsec: cpu, GatewayRSSBytes: gatewayRSS}
	processes, err := readLimitedFile(filepath.Join(directory, "cgroup.procs"), maxCgroupProcessBytes)
	if err != nil {
		return nil, err
	}
	processIDs := strings.Fields(string(processes))
	if len(processIDs) > maxCgroupProcessCount {
		return nil, fmt.Errorf("cgroup process count exceeds %d", maxCgroupProcessCount)
	}
	for _, line := range processIDs {
		pid, parseErr := strconv.Atoi(line)
		if parseErr != nil {
			continue
		}
		identity, readErr := monitor.processIdentity(pid)
		if readErr != nil || !monitor.processInCgroup(pid, path) {
			continue
		}
		name, readErr := monitor.readFile(filepath.Join(monitor.ProcRoot, strconv.Itoa(pid), "comm"))
		if readErr != nil || !isPiProcessName(strings.TrimSpace(string(name))) {
			continue
		}
		rss, readErr := processRSS(monitor.ProcRoot, pid)
		if readErr != nil {
			continue
		}
		confirmedIdentity, readErr := monitor.processIdentity(pid)
		if readErr != nil || identity != confirmedIdentity || !monitor.processInCgroup(pid, path) {
			continue
		}
		snapshot.PiRSSBytes += rss
		snapshot.PiProcessCount++
	}
	return snapshot, nil
}

func isPiProcessName(name string) bool { return name == "pi" || name == "pi-rpc" }

func (monitor *Monitor) readFile(path string) ([]byte, error) {
	if monitor.ReadFile != nil {
		return monitor.ReadFile(path)
	}
	return os.ReadFile(path)
}

func (monitor *Monitor) processIdentity(pid int) (string, error) {
	contents, err := monitor.readFile(filepath.Join(monitor.ProcRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", err
	}
	closing := bytes.LastIndexByte(contents, ')')
	if closing < 0 {
		return "", errors.New("invalid process stat")
	}
	fields := strings.Fields(string(contents[closing+1:]))
	if len(fields) <= 19 {
		return "", errors.New("invalid process stat")
	}
	return fields[19], nil
}

func (monitor *Monitor) processInCgroup(pid int, expected string) bool {
	contents, err := monitor.readFile(filepath.Join(monitor.ProcRoot, strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(contents), "\n") {
		if strings.HasPrefix(line, "0::") && strings.TrimSpace(strings.TrimPrefix(line, "0::")) == expected {
			return true
		}
	}
	return false
}

func readLimitedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), limit)
	}
	return contents, nil
}

func unifiedCgroupPath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "0::") {
			return strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "0::")), nil
		}
	}
	return "", scanner.Err()
}

func integerFile(path string) (int64, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(contents)), 10, 64)
}

func statValue(path, key string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == key {
			return strconv.ParseInt(fields[1], 10, 64)
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("missing " + key)
}

func processRSS(root string, pid int) (int64, error) {
	file, err := os.Open(filepath.Join(root, strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[0] == "VmRSS:" {
			value, err := strconv.ParseInt(fields[1], 10, 64)
			return value * 1024, err
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("missing VmRSS")
}
