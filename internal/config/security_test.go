package config

import (
	"strings"
	"testing"
)

const strongTestSecret = "f053b1d487c92a6e735d80ba9c6142efb98a630dc47e25189d5fa03c1e7264ab"

func TestValidateSecret(t *testing.T) {
	for _, value := range []string{"", "short", "your_jwt_secret_here", strings.Repeat("a", 64), strings.Repeat("abcd", 16), "your_jwt_secret_here_change_me_before_use"} {
		if err := Validate(Map{"JWT_SECRET": value}); err == nil {
			t.Fatal("accepted an unsafe JWT secret")
		} else if value != "" && strings.Contains(err.Error(), value) {
			t.Fatal("validation error disclosed the secret")
		}
	}
	if err := Validate(Map{"JWT_SECRET": strongTestSecret}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProxyConfiguration(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "127.0.0.1/33", "127.0.0.1/32,"} {
		if err := Validate(Map{"JWT_SECRET": strongTestSecret, "TRUSTED_PROXY_CIDRS": value}); err == nil {
			t.Fatal("accepted invalid proxy CIDRs")
		}
	}
	if err := Validate(Map{"JWT_SECRET": strongTestSecret, "TRUSTED_PROXY_CIDRS": "127.0.0.1/32, ::1/128", "NOTIFY_ALLOW_PRIVATE_NETWORK": "true"}); err != nil {
		t.Fatal(err)
	}
	if err := Validate(Map{"JWT_SECRET": strongTestSecret, "NOTIFY_ALLOW_PRIVATE_NETWORK": "yes"}); err == nil {
		t.Fatal("accepted ambiguous private-network configuration")
	}
}
