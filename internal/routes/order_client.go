package routes

import (
	"encoding/json"
	"net/http"
	"net/netip"

	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/services"
)

// orderClient uses only the peer address or a forwarding chain validated by
// ProxyHeaders. The request body cannot override recorded request metadata.
func (a *App) orderClient(r *http.Request, caller services.OrderCaller) services.OrderCaller {
	caller.ClientIP, caller.ClientLocation = nil, nil
	addr, err := netip.ParseAddr(httpx.ClientIP(r))
	if err != nil || addr.Zone() != "" || addr.IsUnspecified() || addr.IsMulticast() {
		return caller
	}
	ip := addr.Unmap().String()
	caller.ClientIP = &ip
	if a.c.Geolocation != nil {
		if data, err := json.Marshal(a.c.Geolocation.Lookup(ip)); err == nil {
			location := string(data)
			caller.ClientLocation = &location
		}
	}
	return caller
}
