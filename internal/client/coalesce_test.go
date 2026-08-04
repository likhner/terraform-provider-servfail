package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetZone_CoalescesConcurrent(t *testing.T) {
	var hits int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-release
		_ = json.NewEncoder(w).Encode(Zone{Name: "example.com."})
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "")
	const n = 8
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if z, err := c.GetZone(context.Background(), "ns.", "example.com."); err != nil || z.Name != "example.com." {
				t.Errorf("GetZone = (%v, %v)", z, err)
			}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 upstream request, got %d", got)
	}
}

func TestGetPrimary_Memoized(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_ = json.NewEncoder(w).Encode(map[string]string{"primary": "ns.example."})
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "")
	for range 3 {
		if p, err := c.GetPrimary(context.Background(), "example.com."); err != nil || p != "ns.example." {
			t.Fatalf("GetPrimary = (%q, %v)", p, err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 upstream request, got %d", got)
	}
}
