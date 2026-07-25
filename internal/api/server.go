package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"fanctl/internal/config"
	"fanctl/internal/hwmon"
)

type Server struct {
	configPath string
	cfg        *config.Config
	cfgLock    *sync.RWMutex
	scanner    *hwmon.Scanner
}

func NewServer(configPath string, cfg *config.Config, cfgLock *sync.RWMutex, scanner *hwmon.Scanner) *Server {
	return &Server{
		configPath: configPath,
		cfg:        cfg,
		cfgLock:    cfgLock,
		scanner:    scanner,
	}
}

func (s *Server) Start(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/telemetry", s.handleGetTelemetry)
	mux.HandleFunc("GET /api/v1/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/v1/config", s.handleUpdateConfig)

	addr := fmt.Sprintf(":%d", port)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleGetTelemetry(w http.ResponseWriter, r *http.Request) {
	telemetry, err := s.scanner.GetTelemetry()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(telemetry)
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.cfgLock.RLock()
	defer s.cfgLock.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.cfg)
}
