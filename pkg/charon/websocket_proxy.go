package charon

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// isWebSocketUpgrade returns true when the request is a WebSocket upgrade.
func isWebSocketUpgrade(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(req.Header.Get("Connection")), "upgrade")
}

// isEventStream returns true when the client is requesting a Server-Sent Events
// (SSE) stream.
func isEventStream(req *http.Request) bool {
	return strings.Contains(req.Header.Get("Accept"), "text/event-stream")
}

var wsUpgrader = websocket.Upgrader{
	// Allow all origins for the reverse-proxy use case. In production, callers
	// should configure a more restrictive CheckOrigin function.
	CheckOrigin: func(r *http.Request) bool { return true },
	// Use the same subprotocols as the client requested.
	Subprotocols: nil,
}

// proxyWebSocket upgrades both the client connection and the connection to the
// backend, then tunnels frames bidirectionally until one side closes.
func proxyWebSocket(w http.ResponseWriter, req *http.Request, targetURL *url.URL) error {
	// Build the backend WebSocket URL (ws:// or wss://).
	backendURL := *targetURL
	switch backendURL.Scheme {
	case "https":
		backendURL.Scheme = "wss"
	default:
		backendURL.Scheme = "ws"
	}
	backendURL.Path = req.URL.Path
	backendURL.RawQuery = req.URL.RawQuery

	// Forward selected request headers to the backend (strip hop-by-hop headers
	// that are managed by the WebSocket handshake itself).
	requestHeader := http.Header{}
	for _, h := range []string{
		"Authorization",
		"Cookie",
		"X-Tenant-ID",
		"X-Identity-ID",
		"X-Forwarded-For",
		"X-Session-ID",
	} {
		if v := req.Header.Get(h); v != "" {
			requestHeader.Set(h, v)
		}
	}
	// Forward requested subprotocols.
	if sp := req.Header.Get("Sec-Websocket-Protocol"); sp != "" {
		requestHeader.Set("Sec-Websocket-Protocol", sp)
		wsUpgrader.Subprotocols = websocket.Subprotocols(req)
	}

	// Connect to the backend.
	dialer := websocket.DefaultDialer
	backendConn, backendResp, err := dialer.Dial(backendURL.String(), requestHeader)
	if err != nil {
		if backendResp != nil {
			return fmt.Errorf("websocket dial failed (%d): %w", backendResp.StatusCode, err)
		}
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer backendConn.Close()

	// Upgrade the client connection.
	clientConn, err := wsUpgrader.Upgrade(w, req, nil)
	if err != nil {
		return fmt.Errorf("websocket upgrade failed: %w", err)
	}
	defer clientConn.Close()

	errc := make(chan error, 2)

	// Client → Backend
	go func() {
		for {
			msgType, msg, err := clientConn.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if err := backendConn.WriteMessage(msgType, msg); err != nil {
				errc <- err
				return
			}
		}
	}()

	// Backend → Client
	go func() {
		for {
			msgType, msg, err := backendConn.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				errc <- err
				return
			}
		}
	}()

	// Wait for either side to close.
	<-errc
	return nil
}

// proxyStream proxies a long-lived streaming response (e.g. Server-Sent Events)
// by making a plain HTTP request to the backend and copying the response body
// incrementally to the client, flushing after every write.
func proxyStream(w http.ResponseWriter, req *http.Request, targetURL *url.URL) error {
	backendURL := *targetURL
	backendURL.Path = req.URL.Path
	backendURL.RawQuery = req.URL.RawQuery

	// Build the backend request, copying the method, headers, and body.
	backendReq, err := http.NewRequestWithContext(req.Context(), req.Method, backendURL.String(), req.Body)
	if err != nil {
		return fmt.Errorf("building stream request: %w", err)
	}
	for key, vals := range req.Header {
		for _, v := range vals {
			backendReq.Header.Add(key, v)
		}
	}

	client := &http.Client{Timeout: 0} // No timeout — stream may last forever.
	resp, err := client.Do(backendReq)
	if err != nil {
		return fmt.Errorf("stream backend request: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers to the client.
	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}

// streamingResponse is a sentinel *http.Response returned by forwardRequest
// for WebSocket/SSE paths where the response has already been written directly
// to the http.ResponseWriter.
func streamingResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusSwitchingProtocols,
		Header:     make(http.Header),
		Body:       http.NoBody,
		// Indicate that the body was streamed directly.
		Trailer: http.Header{"X-Charon-Streamed": []string{"true"}},
		// Set a zero request to avoid nil-pointer dereferences downstream.
		Request: &http.Request{},
	}
}

// noopFlusher wraps any http.ResponseWriter that doesn't implement Flusher
// so that proxyStream can always call Flush safely (unused; kept for clarity).
type noopFlusher struct {
	http.ResponseWriter
}

func (noopFlusher) Flush() {}

// streamTimeout is the maximum time to wait for the first byte from a backend
// streaming response before giving up.
const streamTimeout = 30 * time.Second
