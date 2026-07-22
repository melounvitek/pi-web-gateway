package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type shutdownRegistry interface{ Shutdown(context.Context) error }

func RequestRestart(path string, registry shutdownRegistry, shutdown func() error) error {
	if path == "" {
		return errors.New("GRIPI_RESTART_PATH must be set")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	temporary.Close()
	defer os.Remove(temporaryPath)
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	var cleanupErr error
	if registry != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cleanupErr = registry.Shutdown(ctx)
		cancel()
	}
	if err := shutdown(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return cleanupErr
}
