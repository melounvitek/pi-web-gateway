package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRegistry struct {
	path  string
	calls *[]string
	err   error
}

func (fake *fakeRegistry) Shutdown(context.Context) error {
	*fake.calls = append(*fake.calls, "close:"+boolText(fileExists(fake.path)))
	return fake.err
}
func TestRequestRestartCreatesMarkerBeforeCleanupAndShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "restart-request")
	calls := []string{}
	registry := &fakeRegistry{path: path, calls: &calls}
	err := RequestRestart(path, registry, func() error { calls = append(calls, "shutdown:"+boolText(fileExists(path))); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "close:true,shutdown:true" {
		t.Fatalf("calls = %v", calls)
	}
}
func TestRequestRestartRemovesMarkerWhenShutdownFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart")
	expected := errors.New("shutdown failed")
	err := RequestRestart(path, nil, func() error { return expected })
	if !errors.Is(err, expected) || fileExists(path) {
		t.Fatalf("error = %v, exists = %v", err, fileExists(path))
	}
}
func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
