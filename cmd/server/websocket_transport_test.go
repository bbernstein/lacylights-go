package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/graphql/resolvers"
)

func TestNewWebsocketTransport_Config(t *testing.T) {
	ws := newWebsocketTransport()
	if ws.KeepAlivePingInterval != 10*time.Second {
		t.Errorf("KeepAlivePingInterval = %v, want 10s", ws.KeepAlivePingInterval)
	}
}

// TestNewWebsocketTransport_Handshake checks that a GraphQL WebSocket
// handshake succeeds for both supported subprotocols, even from a foreign
// Origin (the server has always accepted any origin for subscriptions).
func TestNewWebsocketTransport_Handshake(t *testing.T) {
	srv := handler.New(generated.NewExecutableSchema(generated.Config{
		Resolvers: &resolvers.Resolver{},
	}))
	srv.AddTransport(newWebsocketTransport())
	ts := httptest.NewServer(srv)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	for _, proto := range []string{"graphql-transport-ws", "graphql-ws"} {
		t.Run(proto, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
				Subprotocols: []string{proto},
				HTTPHeader:   http.Header{"Origin": []string{"http://other-origin.example"}},
			})
			if err != nil {
				t.Fatalf("dial with foreign Origin failed: %v", err)
			}
			defer func() { _ = conn.CloseNow() }()

			if conn.Subprotocol() != proto {
				t.Fatalf("negotiated subprotocol = %q, want %q", conn.Subprotocol(), proto)
			}

			if err := wsjson.Write(ctx, conn, map[string]any{"type": "connection_init", "payload": map[string]any{}}); err != nil {
				t.Fatalf("write connection_init: %v", err)
			}

			var msg struct {
				Type string `json:"type"`
			}
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				t.Fatalf("read connection_ack: %v", err)
			}
			if msg.Type != "connection_ack" {
				t.Fatalf("first message type = %q, want connection_ack", msg.Type)
			}
		})
	}
}
