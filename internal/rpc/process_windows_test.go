//go:build windows

package rpc

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsJobClosesDescendantAfterParentCanBeTerminated(t *testing.T) {
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("PowerShell is required")
	}
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	script := `$child = Start-Process -FilePath powershell.exe -ArgumentList '-NoProfile','-Command','Start-Sleep -Seconds 30' -PassThru; Set-Content -Path $env:CHILD_PID_PATH -Value $child.Id; Start-Sleep -Seconds 30`
	command := exec.Command(powershell, "-NoProfile", "-Command", script)
	command.Env = append(os.Environ(), "CHILD_PID_PATH="+pidPath)
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	group, err := attachProcessGroup(command)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	childPID := 0
	for time.Now().Before(deadline) {
		contents, readErr := os.ReadFile(pidPath)
		if readErr == nil {
			childPID, _ = strconv.Atoi(strings.TrimSpace(string(contents)))
			if childPID > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if childPID == 0 {
		_ = group.Close()
		_ = command.Wait()
		t.Fatal("descendant process did not start")
	}
	if err := group.Close(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !windowsProcessActive(uint32(childPID)) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("descendant process %d survived Job Object close", childPID)
}

func windowsProcessActive(pid uint32) bool {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false
	}
	if err != nil {
		return true
	}
	defer windows.CloseHandle(process)
	var exitCode uint32
	return windows.GetExitCodeProcess(process, &exitCode) == nil && exitCode == 259
}
