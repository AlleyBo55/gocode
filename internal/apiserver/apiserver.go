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
// authValidator is optional — if non-nil, it checks Bearer tokens on non-health endpoints.
func NewHandler(cfg Config, chatFn func(msg string) (string, error), statusFn func() (int, string), authValidator ...func(string) bool) http.Handler {
	mux := http.NewServeMux()

	// Auth middleware: wraps handlers that need authentication.
	requireAuth := func(next http.HandlerFunc) http.HandlerFunc {
		if len(authValidator) == 0 || authValidator[0] == nil {
			return next
		}
		validate := authValidator[0]
		return func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" || len(auth) < 8 || auth[:7] != "Bearer " {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if !validate(auth[7:]) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": cfg.Version})
	})

	mux.HandleFunc("/v1/chat", requireAuth(func(w http.ResponseWriter, r *http.Request) {
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
	}))

	mux.HandleFunc("/v1/status", requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		msgs, model := statusFn()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(StatusResponse{Messages: msgs, Model: model})
	}))

	return mux
}
