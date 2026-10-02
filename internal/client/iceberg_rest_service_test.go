package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
)

// icebergAvailableExample and icebergUnavailableExample are the verbatim
// examples of the Gravitino v1.3.1 system spec (docs/open-api/system.yaml,
// components/examples/IcebergRESTServiceAvailable and
// IcebergRESTServiceUnavailable).
const (
	icebergAvailableExample   = `{"code":0,"uri":"http://gravitino-host:9001/iceberg"}`
	icebergUnavailableExample = `{"code":0,"uri":null}`
)

func TestGetIcebergRestServiceURIAvailable(t *testing.T) {
	var gotPath, gotMetalake string
	var hadMetalake bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMetalake = r.URL.Query().Get("metalake")
		_, hadMetalake = r.URL.Query()["metalake"]
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		_, _ = w.Write([]byte(icebergAvailableExample))
	}))
	defer server.Close()

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := c.GetIcebergRestServiceURI(context.Background(), "my metalake")
	if err != nil {
		t.Fatalf("GetIcebergRestServiceURI() error = %v", err)
	}

	if gotPath != "/api/system/iceberg-rest" {
		t.Errorf("path = %q, want /api/system/iceberg-rest", gotPath)
	}
	if !hadMetalake {
		t.Error("metalake query parameter missing")
	}
	if gotMetalake != "my metalake" {
		t.Errorf("metalake = %q, want %q", gotMetalake, "my metalake")
	}
	if got.Code != 0 {
		t.Errorf("Code = %d, want 0", got.Code)
	}
	if got.URI == nil {
		t.Fatal("URI = nil, want the advertised endpoint")
	}
	if *got.URI != "http://gravitino-host:9001/iceberg" {
		t.Errorf("URI = %q", *got.URI)
	}
}

func TestGetIcebergRestServiceURIOmitsEmptyMetalake(t *testing.T) {
	var hadMetalake bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadMetalake = r.URL.Query()["metalake"]
		_, _ = w.Write([]byte(icebergUnavailableExample))
	}))
	defer server.Close()

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := c.GetIcebergRestServiceURI(context.Background(), "")
	if err != nil {
		t.Fatalf("GetIcebergRestServiceURI() error = %v", err)
	}
	if hadMetalake {
		t.Error("empty metalake must be omitted from the query, not sent blank")
	}
	if got.URI != nil {
		t.Errorf("URI = %q, want nil for the IcebergRESTServiceUnavailable example", *got.URI)
	}
}

func TestGetIcebergRestServiceURIHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":1002,"type":"NotFoundException","message":"no such endpoint"}`))
	}))
	defer server.Close()

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := c.GetIcebergRestServiceURI(context.Background(), "")
	if err == nil {
		t.Fatal("expected an error for HTTP 404")
	}
	if got != nil {
		t.Errorf("result = %+v, want nil on error", got)
	}
	if !IsNotFoundError(err) {
		t.Errorf("IsNotFoundError(%v) = false, want true", err)
	}
}

func TestIcebergRESTServiceResponseDTOMatchesSpec(t *testing.T) {
	var available models.IcebergRESTServiceResponse
	if err := json.Unmarshal([]byte(icebergAvailableExample), &available); err != nil {
		t.Fatalf("failed to decode the available example: %v", err)
	}
	if available.Code != 0 || available.URI == nil || *available.URI != "http://gravitino-host:9001/iceberg" {
		t.Errorf("decoded available example = %+v", available)
	}

	var unavailable models.IcebergRESTServiceResponse
	if err := json.Unmarshal([]byte(icebergUnavailableExample), &unavailable); err != nil {
		t.Fatalf("failed to decode the unavailable example: %v", err)
	}
	if unavailable.Code != 0 || unavailable.URI != nil {
		t.Errorf("decoded unavailable example = %+v, want uri null", unavailable)
	}
}
