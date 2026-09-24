package services

import (
	"errors"
	"testing"
	"time"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/httpclient"
)

// createService 缓存挂在实例上，每个用例都新建一个，不然会串。
func createRateService(t *testing.T, env map[string]string) (*ExchangeRateService, *fakeHTTP) {
	t.Helper()
	http := newFakeHTTP()
	service := NewExchangeRateService(http, config.Map(env))
	return service, http
}

// Port of server/src/services/exchange-rate.service.spec.ts
func TestExchangeRateService(t *testing.T) {
	t.Run("getRate/fetches rate from open-er-api provider", func(t *testing.T) {
		service, http := createRateService(t, map[string]string{
			"EXCHANGE_RATE_PROVIDER": "open-er-api",
		})
		http.getFn = func(string, time.Duration) (*httpclient.Response, error) {
			return &httpclient.Response{Status: 200, Data: map[string]any{
				"rates": map[string]any{"USD": 0.138},
			}}, nil
		}

		requireCloseTo(t, service.GetRate("CNY", "USD"), 0.138, 3)
	})

	t.Run("getRate/uses fallback when fetch fails", func(t *testing.T) {
		service, http := createRateService(t, map[string]string{
			"FALLBACK_CNY_TO_USD": "0.14",
		})
		http.getFn = func(string, time.Duration) (*httpclient.Response, error) {
			return nil, errors.New("Network error")
		}

		requireCloseTo(t, service.GetRate("CNY", "USD"), 0.14, 2)
	})

	t.Run("getRate/uses fallback when the provider returns a non-numeric rate", func(t *testing.T) {
		service, http := createRateService(t, map[string]string{
			"FALLBACK_CNY_TO_USD": "0.14",
		})
		http.getFn = func(string, time.Duration) (*httpclient.Response, error) {
			return &httpclient.Response{Status: 200, Data: map[string]any{
				"rates": map[string]any{"USD": "oops"},
			}}, nil
		}

		requireCloseTo(t, service.GetRate("CNY", "USD"), 0.14, 2)
	})

	t.Run("getRate/applies margin multiplier", func(t *testing.T) {
		service, http := createRateService(t, map[string]string{
			"EXCHANGE_RATE_MARGIN": "1.05",
		})
		http.getFn = func(string, time.Duration) (*httpclient.Response, error) {
			return &httpclient.Response{Status: 200, Data: map[string]any{
				"rates": map[string]any{"USD": 0.14},
			}}, nil
		}

		requireCloseTo(t, service.GetRate("CNY", "USD"), 0.14*1.05, 4)
	})

	t.Run("getRate/returns cached rate on subsequent calls", func(t *testing.T) {
		service, http := createRateService(t, map[string]string{})
		http.getFn = func(string, time.Duration) (*httpclient.Response, error) {
			return &httpclient.Response{Status: 200, Data: map[string]any{
				"rates": map[string]any{"USD": 0.138},
			}}, nil
		}

		service.GetRate("CNY", "USD")
		service.GetRate("CNY", "USD")

		if http.getCalls != 1 {
			t.Errorf("http.get called %d times, want 1", http.getCalls)
		}
	})

	t.Run("getRate/skips the HTTP call for an unknown provider and uses fallback", func(t *testing.T) {
		service, http := createRateService(t, map[string]string{
			"EXCHANGE_RATE_PROVIDER": "static",
			"FALLBACK_CNY_TO_USD":    "0.15",
		})

		requireCloseTo(t, service.GetRate("CNY", "USD"), 0.15, 2)
		if http.getCalls != 0 {
			t.Errorf("http.get called %d times, want 0 (unknown provider must skip HTTP)", http.getCalls)
		}
	})

	t.Run("getRate/falls back to CONFIG_DEFAULTS when .env has nothing set", func(t *testing.T) {
		service, http := createRateService(t, map[string]string{})
		http.getFn = func(string, time.Duration) (*httpclient.Response, error) {
			return nil, errors.New("offline")
		}

		requireCloseTo(t, service.GetRate("CNY", "USD"), 0.14, 2)
	})

	t.Run("getCnyToUsdRate/delegates to getRate(CNY, USD)", func(t *testing.T) {
		service, _ := createRateService(t, map[string]string{
			"EXCHANGE_RATE_PROVIDER": "static",
		})

		requireCloseTo(t, service.GetCnyToUsdRate(), 0.14, 2)
	})
}
