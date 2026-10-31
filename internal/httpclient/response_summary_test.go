package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReadResponseKeepsBoundedPlainTextAndPreservesSizeLimit(t *testing.T) {
	plain := "callback accepted " + strings.Repeat("x", 3000)
	data, summary, err := readResponse(&http.Response{Body: io.NopCloser(strings.NewReader(plain))})
	if err != nil || data != nil || len(summary) != 2048 || !strings.HasPrefix(summary, "callback accepted") {
		t.Fatalf("summary bytes=%d data=%v err=%v", len(summary), data, err)
	}
	data, summary, err = readResponse(&http.Response{Body: io.NopCloser(strings.NewReader(`{"ok":true}`))})
	if err != nil || data == nil || summary != `{"ok":true}` {
		t.Fatalf("JSON data=%v summary=%s err=%v", data, summary, err)
	}
	_, _, err = readResponse(&http.Response{Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseBytes+1)))})
	if err == nil {
		t.Fatal("oversized response accepted")
	}
}
