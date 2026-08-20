package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iszy/geo-debug-server/internal/buildinfo"
	"github.com/iszy/geo-debug-server/internal/config"
	debugserver "github.com/iszy/geo-debug-server/internal/server"
	"github.com/iszy/geo-debug-server/internal/store"
)

func main() {
	cfg := config.Parse()
	if cfg.Version {
		fmt.Print(buildinfo.String())
		return
	}
	ctx := context.Background()
	database, err := store.Open(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer database.Close()

	httpServer := &http.Server{
		Addr:              cfg.Listen,
		Handler:           debugserver.New(database, cfg),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stopped
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownContext); err != nil {
			log.Printf("shutdown server: %v", err)
		}
	}()

	log.Printf("geo debug server listening on %s%s", cfg.Listen, cfg.BasePath)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve HTTP: %v", err)
	}
}
