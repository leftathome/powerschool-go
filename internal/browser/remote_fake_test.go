package browser

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// fakeToken is the secret every test endpoint carries. No error, log line or
// formatted error value may ever contain it.
const fakeToken = "tok-5ecr3t-0f-the-browser-service"

// fakeCDP is a DevTools-protocol WebSocket server that records every method
// it is sent and answers just enough for chromedp to open a browser context,
// create a page in it and start navigating. It has no DOM: a navigation is
// acknowledged but never finishes loading, so a sign-in driven against it
// always stops before the form is filled. That is what the tests using it
// need: they are about what is created, what is disposed, and when.
type fakeCDP struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	calls    []fakeCall
	behave   map[string]string // method -> "stall" | "close" | "refuse-once" | "refuse-echo"
	requests []string          // request URIs of the connections made
	seen     map[string]chan struct{}

	closed chan struct{} // closed when the first connection has ended
	once   sync.Once
}

type fakeCall struct {
	Method    string
	SessionID string
	Params    map[string]any
}

const (
	fakeContextID = "CTX-FAKE-1"
	fakeTargetID  = "TGT-FAKE-1"
	fakeSessionID = "SESS-FAKE-1"
	// closedMarker is recorded in the call list when the connection ends,
	// so tests can assert what happened before it.
	closedMarker = "<connection closed>"
)

func newFakeCDP(t *testing.T) *fakeCDP {
	t.Helper()
	f := &fakeCDP{
		t:      t,
		behave: map[string]string{},
		seen:   map[string]chan struct{}{},
		closed: make(chan struct{}),
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// url is the endpoint of the fake, token included.
func (f *fakeCDP) url() string {
	return "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/?token=" + fakeToken
}

// on sets what the fake does when it receives method.
func (f *fakeCDP) on(method, behaviour string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.behave[method] = behaviour
}

// sawMethod returns a channel closed once method has been received.
func (f *fakeCDP) sawMethod(method string) <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seenLocked(method)
}

func (f *fakeCDP) seenLocked(method string) chan struct{} {
	ch, ok := f.seen[method]
	if !ok {
		ch = make(chan struct{})
		f.seen[method] = ch
	}
	return ch
}

func (f *fakeCDP) record(c fakeCall) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	ch := f.seenLocked(c.Method)
	select {
	case <-ch:
	default:
		close(ch)
	}
	return f.behave[c.Method]
}

func (f *fakeCDP) snapshot() []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeCall(nil), f.calls...)
}

func (f *fakeCDP) connections() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// waitClosed waits for the client to have closed its connection.
func (f *fakeCDP) waitClosed(t *testing.T) {
	t.Helper()
	select {
	case <-f.closed:
	case <-time.After(5 * time.Second):
		t.Fatalf("the connection to the remote browser was never closed")
	}
}

func (f *fakeCDP) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.RequestURI())
	f.mu.Unlock()

	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}
	defer func() {
		_ = conn.Close()
		f.record(fakeCall{Method: closedMarker})
		f.once.Do(func() { close(f.closed) })
	}()
	f.loop(conn)
}

func (f *fakeCDP) loop(conn net.Conn) {
	for {
		data, err := wsutil.ReadClientText(conn)
		if err != nil {
			return
		}
		var msg struct {
			ID        int64          `json:"id"`
			Method    string         `json:"method"`
			SessionID string         `json:"sessionId"`
			Params    map[string]any `json:"params"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch f.record(fakeCall{Method: msg.Method, SessionID: msg.SessionID, Params: msg.Params}) {
		case "stall":
			continue
		case "close":
			return
		case "refuse-echo":
			// A refusal that is not "no window", from a browser that
			// repeats a secret in its message.
			refusal := map[string]any{"id": msg.ID, "error": map[string]any{
				"code": -32602, "message": "Invalid parameters " + testPassword + " " + fakeToken,
			}}
			out, _ := json.Marshal(refusal)
			if err := wsutil.WriteServerText(conn, out); err != nil {
				return
			}
			continue
		case "refuse-once":
			f.on(msg.Method, "")
			refusal := map[string]any{"id": msg.ID, "error": map[string]any{
				"code": -32000, "message": "Failed to open new tab - no browser is open",
			}}
			out, _ := json.Marshal(refusal)
			if err := wsutil.WriteServerText(conn, out); err != nil {
				return
			}
			continue
		}

		result := map[string]any{}
		switch msg.Method {
		case "Target.createBrowserContext":
			result["browserContextId"] = fakeContextID
		case "Target.createTarget":
			result["targetId"] = fakeTargetID
		case "Target.attachToTarget":
			result["sessionId"] = fakeSessionID
		case "Runtime.evaluate":
			result["result"] = map[string]any{"type": "object", "className": "Window"}
		case "Page.getFrameTree":
			result["frameTree"] = map[string]any{"frame": map[string]any{
				"id": "FRAME-FAKE-1", "loaderId": "LOADER-FAKE-0", "url": "about:blank",
				"securityOrigin": "://", "mimeType": "text/html",
			}}
		case "DOM.getDocument":
			result["root"] = map[string]any{
				"nodeId": 1, "backendNodeId": 1, "nodeType": 9, "nodeName": "#document",
				"localName": "", "nodeValue": "",
			}
		case "Page.navigate":
			result["frameId"] = "FRAME-FAKE-1"
			result["loaderId"] = "LOADER-FAKE-1"
		}
		resp := map[string]any{"id": msg.ID, "result": result}
		if msg.SessionID != "" {
			resp["sessionId"] = msg.SessionID
		}
		out, _ := json.Marshal(resp)
		if err := wsutil.WriteServerText(conn, out); err != nil {
			return
		}
	}
}

// index returns the position of the first call of method, or -1.
func indexOfCall(calls []fakeCall, method string) int {
	for i, c := range calls {
		if c.Method == method {
			return i
		}
	}
	return -1
}

func countCalls(calls []fakeCall, method string) int {
	n := 0
	for _, c := range calls {
		if c.Method == method {
			n++
		}
	}
	return n
}

// assertIsolatedAndDisposed checks the properties every remote sign-in must
// have, whatever its outcome, once it got as far as creating its browser
// context: the context was created to be disposed on detach, the page was
// created inside it, it was disposed over the still-open connection, and
// nothing outside it was touched.
func assertIsolatedAndDisposed(t *testing.T, calls []fakeCall, wantTarget bool) {
	t.Helper()

	ci := indexOfCall(calls, "Target.createBrowserContext")
	if ci < 0 {
		t.Fatalf("no browser context was created; calls: %v", methodsOf(calls))
	}
	if got := calls[ci].Params["disposeOnDetach"]; got != true {
		t.Errorf("browser context created with disposeOnDetach=%v, want true", got)
	}
	if n := countCalls(calls, "Target.createBrowserContext"); n != 1 {
		t.Errorf("%d browser contexts created, want 1", n)
	}

	ti := indexOfCall(calls, "Target.createTarget")
	if wantTarget {
		if ti < ci {
			t.Fatalf("the page target was not created after the browser context; calls: %v", methodsOf(calls))
		}
		if got := calls[ti].Params["browserContextId"]; got != fakeContextID {
			t.Errorf("page target created in browser context %v, want %q (not the default context)", got, fakeContextID)
		}
		if n := countCalls(calls, "Target.createTarget"); n < 1 || n > 2 {
			t.Errorf("%d attempts to create the page, want 1 (or 2 when the first is refused)", n)
		}
	}

	di := indexOfCall(calls, "Target.disposeBrowserContext")
	end := indexOfCall(calls, closedMarker)
	if di < 0 {
		t.Fatalf("the browser context was never disposed; calls: %v", methodsOf(calls))
	}
	if end >= 0 && di > end {
		t.Errorf("the browser context was disposed after the connection closed")
	}
	if di < ci {
		t.Errorf("dispose came before create")
	}

	assertNothingElseTouched(t, calls)
}

// assertNothingElseTouched checks that the remote browser and everything in
// it that is not this sign-in's was left alone.
func assertNothingElseTouched(t *testing.T, calls []fakeCall) {
	t.Helper()
	for _, c := range calls {
		switch c.Method {
		case "Browser.close", "Browser.crash", "Browser.resetPermissions",
			"Storage.clearCookies", "Network.clearBrowserCookies", "Network.clearBrowserCache",
			"Storage.clearDataForOrigin", "Target.getBrowserContexts":
			t.Errorf("forbidden method sent to the shared browser: %s", c.Method)
		case "Target.disposeBrowserContext":
			if id := c.Params["browserContextId"]; id != fakeContextID {
				t.Errorf("disposed a browser context that is not this sign-in's: %v", id)
			}
		case "Target.closeTarget", "Target.attachToTarget", "Target.activateTarget":
			if id := c.Params["targetId"]; id != fakeTargetID {
				t.Errorf("%s on a target that is not this sign-in's: %v", c.Method, id)
			}
		case "Target.createTarget":
			if id := c.Params["browserContextId"]; id != fakeContextID {
				t.Errorf("a target was created outside this sign-in's browser context: %v", id)
			}
		}
	}
}

// assertNoInput checks that nothing was typed or clicked: no credential
// reached the browser.
func assertNoInput(t *testing.T, calls []fakeCall) {
	t.Helper()
	for _, c := range calls {
		if strings.HasPrefix(c.Method, "Input.") {
			t.Errorf("input was sent to the browser before the sign-in page loaded: %s", c.Method)
		}
	}
}

func methodsOf(calls []fakeCall) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.Method
	}
	return out
}
