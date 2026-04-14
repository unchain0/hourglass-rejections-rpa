package hourglass

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

type testServer struct {
	URL string
}

type inMemoryRoundTripper struct {
	fallback http.RoundTripper
}

var (
	testServersMu        sync.RWMutex
	testServers          = make(map[string]http.Handler)
	testServerCounter    atomic.Int64
	installTransportOnce sync.Once
)

func newHTTPTestServer(t *testing.T, handler http.Handler) *testServer {
	t.Helper()
	installTestTransport()

	url := fmt.Sprintf("http://testserver-%d.local", testServerCounter.Add(1))

	testServersMu.Lock()
	testServers[url] = handler
	testServersMu.Unlock()

	return &testServer{URL: url}
}

func (s *testServer) Close() {
	testServersMu.Lock()
	delete(testServers, s.URL)
	testServersMu.Unlock()
}

func installTestTransport() {
	installTransportOnce.Do(func() {
		fallback := http.DefaultTransport
		http.DefaultTransport = &inMemoryRoundTripper{fallback: fallback}
	})
}

func (rt *inMemoryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	target := req.URL.Scheme + "://" + req.URL.Host

	testServersMu.RLock()
	handler := testServers[target]
	testServersMu.RUnlock()

	if handler == nil {
		return rt.fallback.RoundTrip(req)
	}

	respCh := make(chan *http.Response, 1)
	go func() {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		respCh <- recorder.Result()
	}()

	select {
	case resp := <-respCh:
		return resp, nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}
