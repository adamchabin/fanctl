package api

import (
	"encoding/json"
	"net/http"

	"fanctl/internal/config"
)

func (s *Server) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	var newCfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&newCfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.cfgLock.Lock()
	defer s.cfgLock.Unlock()

	if err := newCfg.Save(s.configPath); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	*s.cfg = newCfg

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(s.cfg)
}
