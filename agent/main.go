package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/angelorc/vmsan/agent/shell"
)

const version = "0.1.0"

func main() {
	port := flag.Int("port", 9119, "listen port")
	token := flag.String("token", "", "auth token (or VMSAN_AGENT_TOKEN env)")
	flag.Parse()

	if *token == "" {
		*token = os.Getenv("VMSAN_AGENT_TOKEN")
	}
	if *token == "" {
		log.Fatal("auth token required: use --token or VMSAN_AGENT_TOKEN env")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mux := http.NewServeMux()

	// Unauthenticated
	mux.HandleFunc("GET /health", handleHealth)

	// Authenticated
	mux.Handle("POST /exec", authMiddleware(*token, http.HandlerFunc(handleRun)))
	mux.Handle("POST /exec/{id}/kill", authMiddleware(*token, http.HandlerFunc(handleKill)))
	mux.Handle("POST /files/write", authMiddleware(*token, http.HandlerFunc(handleFilesWrite)))
	mux.Handle("POST /files/read", authMiddleware(*token, http.HandlerFunc(handleFilesRead)))

	// Shell subsystem (WebSocket + REST)
	shellHandler := shell.NewHandler(*token, logger)
	shellHandler.Register(mux)

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	logger.Info("server starting", "version", version, "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
