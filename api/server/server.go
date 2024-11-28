package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/sozercan/guac-ai-mole/api/models"
	"github.com/sozercan/guac-ai-mole/internal/analyzer"
	"github.com/sozercan/guac-ai-mole/internal/config"
	"github.com/sozercan/guac-ai-mole/internal/guac"
	"github.com/sozercan/guac-ai-mole/internal/llm"
)

type Server struct {
	cfg      config.ServerConfig
	router   *chi.Mux
	analyzer *analyzer.Analyzer
}

func New(cfg config.Config) (*Server, error) {
	guacClient, err := guac.NewClient(cfg.GUAC.GraphQLEndpoint)
	if err != nil {
		return nil, err
	}

	llmProvider, err := llm.NewOpenAI(&cfg.OpenAI)
	if err != nil {
		return nil, err
	}

	analyzer := analyzer.New(guacClient, llmProvider)

	s := &Server{
		cfg:      cfg.Server,
		router:   chi.NewRouter(),
		analyzer: analyzer,
	}

	s.setupRoutes()
	return s, nil
}

func (s *Server) setupRoutes() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(30 * time.Second))

	// Serve static files
	fs := http.FileServer(http.Dir("web/static"))
	s.router.Handle("/*", fs)

	// API routes
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Post("/analyze", s.handleAnalyze)
		r.Get("/health", s.handleHealth)
	})
}

func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	var req models.AnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	ctx := r.Context()
	result, err := s.analyzer.Analyze(ctx, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) Run() error {
	addr := s.cfg.Host + ":" + s.cfg.Port
	return http.ListenAndServe(addr, s.router)
}
