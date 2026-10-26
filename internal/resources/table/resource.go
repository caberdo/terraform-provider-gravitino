package table

import (
	"context"
	"fmt"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/providerbase"
	"github.com/gravitino/terraform-provider-gravitino/internal/resources/resourceutil"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*tableResource)(nil)
var _ resource.ResourceWithImportState = (*tableResource)(nil)
var _ resource.ResourceWithConfigure = (*tableResource)(nil)
var _ resource.ResourceWithModifyPlan = (*tableResource)(nil)
var _ resource.ResourceWithConfigValidators = (*tableResource)(nil)

type tableResource struct {
	providerbase.Resource
}

// NewTableResource returns the gravitino_table resource.
func NewTableResource() resource.Resource {
	return &tableResource{}
}

func (r *tableResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_table"
}

// Create creates the table.
func (r *tableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var tfPlan models.TableResourceTFModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &tfPlan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan := models.TableResourceModelFromTF(ctx, tfPlan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating table", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})

	r.Client.CheckExternalTypesSupported(plan.Columns, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq, diags := r.buildCreateRequest(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Gravitino 1.3.1 added the data-skipping index types and the index
	// properties; a 1.3.0 server rejects them with an opaque error, so the
	// server version is verified when the table uses either.
	if err := r.Client.CheckIndexesSupported(ctx, createReq.Indexes); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("index"), "Unsupported index", err.Error())
		return
	}

	tableResp, err := r.Client.CreateTable(
		ctx,
		plan.Metalake.ValueString(),
		plan.Catalog.ValueString(),
		plan.Schema.ValueString(),
		createReq,
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating table", plan.Name.ValueString(), err)...)
		return
	}

	// The state is built from the response, like a refresh: Gravitino assigns
	// values the configuration cannot express (the name of an index, the default
	// null ordering of a sort order, the properties a catalog adds), and a
	// computed attribute that is left unknown is a hard Terraform error.
	// The configured properties win over the reported ones.
	created := tableStateFromServer(ctx, &tableResp.Table, &plan, false, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tfCreated := models.TableResourceModelToTF(ctx, created, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &tfCreated)...)

	tflog.Debug(ctx, "Created table", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})
}

// Read refreshes the table and drops it from state when it no longer exists.

// Read refreshes the table and drops it from state when it no longer exists.
func (r *tableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var tfState models.TableResourceTFModel
	resp.Diagnostics.Append(req.State.Get(ctx, &tfState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state := models.TableResourceModelFromTF(ctx, tfState, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

	tableResp, err := r.Client.GetTable(
		ctx,
		state.Metalake.ValueString(),
		state.Catalog.ValueString(),
		state.Schema.ValueString(),
		state.Name.ValueString(),
	)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading table", state.Name.ValueString(), err)...)
		return
	}

	state = tableStateFromServer(ctx, &tableResp.Table, &state, true, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tfState = models.TableResourceModelToTF(ctx, state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &tfState)...)

	tflog.Debug(ctx, "Read table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})
}

// Update applies the table updates of tables.yaml.

// Update applies the table updates of tables.yaml.
func (r *tableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var tfPlan, tfState models.TableResourceTFModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &tfPlan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &tfState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan := models.TableResourceModelFromTF(ctx, tfPlan, &resp.Diagnostics)
	state := models.TableResourceModelFromTF(ctx, tfState, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

	r.Client.CheckExternalTypesSupported(plan.Columns, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// These attributes have no update request in tables.yaml. A change normally
	// turns into a replacement during plan (see ModifyPlan); reaching this guard
	// means the plan contained unknown values, in which case reporting an error
	// is better than silently ignoring the change.
	if changed := immutableBlockChange(plan, state); changed != "" {
		resp.Diagnostics.AddError(
			fmt.Sprintf("%s cannot be updated in place", changed),
			fmt.Sprintf("Gravitino v1.3.0 defines no update request for %s of an existing table, so changing it forces a "+
				"new table. The change could not be detected while planning because the configured value was not known "+
				"yet; make the value known in the plan or drop the table explicitly.", changed),
		)
		return
	}

	if columnsReplaceRequired(plan.Columns, state.Columns) {
		resp.Diagnostics.AddError(
			"column cannot be updated in place",
			"The set of column names changed, or a column default value was removed. Gravitino cannot tell a renamed "+
				"column from a deleted and added one, and it cannot clear a column default value, so such a change "+
				"forces a new table. The change could not be detected while planning because the configured value was "+
				"not known yet.",
		)
		return
	}

	var updates []models.TableUpdateRequest

	if !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameTableRequest(plan.Name.ValueString()))
	}

	if !plan.Comment.Equal(state.Comment) {
		updates = append(updates, models.NewUpdateTableCommentRequest(plan.Comment.ValueString()))
	}

	if !plan.Properties.IsUnknown() {
		propertyUpdates, propertyDiags := tablePropertyUpdates(ctx, plan.Properties, state.Properties)
		resp.Diagnostics.Append(propertyDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		updates = append(updates, propertyUpdates...)
	}

	columnUpdates, columnDiags := tableColumnUpdates(plan.Columns, state.Columns)
	resp.Diagnostics.Append(columnDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updates = append(updates, columnUpdates...)

	if len(updates) > 0 {
		if _, err := r.Client.UpdateTable(
			ctx,
			state.Metalake.ValueString(),
			state.Catalog.ValueString(),
			state.Schema.ValueString(),
			state.Name.ValueString(),
			updates,
		); err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating table", state.Name.ValueString(), err)...)
			return
		}
	}

	tableResp, err := r.Client.GetTable(
		ctx,
		plan.Metalake.ValueString(),
		plan.Catalog.ValueString(),
		plan.Schema.ValueString(),
		plan.Name.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading table after update", state.Name.ValueString(), err)...)
		return
	}

	// The applied state is built from the table Gravitino reports back, with the
	// properties of the configuration winning over the reported ones.
	updated := tableStateFromServer(ctx, &tableResp.Table, &plan, false, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tfUpdated := models.TableResourceModelToTF(ctx, updated, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &tfUpdated)...)

	tflog.Debug(ctx, "Updated table", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})
}

// Delete drops the table. A table that is already gone is treated as deleted.

// Delete drops the table. A table that is already gone is treated as deleted.
func (r *tableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var tfState models.TableResourceTFModel
	resp.Diagnostics.Append(req.State.Get(ctx, &tfState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state := models.TableResourceModelFromTF(ctx, tfState, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Dropping table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

	_, err := r.Client.DropTable(
		ctx,
		state.Metalake.ValueString(),
		state.Catalog.ValueString(),
		state.Schema.ValueString(),
		state.Name.ValueString(),
	)
	if err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.Append(client.NewResourceError("dropping table", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Dropped table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})
}

// ImportState imports a table from a metalake.catalog.schema.table id.

// ImportState imports a table from a metalake.catalog.schema.table id.
func (r *tableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := resourceutil.SplitImportID(req, resp, 4, "metalake.catalog.schema.table")
	if !ok {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// buildCreateRequest converts the planned table into a TableCreateRequest.

func tableID(metalake, catalog, schema, name string) string {
	return fmt.Sprintf("%s.%s.%s.%s", metalake, catalog, schema, name)
}

// dataTypeStringValidator validates that a column type is a primitive type
// name or a structured data type that Gravitino understands.
