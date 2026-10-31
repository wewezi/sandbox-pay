package config

import (
	"fmt"
	"net/netip"
	"strings"
)

// Validate checks settings whose unsafe defaults must prevent startup. Errors
// identify the setting without including its value.
func Validate(c Config) error {
	secret := strings.TrimSpace(c.Get("JWT_SECRET"))
	if len(secret) < 32 {
		return fmt.Errorf("JWT_SECRET must contain at least 32 bytes of random data")
	}
	lower := strings.ToLower(secret)
	for _, marker := range []string{"your_jwt", "your-secret", "your_secret", "changeme", "change-me", "change_me", "replace", "placeholder", "example", "not-for-production", "not_for_production"} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("JWT_SECRET must be generated randomly, not a placeholder")
		}
	}
	for period := 1; period <= len(secret)/2; period++ {
		if len(secret)%period == 0 && strings.Repeat(secret[:period], len(secret)/period) == secret {
			return fmt.Errorf("JWT_SECRET must not be a repeated pattern")
		}
	}
	distinct := make(map[byte]bool)
	for i := range secret {
		distinct[secret[i]] = true
	}
	if len(distinct) < 8 {
		return fmt.Errorf("JWT_SECRET must contain sufficiently varied random data")
	}
	if _, err := TrustedProxyCIDRs(c); err != nil {
		return err
	}
	if value := strings.TrimSpace(c.Get("NOTIFY_ALLOW_PRIVATE_NETWORK")); value != "" && value != "true" && value != "false" && value != "1" && value != "0" {
		return fmt.Errorf("NOTIFY_ALLOW_PRIVATE_NETWORK must be true or false")
	}
	return nil
}

// TrustedProxyCIDRs is empty by default: forwarding headers then have no effect.
func TrustedProxyCIDRs(c Config) ([]netip.Prefix, error) {
	raw := strings.TrimSpace(c.Get("TRUSTED_PROXY_CIDRS"))
	if raw == "" {
		return nil, nil
	}
	var prefixes []netip.Prefix
	for _, item := range strings.Split(raw, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXY_CIDRS must be a comma-separated list of IP CIDRs")
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}
