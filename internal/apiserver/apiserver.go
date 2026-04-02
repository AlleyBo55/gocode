package apiserver

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Config holds the API server configuration.
type Config struct {
	Version string
}

// ChatRequest is the request body for POST /v1/chat.
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse is the response body for POST /v1/chat.
type ChatResponse struct {
	Response string `json:"response"`
	Error    string `json:"error,omitempty"`
}

// StatusResponse is the response body for GET /v1/status.
type StatusResponse struct {
	Messages int    `json:"messages"`
	Model    string `json:"model"`
}

// NewHandler creates an http.Handler with all API routes.
// The chatFn callback processes a user message and returns the assistant response text.
// The statusFn callback returns (messageCount, modelName).
func NewHandler(cfg Config, chatFn func(msg string) (string, error), statusFn func() (int, string)) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": cfg.Version})
	})

	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"invalid json: %v"}`, err), http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			http.Error(w, `{"error":"message is required"}`, http.StatusBadRequest)
			return
		}
		resp, err := chatFn(req.Message)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ChatResponse{Error: err.Error()})
			return
		}
		json.NewEncoder(w).Encode(ChatResponse{Response: resp})
	})

	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		msgs, model := statusFn()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StatusResponse{Messages: msgs, Model: model})
	})

	return mux
}
