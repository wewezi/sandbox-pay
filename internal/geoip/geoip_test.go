package geoip

import (
	"sync"
	"testing"
)

func TestLookupClassifiesNonpublicAndInvalidAddresses(t *testing.T) {
	locator := New()
	for _, test := range []struct{ ip, network string }{
		{"127.0.0.1", "loopback"}, {"::1", "loopback"}, {"::ffff:127.0.0.1", "loopback"},
		{"10.2.3.4", "private"}, {"192.168.1.1", "private"}, {"fd12::1", "private"},
		{"169.254.1.1", "link_local"}, {"fe80::1", "link_local"},
		{"100.64.1.2", "reserved"}, {"203.0.113.1", "reserved"}, {"2001:db8::1", "reserved"},
		{"0.0.0.0", "reserved"}, {"255.255.255.255", "reserved"}, {"224.0.0.1", "reserved"},
		{"not-an-ip", "unknown"}, {"", "unknown"}, {"fe80::1%eth0", "unknown"},
	} {
		t.Run(test.ip, func(t *testing.T) {
			result := locator.Lookup(test.ip)
			if result.Network != test.network || result.Country != "" || result.City != "" || result.ISP != "" {
				t.Fatalf("Lookup(%q) = %+v", test.ip, result)
			}
		})
	}
}

func TestRegionFieldFormatAndMissingValues(t *testing.T) {
	result := parse("中国|广东省|深圳市|电信|CN", "public")
	if result.Country != "中国" || result.Region != "广东省" || result.City != "深圳市" || result.ISP != "电信" {
		t.Fatalf("region fields shifted: %+v", result)
	}
	result = parse("美国|0|0|Google|US", "public")
	if result.Region != "" || result.City != "" || result.ISP != "Google" {
		t.Fatalf("placeholders: %+v", result)
	}
}

func TestEmbeddedDatabasesResolveBothAddressFamilies(t *testing.T) {
	for _, db := range []*database{&ipv4, &ipv6} {
		if _, err := db.load(); err != nil {
			t.Fatalf("%s: %v", db.file, err)
		}
	}
	locator := New()
	for _, ip := range []string{"8.8.8.8", "223.5.5.5", "2001:4860:4860::8888"} {
		result := locator.Lookup(ip)
		if result.Network != "public" || result.Country == "" {
			t.Fatalf("unresolved %s: %+v", ip, result)
		}
	}
	if locator.Lookup("8.8.8.8") != locator.Lookup("::ffff:8.8.8.8") {
		t.Fatal("mapped IPv4 changed ownership")
	}
}

func TestConcurrentLookupsShareDataWithoutSharingMutableSearchers(t *testing.T) {
	locator := New()
	expected := locator.Lookup("223.5.5.5")
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				if result := New().Lookup("223.5.5.5"); result != expected {
					t.Errorf("inconsistent lookup: %+v", result)
					return
				}
			}
		}()
	}
	workers.Wait()
}
