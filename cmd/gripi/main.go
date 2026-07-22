package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	gripi "github.com/melounvitek/gripi"
	"github.com/melounvitek/gripi/internal/config"
	gateway "github.com/melounvitek/gripi/internal/server"
)

func main() {
	cfg, err := config.Load(os.Environ())
	if err != nil {
		log.Fatal(err)
	}
	handler, err := gateway.NewHandler(cfg, gripi.WebFiles)
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		log.Fatal(err)
	}

	shutdownSignal, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	serveErrors := make(chan error, 1)
	go func() {
		log.Printf("Gripi listening on %s", cfg.Address)
		serveErrors <- server.Serve(listener)
	}()

	exitCode := 0
	select {
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("gateway server: %v", err)
			exitCode = 1
		}
	case <-shutdownSignal.Done():
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("close gateway listener: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("gateway shutdown: %v", err)
		}
		if err := <-serveErrors; err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			log.Printf("gateway server: %v", err)
		}
	}
	if closer, ok := handler.(interface{ Close(context.Context) error }); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := closer.Close(ctx); err != nil {
			log.Printf("close RPC clients: %v", err)
			exitCode = 1
		}
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
