package server

import (
	"encoding/json"
	"io"
	"net/http"

	"takealot/pkg/workspace"
)

func (s *Server) SetPreferences(preferences *workspace.PreferencesStore) { s.preferences = preferences }

func (s *Server) handlePreferences(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.preferences == nil {
		jsonResponse(w, 503, map[string]string{"error": "界面配置未就绪"})
		return
	}
	if r.Method == http.MethodGet {
		jsonResponse(w, 200, s.preferences.Get())
		return
	}
	if r.Method != http.MethodPut {
		w.WriteHeader(405)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(415)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var patch struct {
		Theme         *string `json:"theme"`
		ActiveStoreID *string `json:"active_store_id"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&patch) != nil || decoder.Decode(new(any)) != io.EOF {
		jsonResponse(w, 400, map[string]string{"error": "界面配置格式错误"})
		return
	}
	value, err := s.preferences.Update(patch.Theme, patch.ActiveStoreID)
	if err != nil {
		jsonResponse(w, 400, map[string]string{"error": "无法保存界面配置"})
		return
	}
	jsonResponse(w, 200, value)
}
