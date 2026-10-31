package httpclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBlocksNonpublicDestinations(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "0.0.0.0", "224.0.0.1", "[::1]", "[::ffff:127.0.0.1]", "[fc00::1]", "[fe80::1]", "[64:ff9b::7f00:1]"} {
		t.Run(host, func(t *testing.T) {
			d := NewWithOptions(Options{DialContext: func(context.Context, string, string) (net.Conn, error) {
				t.Fatal("attempted a network connection to a forbidden destination")
				return nil, errors.New("unexpected dial")
			}})
			if _, err := d.Post("http://"+host+"/notify", "{}", nil, time.Second); err == nil {
				t.Fatal("accepted a nonpublic destination")
			}
		})
	}
}

func TestRejectsAnyPrivateDNSResult(t *testing.T) {
	d := NewWithOptions(Options{
		LookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
		},
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("dialed a mixed public/private DNS result")
			return nil, errors.New("unexpected dial")
		},
	})
	if _, err := d.Post("http://mixed.example/notify", "{}", nil, time.Second); err == nil {
		t.Fatal("accepted a DNS answer containing a private IP")
	}
}

func TestPinsValidatedDNSAddressAndDisablesProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	const body = `{"orderNo":"order-1"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if string(data) != body || r.Header.Get("X-Sandbox-Pay-Signature") != "test-signature" || r.Host != "public.example" {
			t.Error("callback body, signature, or hostname changed")
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()
	lookups := 0
	d := NewWithOptions(Options{
		LookupIP: func(context.Context, string) ([]net.IPAddr, error) {
			lookups++
			if lookups > 1 {
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			}
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != "8.8.8.8:80" {
				t.Fatalf("dial target was not pinned: %s", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	})
	resp, err := d.Post("http://public.example/notify", body, map[string]string{"X-Sandbox-Pay-Signature": "test-signature"}, time.Second)
	if err != nil || resp.Status != 200 || lookups != 1 {
		t.Fatalf("response=%v error=%v DNS lookups=%d", resp, err, lookups)
	}
}

func TestBlocksPrivateRedirect(t *testing.T) {
	for _, destination := range []string{"127.0.0.1", "private.example"} {
		t.Run(destination, func(t *testing.T) {
			testBlockedRedirect(t, destination)
		})
	}
}

func testBlockedRedirect(t *testing.T, destination string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+destination+"/secret", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	dials := 0
	d := NewWithOptions(Options{
		LookupIP: func(_ context.Context, host string) ([]net.IPAddr, error) {
			if host == "private.example" {
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			}
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dials++
			if dials > 1 {
				t.Fatal("connected to the private redirect")
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	})
	if _, err := d.Post("http://public.example/notify", "{}", nil, time.Second); err == nil || dials != 1 {
		t.Fatalf("private redirect result: error=%v dials=%d", err, dials)
	}
}

func TestLocalDevelopmentAndResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/large" {
			_, _ = io.WriteString(w, strings.Repeat("x", maxResponseBytes+1))
			return
		}
		_, _ = io.WriteString(w, `{"rates":{"USD":0.14}}`)
	}))
	defer server.Close()
	d := NewWithOptions(Options{AllowPrivateNetwork: true})
	resp, err := d.Get(server.URL, time.Second)
	if err != nil || resp.Status != 200 || resp.Data == nil {
		t.Fatalf("local development response=%v error=%v", resp, err)
	}
	if _, err := d.Get(server.URL+"/large", time.Second); err == nil {
		t.Fatal("read an unbounded response")
	}
}
