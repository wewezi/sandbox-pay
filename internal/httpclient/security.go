package httpclient

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"
)

// Options permits explicit local development and deterministic network tests.
// Production callers should leave LookupIP and DialContext unset.
type Options struct {
	AllowPrivateNetwork bool
	LookupIP            func(context.Context, string) ([]net.IPAddr, error)
	DialContext         func(context.Context, string, string) (net.Conn, error)
}

// NewWithOptions prevents requests to nonpublic networks, including every
// redirect. DNS results are validated together and the connection uses an
// already validated IP rather than resolving the name a second time.
func NewWithOptions(options Options) *Default {
	lookup := options.LookupIP
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	dial := options.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	resolve := func(ctx context.Context, host string) ([]netip.Addr, error) {
		var addresses []netip.Addr
		if literal, err := netip.ParseAddr(host); err == nil {
			addresses = []netip.Addr{literal.Unmap()}
		} else {
			results, err := lookup(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve destination: %w", err)
			}
			for _, result := range results {
				addr, ok := netip.AddrFromSlice(result.IP)
				if !ok || result.Zone != "" {
					return nil, fmt.Errorf("destination has an invalid IP address")
				}
				addresses = append(addresses, addr.Unmap())
			}
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("destination has no IP addresses")
		}
		for _, addr := range addresses {
			if addr.Zone() != "" || (!options.AllowPrivateNetwork && !publicAddress(addr)) {
				return nil, fmt.Errorf("destination IP is not a public network address")
			}
		}
		return addresses, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // Environment proxies would bypass destination checks.
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addresses, err := resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, addr := range addresses {
			conn, err := dial(ctx, network, net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
	client := &http.Client{
		Transport: guardedTransport{transport},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			if err := validateDestination(req.URL); err != nil {
				return err
			}
			_, err := resolve(req.Context(), req.URL.Hostname())
			return err
		},
	}
	return &Default{HTTP: client}
}

type guardedTransport struct{ base *http.Transport }

func (t guardedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := validateDestination(req.URL); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func validateDestination(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("destination must be an HTTP(S) URL without credentials")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("destination has an invalid port")
		}
	}
	return nil
}

var nonpublicNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

func publicAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	// Current public IPv6 allocations are within 2000::/3. This also rejects
	// IPv4-compatible and NAT64 addresses that can conceal private IPv4 targets.
	if addr.Is6() && !netip.MustParsePrefix("2000::/3").Contains(addr) {
		return false
	}
	for _, network := range nonpublicNetworks {
		if network.Contains(addr) {
			return false
		}
	}
	return true
}
