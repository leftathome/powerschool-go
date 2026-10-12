package cdptest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// LossyProxy is a DevTools WebSocket proxy in front of a real browser that
// forwards everything except the replies to the commands chosen by drop:
// the browser executes those commands, and the client never hears back.
// It is how a test loses the answer to one specific command.
type LossyProxy struct {
	// URL is the proxy's ws:// endpoint.
	URL string

	mu      sync.Mutex
	dropped int
}

// Dropped returns how many replies have been withheld so far.
func (p *LossyProxy) Dropped() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dropped
}

// NewLossyProxy starts a proxy to the browser at upstream (its ws:// URL).
// drop is called for every command the client sends; when it returns true,
// the reply to that command is not passed back.
func NewLossyProxy(t testing.TB, upstream string, drop func(method string, params json.RawMessage) bool) *LossyProxy {
	t.Helper()
	p := &LossyProxy{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		up, _, _, err := ws.Dial(ctx, upstream)
		cancel()
		if err != nil {
			return
		}
		defer up.Close()

		var mu sync.Mutex
		withheld := map[int64]bool{}
		done := make(chan struct{}, 2)

		go func() {
			defer func() { done <- struct{}{} }()
			for {
				data, op, err := wsutil.ReadClientData(client)
				if err != nil {
					return
				}
				var msg struct {
					ID     int64           `json:"id"`
					Method string          `json:"method"`
					Params json.RawMessage `json:"params"`
				}
				if json.Unmarshal(data, &msg) == nil && msg.Method != "" && drop(msg.Method, msg.Params) {
					mu.Lock()
					withheld[msg.ID] = true
					mu.Unlock()
				}
				if err := wsutil.WriteClientMessage(up, op, data); err != nil {
					return
				}
			}
		}()
		go func() {
			defer func() { done <- struct{}{} }()
			for {
				data, op, err := wsutil.ReadServerData(up)
				if err != nil {
					return
				}
				var msg struct {
					ID int64 `json:"id"`
				}
				if json.Unmarshal(data, &msg) == nil && msg.ID != 0 {
					mu.Lock()
					hold := withheld[msg.ID]
					mu.Unlock()
					if hold {
						p.mu.Lock()
						p.dropped++
						p.mu.Unlock()
						continue
					}
				}
				if err := wsutil.WriteServerMessage(client, op, data); err != nil {
					return
				}
			}
		}()
		<-done
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	p.URL = "ws" + strings.TrimPrefix(srv.URL, "http")
	return p
}
