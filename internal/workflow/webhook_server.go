package workflow

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// WebhookRegistration holds the config for a registered webhook.
type WebhookRegistration struct {
	WorkflowID string
	NodeID     string
	Path       string             // unique URL path segment, e.g. UUID
	Method     string             // "GET", "POST", "ANY"
	HMACSecret string             // if non-empty, validate X-Hub-Signature-256 header
	TriggerFn  func(items []Item) // called when the webhook fires
}

// WebhookServer is a standalone net/http server on a configurable port.
type WebhookServer struct {
	addr   string
	mu     sync.RWMutex
	routes map[string]*WebhookRegistration // path → registration
	server *http.Server
	logger zerolog.Logger
}

// NewWebhookServer creates a server that will listen on addr (e.g. ":9321").
func NewWebhookServer(addr string, logger zerolog.Logger) *WebhookServer {
	s := &WebhookServer{
		addr:   addr,
		routes: make(map[string]*WebhookRegistration),
		logger: logger,
	}
	s.server = &http.Server{
		Addr:         addr,
		Handler:      s,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

// Start starts the HTTP server in a goroutine. Returns immediately.
func (s *WebhookServer) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("webhook server: listen on %s: %w", s.addr, err)
	}
	s.logger.Info().Str("addr", s.addr).Msg("webhook server starting")
	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error().Err(err).Msg("webhook server error")
		}
	}()
	return nil
}

// Stop gracefully shuts down the server with a 5-second timeout.
func (s *WebhookServer) Stop(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.logger.Info().Msg("webhook server shutting down")
	return s.server.Shutdown(shutdownCtx)
}

// Register adds a webhook route. Path must be unique.
func (s *WebhookServer) Register(reg *WebhookRegistration) error {
	if reg == nil {
		return fmt.Errorf("webhook server: registration must not be nil")
	}
	if reg.Path == "" {
		return fmt.Errorf("webhook server: registration path must not be empty")
	}
	if reg.TriggerFn == nil {
		return fmt.Errorf("webhook server: registration TriggerFn must not be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.routes[reg.Path]; exists {
		return fmt.Errorf("webhook server: path %q is already registered", reg.Path)
	}
	s.routes[reg.Path] = reg
	s.logger.Info().
		Str("path", reg.Path).
		Str("workflow_id", reg.WorkflowID).
		Str("node_id", reg.NodeID).
		Msg("webhook registered")
	return nil
}

// Deregister removes a webhook route by path.
func (s *WebhookServer) Deregister(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.routes, path)
	s.logger.Info().Str("path", path).Msg("webhook deregistered")
}

// ServeHTTP handles all incoming webhook requests.
// Routes: POST/GET /webhook/{path}
// Returns 404 if path not found, 405 if method doesn't match, 200 on success.
func (s *WebhookServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Hub-Signature-256")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Set CORS headers on all responses
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Content-Type", "application/json")

	// Parse path: must be /webhook/{path}
	urlPath := r.URL.Path
	const prefix = "/webhook/"
	if !strings.HasPrefix(urlPath, prefix) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	pathKey := strings.TrimPrefix(urlPath, prefix)
	pathKey = strings.Trim(pathKey, "/")
	if pathKey == "" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}

	s.mu.RLock()
	reg, ok := s.routes[pathKey]
	s.mu.RUnlock()

	if !ok {
		writeJSONError(w, http.StatusNotFound, "webhook not found")
		return
	}

	// Validate HTTP method
	if reg.Method != "ANY" {
		if r.Method != reg.Method {
			writeJSONError(w, http.StatusMethodNotAllowed, fmt.Sprintf("method not allowed; expected %s", reg.Method))
			return
		}
	} else {
		// ANY: only allow GET and POST
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
	}

	// Limit body size to 1MB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	// Read body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large or read error")
		return
	}

	// HMAC validation
	if reg.HMACSecret != "" {
		sigHeader := r.Header.Get("X-Hub-Signature-256")
		if sigHeader == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing X-Hub-Signature-256 header")
			return
		}
		if !validateHMAC(bodyBytes, reg.HMACSecret, sigHeader) {
			writeJSONError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	// Parse body as JSON into map[string]interface{}
	var data map[string]interface{}
	if len(bodyBytes) == 0 {
		data = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(bodyBytes, &data); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON body: %s", err.Error()))
			return
		}
	}

	item := NewItem(data)

	s.logger.Debug().
		Str("path", pathKey).
		Str("workflow_id", reg.WorkflowID).
		Str("method", r.Method).
		Msg("webhook triggered")

	// Call the trigger function
	reg.TriggerFn([]Item{item})

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}

// validateHMAC checks the X-Hub-Signature-256 header against the body and secret.
// The header format is: "sha256=<hex digest>"
func validateHMAC(body []byte, secret, sigHeader string) bool {
	const sigPrefix = "sha256="
	if !strings.HasPrefix(sigHeader, sigPrefix) {
		return false
	}
	sigHex := strings.TrimPrefix(sigHeader, sigPrefix)
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)

	return hmac.Equal(expected, sigBytes)
}

// writeJSONError writes a JSON error response with the given status code and message.
func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp, _ := json.Marshal(map[string]string{"error": msg})
	_, _ = w.Write(resp)
}
