package realip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(remote, xff, xri string) *http.Request {
	r := httptest.NewRequest("POST", "/x", nil)
	r.RemoteAddr = remote
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	if xri != "" {
		r.Header.Set("X-Real-Ip", xri)
	}
	return r
}

func TestClient(t *testing.T) {
	cases := []struct {
		name       string
		remote     string
		xff, xri   string
		trustProxy bool
		want       string
	}{
		{"no proxy", "10.0.0.1:5555", "", "", false, "10.0.0.1"},
		{"no proxy ignores headers", "10.0.0.1:5555", "1.2.3.4", "9.9.9.9", false, "10.0.0.1"},
		{"real ip", "10.0.0.1:5555", "", "203.0.113.7", true, "203.0.113.7"},
		{"xff last entry", "10.0.0.1:5555", "1.2.3.4, 5.6.7.8", "", true, "5.6.7.8"},
		{"real ip wins over xff", "10.0.0.1:5555", "1.2.3.4", "203.0.113.7", true, "203.0.113.7"},
		{"no headers falls back", "10.0.0.1:5555", "", "", true, "10.0.0.1"},
		{"bare remote no port", "10.0.0.2", "", "", false, "10.0.0.2"},
	}
	for _, tc := range cases {
		if got := Client(req(tc.remote, tc.xff, tc.xri), tc.trustProxy); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}
