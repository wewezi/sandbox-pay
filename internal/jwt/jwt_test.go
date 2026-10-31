package jwt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseExpiresIn(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int
	}{
		{"3600", 3600}, {" 60 ", 60}, {"24h", 86400}, {"1440m", 86400},
		{"7d", 604800}, {"15s", 15}, {"", 86400}, {"0", 86400},
		{"-1", 86400}, {"1hour", 86400}, {"9999999999999999999999999999999", 86400},
		{"9999999999999999999d", 86400},
	} {
		t.Run(test.value, func(t *testing.T) {
			if got := ParseExpiresIn(test.value); got != test.want {
				t.Fatalf("got %d, want %d", got, test.want)
			}
		})
	}
}

func TestVerifyRejectsInvalidExpiration(t *testing.T) {
	const secret = "synthetic-test-secret"
	for _, expiration := range []any{nil, "tomorrow", 1.5, time.Now().Unix() - 1, json.Number("9223372036854775808")} {
		claims := map[string]any{"sub": 1}
		if expiration != nil {
			claims["exp"] = expiration
		}
		body, err := json.Marshal(claims)
		if err != nil {
			t.Fatal(err)
		}
		head := b64url([]byte(`{"alg":"HS256","typ":"JWT"}`))
		data := head + "." + b64url(body)
		if Verify(data+"."+computeSig(data, secret), secret) != nil {
			t.Fatal("accepted missing, malformed, or expired exp")
		}
	}
	token := Sign(map[string]any{"sub": 1}, secret, 3600)
	if Verify(token, secret) == nil || Verify(token, "wrong-secret") != nil || Verify(strings.Replace(token, ".", ".tampered", 1), secret) != nil {
		t.Fatal("signature verification failed")
	}
}
