// Package providerbase holds the embeddable types that give every resource and
// data source the same Configure behaviour. Embedding one of these types
// satisfies the framework's ResourceWithConfigure / DataSourceWithConfigure
// interface, so a resource only declares the field it needs:
//
//	type TagResource struct {
//		providerbase.Resource
//	}
//
// Its CRUD methods then read the client from r.Client. Tests inject a client
// with the promoted SetClient, which keeps the production Configure path
// untouched.
package providerbase

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Resource is embedded by every resource. Client is nil until Configure or
// SetClient sets it.
type Resource struct {
	Client *client.Client
}

// Configure resolves the shared client from the provider data. A nil
// ProviderData means the provider has not been configured yet, which is not an
// error.
func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		r.Client = c
	}
}

// SetClient injects the API client; used by tests.
func (r *Resource) SetClient(c *client.Client) {
	r.Client = c
}

// DataSource is embedded by every data source; it is the data-source
// counterpart of Resource.
type DataSource struct {
	Client *client.Client
}

// Configure resolves the shared client from the provider data.
func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	c, diags := client.FromProviderData(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if c != nil {
		d.Client = c
	}
}

// SetClient injects the API client; used by tests.
func (d *DataSource) SetClient(c *client.Client) {
	d.Client = c
}
