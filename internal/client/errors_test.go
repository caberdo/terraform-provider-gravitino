package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Gravitino v1.3.0 docs/open-api/metalakes.yaml, components/examples/NoSuchMetalakeException.
// Note the application code (1003) and the fact that "does not exist" only appears in
// the deprecated stack field: neither can be used to detect a 404.
const realNotFoundBody = `{"code":1003,"type":"NoSuchMetalakeException",` +
	`"message":"Failed to operate metalake(s) [test] operation [LOAD], reason [NoSuchMetalakeException]",` +
	`"stack":["org.apache.gravitino.exceptions.NoSuchMetalakeException: Metalake test does not exist"]}`

func TestIsNotFoundError_RealGravitinoErrorPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(realNotFoundBody))
	}))
	defer srv.Close()

	c, err := New(srv.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	var out map[string]interface{}
	err = c.Get(context.Background(), "/metalakes/test", &out)
	if err == nil {
		t.Fatal("Get() error = nil, want a 404 error")
	}
	if !IsNotFoundError(err) {
		t.Errorf("IsNotFoundError(%v) = false, want true", err)
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error is not an *HTTPError: %T", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", httpErr.StatusCode)
	}
	if httpErr.Response == nil || httpErr.Response.Type != "NoSuchMetalakeException" {
		t.Errorf("Response = %+v, want the decoded Gravitino error payload", httpErr.Response)
	}
}

func TestIsNotFoundError_NonJSONAndEmptyBodies(t *testing.T) {
	for name, body := range map[string]string{
		"empty":     "",
		"plainText": "404 page not found",
		"html":      "<html><body>Not Found</body></html>",
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			c, _ := New(srv.URL, nil)
			var out map[string]interface{}
			err := c.Get(context.Background(), "/metalakes/test", &out)
			if !IsNotFoundError(err) {
				t.Errorf("IsNotFoundError(%v) = false, want true", err)
			}
		})
	}
}

func TestIsNotFoundError_NonNotFoundStatuses(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusConflict, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"code": 1001, "type": "RuntimeException", "message": "boom",
			})
		}))

		c, _ := New(srv.URL, nil)
		var out map[string]interface{}
		err := c.Get(context.Background(), "/metalakes/test", &out)
		if IsNotFoundError(err) {
			t.Errorf("IsNotFoundError(%v) = true for status %d, want false", err, status)
		}
		srv.Close()
	}
}

func TestNewResourceError_SurfacesStatusTypeAndStack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(realNotFoundBody))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, nil)
	var out map[string]interface{}
	err := c.Get(context.Background(), "/metalakes/test", &out)

	diags := NewResourceError("reading metalake", "test", err)
	if !diags.HasError() {
		t.Fatal("expected error diagnostics")
	}
	combined := ""
	for _, d := range diags {
		combined += d.Summary() + " | " + d.Detail() + "\n"
	}
	for _, want := range []string{"403", "NoSuchMetalakeException", "Server Stack Trace"} {
		if !strings.Contains(combined, want) {
			t.Errorf("diagnostics missing %q:\n%s", want, combined)
		}
	}
}

func TestDo_EmptyBodyIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, nil)
	var out map[string]interface{}
	if err := c.Delete(context.Background(), "/metalakes/test", &out); err != nil {
		t.Errorf("Delete() with empty 204 body error = %v, want nil", err)
	}
}

func TestDo_PropagatesContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c, _ := New(srv.URL, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out map[string]interface{}
	err := c.Get(ctx, "/metalakes/test", &out)
	if err == nil {
		t.Fatal("Get() with cancelled context error = nil, want an error")
	}
}
