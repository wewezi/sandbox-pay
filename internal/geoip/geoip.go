// Package geoip resolves request IP ownership with embedded offline databases.
package geoip

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

// Location is the network and geographic ownership known when an order is made.
// IP ownership does not establish the physical location of the payer.
type Location struct {
	Country string `json:"country,omitempty"`
	Region  string `json:"region,omitempty"`
	City    string `json:"city,omitempty"`
	ISP     string `json:"isp,omitempty"`
	Network string `json:"network"`
}

//go:embed data/*.xdb.gz
var databases embed.FS

type database struct {
	file    string
	size    int64
	digest  string
	version *xdb.Version
	once    sync.Once
	content []byte
	err     error
}

var ipv4 = database{file: "data/ip2region_v4.xdb.gz", size: 11122714, digest: "46249ad1b33bcaa4cc37475b93746cec2b00acffc3316cc4f56167ac821ea2ad", version: xdb.IPv4}
var ipv6 = database{file: "data/ip2region_v6.xdb.gz", size: 37277813, digest: "939f6b46bd2b8bec3cf7c5ceb8ba782266ae9b1f35b5ba7916700dec0b7506ed", version: xdb.IPv6}

func (d *database) load() ([]byte, error) {
	d.once.Do(func() {
		compressed, err := databases.ReadFile(d.file)
		if err != nil {
			d.err = err
			return
		}
		reader, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			d.err = err
			return
		}
		defer reader.Close()
		content, err := io.ReadAll(io.LimitReader(reader, d.size+1))
		if err != nil {
			d.err = err
			return
		}
		if int64(len(content)) != d.size {
			d.err = fmt.Errorf("invalid GeoIP database size")
			return
		}
		if fmt.Sprintf("%x", sha256.Sum256(content)) != d.digest {
			d.err = fmt.Errorf("invalid GeoIP database checksum")
			return
		}
		header, err := xdb.LoadHeaderFromBuff(content)
		if err != nil {
			d.err = err
			return
		}
		if header.IPVersion != d.version.Id || header.StartIndexPtr > header.EndIndexPtr || int64(header.EndIndexPtr) >= d.size {
			d.err = fmt.Errorf("invalid GeoIP database header")
			return
		}
		d.content = content
	})
	return d.content, d.err
}

// Locator shares immutable data across instances, including test containers.
type Locator struct{}

func New() *Locator { return &Locator{} }

var reserved = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2001:2::/48"),
}

func network(addr netip.Addr) string {
	switch {
	case addr.IsLoopback():
		return "loopback"
	case addr.IsPrivate():
		return "private"
	case addr.IsLinkLocalUnicast():
		return "link_local"
	case !addr.IsGlobalUnicast():
		return "reserved"
	}
	for _, prefix := range reserved {
		if prefix.Contains(addr) {
			return "reserved"
		}
	}
	return "public"
}

// Lookup performs no network requests, and nonpublic addresses have no invented
// country or city. A fresh searcher avoids the SDK's mutable ioCount race.
func (*Locator) Lookup(ip string) Location {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || addr.Zone() != "" {
		return Location{Network: "unknown"}
	}
	addr = addr.Unmap()
	location := Location{Network: network(addr)}
	if location.Network != "public" {
		return location
	}
	db := &ipv4
	if addr.Is6() {
		db = &ipv6
	}
	content, err := db.load()
	if err != nil {
		return location
	}
	searcher, err := xdb.NewWithBuffer(db.version, content)
	if err != nil {
		return location
	}
	region, err := searcher.Search(addr.AsSlice())
	if err != nil {
		return location
	}
	return parse(region, location.Network)
}

func parse(region, network string) Location {
	parts := strings.Split(region, "|")
	location := Location{Network: network}
	if len(parts) != 5 {
		return location
	}
	clean := func(value string) string {
		value = strings.TrimSpace(value)
		if value == "0" || value == "-" || strings.EqualFold(value, "unknown") {
			return ""
		}
		return value
	}
	location.Country, location.Region, location.City, location.ISP = clean(parts[0]), clean(parts[1]), clean(parts[2]), clean(parts[3])
	return location
}
