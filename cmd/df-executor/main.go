package main

import (
	"context"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer"
)

func main() {
	_ = godotenv.Load() // optional .env

	hmacKeyHex := os.Getenv("OVERSEER_HMAC_KEY")
	if hmacKeyHex == "" {
		log.Fatal("OVERSEER_HMAC_KEY env var is required (hex-encoded HMAC key)")
	}
	hmacKey, err := hex.DecodeString(hmacKeyHex)
	if err != nil {
		log.Fatalf("OVERSEER_HMAC_KEY is not valid hex: %v", err)
	}

	dfhackRunPath := os.Getenv("DFHACK_RUN_PATH")
	if dfhackRunPath == "" {
		log.Fatal("DFHACK_RUN_PATH env var is required (absolute path to dfhack-run)")
	}

	listen := os.Getenv("EXECUTOR_LISTEN")
	if listen == "" {
		listen = "0.0.0.0:8123"
	}

	svc := overseer.NewService(dfhackRunPath)
	server := overseer.NewServer(hmacKey, svc.Submit)

	mux := http.NewServeMux()
	mux.Handle("/ws", server)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	httpServer := &http.Server{
		Addr:    listen,
		Handler: mux,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("df-executor shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(ctx)
	}()

	log.Printf("df-executor listening on %s (ws path /ws, dfhack-run %s)", listen, dfhackRunPath)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Println("df-executor stopped")
}
