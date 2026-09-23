package server

import (
	"net/http"
	"testing"
)

func TestFrameBlockedByHeaders(t *testing.T) {
	cases := []struct {
		name string
		h    http.Header
		want bool
	}{
		{"none", http.Header{}, false},
		{"xfo sameorigin", http.Header{"X-Frame-Options": {"SAMEORIGIN"}}, true},
		{"xfo deny lower", http.Header{"X-Frame-Options": {"deny"}}, true},
		{"csp self", http.Header{"Content-Security-Policy": {"default-src *; frame-ancestors 'self'"}}, true},
		{"csp star", http.Header{"Content-Security-Policy": {"frame-ancestors *"}}, false},
		{"csp without frame-ancestors", http.Header{"Content-Security-Policy": {"script-src 'self'"}}, false},
	}
	for _, c := range cases {
		if got := frameBlockedByHeaders(c.h); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
