package healthprobe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHTTPCheckerExpectMatch 状态码精确匹配通过；不匹配报 HTTP 状态
func TestHTTPCheckerExpectMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	c := HTTPChecker{URL: srv.URL, ExpectStatus: http.StatusTeapot}
	if err := c.Check(context.Background()); err != nil {
		t.Fatalf("matching status: %v", err)
	}

	c.ExpectStatus = http.StatusOK
	err := c.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 418 (expect 200)") {
		t.Fatalf("mismatch err = %v", err)
	}
}

// TestHTTPCheckerDefaultExpect ExpectStatus 缺省 200；连接拒绝报错
func TestHTTPCheckerDefaultExpect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	if err := (HTTPChecker{URL: srv.URL}).Check(context.Background()); err != nil {
		t.Fatalf("default expect 200: %v", err)
	}

	err := (HTTPChecker{URL: "http://127.0.0.1:1"}).Check(context.Background())
	if err == nil {
		t.Fatal("refused connection should error")
	}
}

// TestHTTPCheckerBadURL 非法 URL 构造失败即报错
func TestHTTPCheckerBadURL(t *testing.T) {
	err := (HTTPChecker{URL: "http://[::1]:namedport"}).Check(context.Background())
	if err == nil {
		t.Fatal("bad url should error")
	}
}

// TestHTTPCheckerClientInjection Client 注入生效（自定义 Transport 计数）
func TestHTTPCheckerClientInjection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	calls := 0
	client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return http.DefaultTransport.RoundTrip(req)
	})}
	if err := (HTTPChecker{URL: srv.URL, Client: client}).Check(context.Background()); err != nil {
		t.Fatalf("injected client: %v", err)
	}
	if calls != 1 {
		t.Fatalf("client calls = %d, want 1", calls)
	}
}

// TestTCPChecker 监听通、拒绝报错
func TestTCPChecker(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	if err := (TCPChecker{Addr: ln.Addr().String()}).Check(context.Background()); err != nil {
		t.Fatalf("listening addr: %v", err)
	}
	err = (TCPChecker{Addr: "127.0.0.1:1"}).Check(context.Background())
	if err == nil {
		t.Fatal("refused addr should error")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
