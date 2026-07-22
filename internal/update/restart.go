package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type shutdownRegistry interface{ Shutdown(context.Context) error }

func RequestRestart(ctx context.Context, path string, registry shutdownRegistry, shutdown func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
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
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	keepMarker := false
	defer func() {
		if !keepMarker {
			_ = os.Remove(path)
		}
	}()
	var cleanupErr error
	if registry != nil {
		shutdownContext, cancel := context.WithTimeout(ctx, 10*time.Second)
		cleanupErr = registry.Shutdown(shutdownContext)
		cancel()
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, cleanupErr)
	}
	if err := shutdown(); err != nil {
		return err
	}
	keepMarker = true
	return cleanupErr
}
