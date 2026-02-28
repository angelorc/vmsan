package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/angelorc/vmsan/agent/shell"
)

const version = "0.1.0"

func auditMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("audit",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.statusCode = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Flush() {
	if f, ok := rec.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack delegates to the underlying ResponseWriter if it supports http.Hijacker.
// This is needed for WebSocket upgrade through audit middleware.
func (rec *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rec.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

// Unwrap returns the underlying ResponseWriter for Go 1.20+ http.ResponseController compatibility.
func (rec *statusRecorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}

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

	// Authenticated with audit logging.
	// Note: audit middleware is inside authMiddleware intentionally — only
	// authenticated requests are logged. Auth failures are rejected before
	// reaching the audit layer.
	mux.Handle("POST /exec", authMiddleware(*token, auditMiddleware(logger, http.HandlerFunc(makeRunHandler(logger)))))
	mux.Handle("POST /exec/{id}/kill", authMiddleware(*token, auditMiddleware(logger, http.HandlerFunc(handleKill))))
	mux.Handle("POST /files/write", authMiddleware(*token, auditMiddleware(logger, http.HandlerFunc(makeFilesWriteHandler(logger)))))
	mux.Handle("POST /files/read", authMiddleware(*token, auditMiddleware(logger, http.HandlerFunc(makeFilesReadHandler(logger)))))

	// Shell subsystem (WebSocket + REST)
	shellHandler := shell.NewHandler(*token, logger)
	shellHandler.Register(mux)

	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	logger.Info("server starting", "version", version, "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
