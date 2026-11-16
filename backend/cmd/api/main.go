// Command api is the SpecFlow server entrypoint. It serves the JSON API and
// the embedded web UI from a single process; workflow orchestration will be
// wired in as the product grows.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/specflow/backend/internal/api"
	"example.com/specflow/backend/internal/buildinfo"
	"example.com/specflow/backend/internal/config"
	"example.com/specflow/backend/internal/harness"
	"example.com/specflow/backend/internal/harness/pi"
	"example.com/specflow/backend/internal/orchestrator"
	"example.com/specflow/backend/web"
)

func main() {
	// Recorded before anything can block so the identity reports when this
	// process started serving, not when it finished booting.
	startedAt := time.Now()

	portFlag := flag.String("port", "", "TCP port to listen on (overrides PORT)")
	flag.Parse()

	portFlagSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			portFlagSet = true
		}
	})

	port, err := config.ResolvePort(*portFlag, portFlagSet, os.Getenv)
	if err != nil {
		log.Fatalf("specflow: %v", err)
	}

	staticFS, err := fs.Sub(web.StaticFS, web.DistDir)
	if err != nil {
		log.Fatalf("specflow: embedded UI: %v", err)
	}

	registry := harness.NewRegistry()
	if err := registry.Register(func() harness.Adapter { return pi.New(pi.Options{}) }); err != nil {
		log.Fatalf("specflow: register harness: %v", err)
	}

	engine := orchestrator.New(registry)
	defer engine.Close()

	// The identity lets a client ask which build is answering and whether the
	// checkout it was launched from has moved past it. Both reads are
	// best-effort: the server must start even when they cannot answer.
	checkoutRoot, _ := os.Getwd()

	server := &http.Server{
		Addr: fmt.Sprintf(":%d", port),
		Handler: api.NewRouterWithServer(staticFS, registry, engine, api.ServerIdentity{
			Info:         buildinfo.Read(),
			StartedAt:    startedAt,
			CheckoutRoot: checkoutRoot,
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("specflow listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("specflow: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("specflow: shutdown: %v", err)
	}
}
