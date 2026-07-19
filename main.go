package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tomsoir-service-chess-bots/internal/botsreg"
	"tomsoir-service-chess-bots/internal/chessapi"
	"tomsoir-service-chess-bots/internal/config"
	"tomsoir-service-chess-bots/internal/engineclient"
	"tomsoir-service-chess-bots/internal/fleet"
	"tomsoir-service-chess-bots/internal/play"
	"tomsoir-service-chess-bots/internal/roster"
	"tomsoir-service-chess-bots/internal/wsclient"
)

func main() {
	httpAddr := ":" + strings.TrimPrefix(config.HTTPPort(), ":")
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"service":"chess-bots"}`))
	})
	server := &http.Server{Addr: httpAddr, Handler: mux}

	go func() {
		log.Printf("chess-bots health on %s (enabled=%v)", httpAddr, config.Enabled())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if !config.Enabled() {
		log.Printf("BOTS_ENABLED is false — idle health-only mode")
		<-sigCtx.Done()
	} else {
		runFleet(sigCtx)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	log.Println("shutdown complete")
}

func runFleet(ctx context.Context) {
	identities := roster.DefaultRoster()
	ids := make([]string, len(identities))
	for i, id := range identities {
		ids[i] = id.ID
	}

	reg, err := botsreg.New(config.RedisAddr(), config.RedisPassword())
	if err != nil {
		log.Fatalf("bots registry: %v", err)
	}
	defer reg.Close()
	if err := reg.RegisterAll(ctx, ids); err != nil {
		log.Fatalf("register bots: %v", err)
	}
	log.Printf("registered %d bot identities in redis", len(ids))

	engine, err := engineclient.New(config.EngineGRPCAddr(), config.EngineMaxConcurrency())
	if err != nil {
		log.Fatalf("engine: %v", err)
	}

	chess := chessapi.New(config.ChessHTTPBase())
	presence := wsclient.NewPresenceHub(config.RealtimeWSBase())
	driver := play.New(chess, engine, config.RealtimeWSBase())
	mgr := fleet.New(chess, identities, driver, presence)
	driver.SetOnDone(mgr.MarkGameDone)

	log.Printf("fleet starting (chess=%s engine=%s ws=%s min=%d max=%d)",
		config.ChessHTTPBase(), config.EngineGRPCAddr(), config.RealtimeWSBase(),
		config.MinVisible(), config.MaxVisible())
	mgr.Start(ctx)
}
