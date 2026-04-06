package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// Server is a WebSocket server that accepts a single connection from the
// Chrome Extension and dispatches commands/responses.
type Server struct {
	addr    string
	conn    *websocket.Conn
	connMu  sync.Mutex
	pending map[string]chan *Response
	pendMu  sync.Mutex

	connected chan struct{} // closed when first connection arrives
	connOnce  sync.Once

	logger zerolog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	server *http.Server
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewServer creates a new extension WebSocket server. Addr should be a
// host:port string such as ":9222".
func NewServer(addr string, logger zerolog.Logger) *Server {
	return &Server{
		addr:      addr,
		pending:   make(map[string]chan *Response),
		connected: make(chan struct{}),
		logger:    logger.With().Str("component", "extension-server").Logger(),
	}
}

// Start starts the HTTP/WebSocket server and blocks until the context is
// cancelled or the server shuts down.
func (s *Server) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/monoes", s.handleWS)

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	go func() {
		<-s.ctx.Done()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutCancel()
		_ = s.server.Shutdown(shutCtx)
	}()

	s.logger.Info().Str("addr", s.addr).Msg("extension server listening")
	err := s.server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// StartAsync starts the server in a background goroutine.
func (s *Server) StartAsync(ctx context.Context) {
	go func() {
		if err := s.Start(ctx); err != nil {
			s.logger.Error().Err(err).Msg("extension server error")
		}
	}()
}

// WaitForConnection blocks until the Chrome extension connects or the timeout
// expires.
func (s *Server) WaitForConnection(timeout time.Duration) error {
	select {
	case <-s.connected:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("extension did not connect within %s", timeout)
	}
}

// IsConnected returns true if the Chrome extension is currently connected.
func (s *Server) IsConnected() bool {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return s.conn != nil
}

// SendCommand sends a command to the Chrome extension and waits for the
// matching response. If the response indicates failure, an error is returned.
func (s *Server) SendCommand(cmd *Command, timeout time.Duration) (*Response, error) {
	if cmd.ID == "" {
		cmd.ID = uuid.New().String()
	}

	ch := make(chan *Response, 1)
	s.pendMu.Lock()
	s.pending[cmd.ID] = ch
	s.pendMu.Unlock()

	defer func() {
		s.pendMu.Lock()
		delete(s.pending, cmd.ID)
		s.pendMu.Unlock()
	}()

	data, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}

	s.connMu.Lock()
	conn := s.conn
	s.connMu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("no extension connected")
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, fmt.Errorf("write command: %w", err)
	}

	s.logger.Debug().Str("id", cmd.ID).Str("type", cmd.Type).Msg("command sent")

	select {
	case resp := <-ch:
		if !resp.Success {
			return resp, fmt.Errorf("extension error: %s", resp.Error)
		}
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("command %s timed out after %s", cmd.Type, timeout)
	}
}

// CreateTab asks the extension to open a new tab with the given URL and returns
// the tab ID.
func (s *Server) CreateTab(url string) (int, error) {
	resp, err := s.SendCommand(&Command{
		Type:   CmdCreateTab,
		Params: map[string]interface{}{"url": url},
	}, 30*time.Second)
	if err != nil {
		return 0, err
	}
	dataMap, _ := resp.Data.(map[string]interface{})
	if dataMap == nil {
		return 0, fmt.Errorf("create_tab response missing data")
	}
	tabIDRaw, ok := dataMap["tabId"]
	if !ok {
		return 0, fmt.Errorf("create_tab response missing tabId")
	}
	tabID, ok := tabIDRaw.(float64)
	if !ok {
		return 0, fmt.Errorf("tabId is not a number: %T", tabIDRaw)
	}
	return int(tabID), nil
}

// Close gracefully shuts down the server and closes the WebSocket connection.
func (s *Server) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.connMu.Lock()
	conn := s.conn
	s.conn = nil
	s.connMu.Unlock()

	if conn != nil {
		return conn.Close()
	}
	return nil
}

// handleWS upgrades an incoming HTTP request to a WebSocket connection.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error().Err(err).Msg("websocket upgrade failed")
		return
	}

	// Replace any existing connection.
	s.connMu.Lock()
	old := s.conn
	s.conn = conn
	s.connMu.Unlock()

	if old != nil {
		s.logger.Warn().Msg("replacing existing extension connection")
		_ = old.Close()
	}

	s.connOnce.Do(func() { close(s.connected) })
	s.logger.Info().Str("remote", conn.RemoteAddr().String()).Msg("extension connected")

	s.readLoop(conn)
}

// readLoop reads messages from the WebSocket and dispatches responses to
// waiting callers.
func (s *Server) readLoop(conn *websocket.Conn) {
	defer func() {
		s.connMu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		s.connMu.Unlock()
		_ = conn.Close()
		s.logger.Info().Msg("extension disconnected")
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Error().Err(err).Msg("websocket read error")
			}
			return
		}

		var resp Response
		if err := json.Unmarshal(msg, &resp); err != nil {
			s.logger.Error().Err(err).Str("raw", string(msg)).Msg("invalid response JSON")
			continue
		}

		// Debug: log raw message for eval responses to diagnose data flow.
		if resp.ID != "" {
			rawStr := string(msg)
			if len(rawStr) > 500 {
				rawStr = rawStr[:500] + "..."
			}
			s.logger.Debug().Str("raw", rawStr).Msg("raw response")
		}

		s.logger.Debug().Str("id", resp.ID).Bool("success", resp.Success).Str("error", resp.Error).Msg("response received")

		s.pendMu.Lock()
		ch, ok := s.pending[resp.ID]
		s.pendMu.Unlock()

		if ok {
			ch <- &resp
		} else {
			s.logger.Warn().Str("id", resp.ID).Msg("no pending request for response")
		}
	}
}
