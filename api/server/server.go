package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sozercan/guac-ai-mole/api/models"
	"github.com/sozercan/guac-ai-mole/internal/analyzer"
	"github.com/sozercan/guac-ai-mole/internal/config"
)

type Server struct {
	cfg      config.ServerConfig
	server   *http.Server
	analyzer *analyzer.Analyzer
}

func New(cfg config.Config, analyzer *analyzer.Analyzer) *Server {
	s := &Server{
		cfg:      cfg.Server,
		analyzer: analyzer,
	}

	// Create router and register handlers
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/v1/analyze", s.loggingMiddleware(s.handleAnalyze))
	mux.HandleFunc("/api/v1/health", s.loggingMiddleware(s.handleHealth))

	// Static files
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("/", http.StripPrefix("/", fs))

	// Create server
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	return s
}

func (s *Server) loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response wrapper to capture status code
		rw := &responseWriter{ResponseWriter: w}

		// Call the next handler
		next(rw, r)

		// Log the request details
		slog.Info("HTTP request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start),
			"remote_addr", r.RemoteAddr,
		)
	}
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	// Only allow POST method
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.AnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	slog.Debug("Received analysis request", "request", req)

	result, err := s.analyzer.Analyze(r.Context(), req)
	if err != nil {
		slog.Error("Analysis request failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	slog.Debug("Analysis request completed successfully", "result", result)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Only allow GET method
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) Run() error {
	// Create a channel to listen for errors coming from the listener
	serverErrors := make(chan error, 1)

	// Start the server
	go func() {
		slog.Info("Starting server", "address", s.server.Addr)
		serverErrors <- s.server.ListenAndServe()
	}()

	// Create channel for interrupt signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Wait for interrupt or error
	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		slog.Info("Starting shutdown", "signal", sig)

		// Give outstanding requests a deadline for completion
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Trigger graceful shutdown
		err := s.server.Shutdown(ctx)
		if err != nil {
			// Error from closing listeners
			return fmt.Errorf("shutdown error: %w", err)
		}
	}

	return nil
}

// Custom response writer to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.status == 0 {
		rw.status = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}
