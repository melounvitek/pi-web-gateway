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
	err := RequestRestart(context.Background(), path, registry, func() error { calls = append(calls, "shutdown:"+boolText(fileExists(path))); return nil })
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
	err := RequestRestart(context.Background(), path, nil, func() error { return expected })
	if !errors.Is(err, expected) || fileExists(path) {
		t.Fatalf("error = %v, exists = %v", err, fileExists(path))
	}
}
func TestRequestRestartRemovesMarkerWhenCancelledDuringCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart")
	started := make(chan struct{})
	registry := &cancellableRegistry{started: started}
	ctx, cancel := context.WithCancel(context.Background())
	shutdownCalled := false
	result := make(chan error, 1)
	go func() {
		result <- RequestRestart(ctx, path, registry, func() error { shutdownCalled = true; return nil })
	}()
	<-started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if fileExists(path) || shutdownCalled {
		t.Fatalf("exists = %v, shutdown = %v", fileExists(path), shutdownCalled)
	}
}

type cancellableRegistry struct{ started chan struct{} }

func (registry *cancellableRegistry) Shutdown(ctx context.Context) error {
	close(registry.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestRequestRestartDoesNothingWhenCancelled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	shutdownCalled := false
	err := RequestRestart(ctx, path, nil, func() error { shutdownCalled = true; return nil })
	if !errors.Is(err, context.Canceled) || fileExists(path) || shutdownCalled {
		t.Fatalf("error = %v, exists = %v, shutdown = %v", err, fileExists(path), shutdownCalled)
	}
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }
func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
