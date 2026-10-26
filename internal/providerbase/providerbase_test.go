package providerbase_test

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/providerbase"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestResourceConfigure(t *testing.T) {
	var base providerbase.Resource

	// A nil ProviderData means the provider is not configured yet.
	resp := &resource.ConfigureResponse{}
	base.Configure(context.Background(), resource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("nil provider data must not produce diagnostics: %v", resp.Diagnostics)
	}
	if base.Client != nil {
		t.Fatal("nil provider data must leave the client unset")
	}

	// Unexpected provider data is an error and leaves the client unset.
	resp = &resource.ConfigureResponse{}
	base.Configure(context.Background(), resource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for unexpected provider data")
	}
	if base.Client != nil {
		t.Fatal("a rejected provider data value must leave the client unset")
	}

	c, err := client.New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	resp = &resource.ConfigureResponse{}
	base.Configure(context.Background(), resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid provider data must not produce diagnostics: %v", resp.Diagnostics)
	}
	if base.Client != c {
		t.Fatal("Configure did not store the client")
	}
}

func TestResourceSetClient(t *testing.T) {
	c, err := client.New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	var base providerbase.Resource
	base.SetClient(c)
	if base.Client != c {
		t.Fatal("SetClient did not store the client")
	}
}

func TestDataSourceConfigure(t *testing.T) {
	var base providerbase.DataSource

	resp := &datasource.ConfigureResponse{}
	base.Configure(context.Background(), datasource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("nil provider data must not produce diagnostics: %v", resp.Diagnostics)
	}
	if base.Client != nil {
		t.Fatal("nil provider data must leave the client unset")
	}

	resp = &datasource.ConfigureResponse{}
	base.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic for unexpected provider data")
	}
	if base.Client != nil {
		t.Fatal("a rejected provider data value must leave the client unset")
	}

	c, err := client.New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	resp = &datasource.ConfigureResponse{}
	base.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("valid provider data must not produce diagnostics: %v", resp.Diagnostics)
	}
	if base.Client != c {
		t.Fatal("Configure did not store the client")
	}
}

func TestDataSourceSetClient(t *testing.T) {
	c, err := client.New("http://127.0.0.1:1", nil)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	var base providerbase.DataSource
	base.SetClient(c)
	if base.Client != c {
		t.Fatal("SetClient did not store the client")
	}
}
