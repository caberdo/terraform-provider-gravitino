package auth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHasNegotiateChallenge(t *testing.T) {
	tests := map[string]struct {
		header string
		want   bool
	}{
		"real spnego challenge with token": {header: "Negotiate YIIFhgYGKwYBBQUCoIIF", want: true},
		"bare negotiate":                   {header: "Negotiate", want: true},
		"trailing space":                   {header: "Negotiate ", want: true},
		"leading space":                    {header: " Negotiate abc", want: true},
		"basic auth":                       {header: `Basic realm="x"`, want: false},
		"empty":                            {header: "", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tc.header != "" {
				resp.Header.Set("WWW-Authenticate", tc.header)
			}
			if got := hasNegotiateChallenge(resp); got != tc.want {
				t.Errorf("hasNegotiateChallenge(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}

func TestCloneRequestForRetry_RewindsBody(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/api/metalakes", strings.NewReader(`{"name":"ml"}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	// Simulate the first attempt consuming the body.
	if _, err := io.ReadAll(req.Body); err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	retry, err := cloneRequestForRetry(req)
	if err != nil {
		t.Fatalf("cloneRequestForRetry() error = %v", err)
	}

	got, err := io.ReadAll(retry.Body)
	if err != nil {
		t.Fatalf("ReadAll(retry.Body) error = %v", err)
	}
	if want := `{"name":"ml"}`; string(got) != want {
		t.Errorf("retried body = %q, want %q", got, want)
	}
}

func TestCloneRequestForRetry_NonRewindableBody(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com/api/metalakes", io.NopCloser(bytes.NewBufferString("x")))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.GetBody = nil

	if _, err := cloneRequestForRetry(req); err == nil {
		t.Error("cloneRequestForRetry() error = nil for a non-rewindable body, want an error")
	}
}
