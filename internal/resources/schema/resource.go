package schema

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &SchemaResource{}
var _ resource.ResourceWithImportState = &SchemaResource{}
var _ resource.ResourceWithConfigure = &SchemaResource{}

var auditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type SchemaResource struct {
	client *client.Client
}

type SchemaResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Name       types.String `tfsdk:"name"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func NewSchemaResource() resource.Resource {
	return &SchemaResource{}
}

func (r *SchemaResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_schema"
}

func (r *SchemaResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Gravitino schema within a metalake and catalog.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The compound identifier in the format metalake.catalog.schema.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"metalake": schema.StringAttribute{
				Description: "The metalake name. Changing it forces the schema to be recreated, because a schema is addressed by its metalake, catalog and name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name. Changing it forces the schema to be recreated, because a schema is addressed by its metalake, catalog and name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The schema name. Changing it forces the schema to be recreated: the Gravitino schema update API only supports property updates, not renaming.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Description: "A comment describing the schema. Changing it forces the schema to be recreated: the Gravitino schema update API only supports property updates, not comment updates.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties for the schema. Adding, changing and removing entries is applied in place; no other schema attribute can be updated in place.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the schema. The server sets last_modifier and last_modified_time on every in-place update, so this attribute deliberately has no UseStateForUnknown plan modifier: an update writes the refreshed values to state.",
				Computed:       true,
				AttributeTypes: auditAttrTypes,
			},
		},
	}
}

func (r *SchemaResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid provider data",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue.", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *SchemaResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SchemaResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating schema", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "name": plan.Name.ValueString()})

	properties := propertyMap(ctx, plan.Properties, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := &models.SchemaCreateRequest{
		Name:       plan.Name.ValueString(),
		Comment:    plan.Comment.ValueString(),
		Properties: properties,
	}

	schemaResp, err := r.client.CreateSchema(ctx, plan.Metalake.ValueString(), plan.Catalog.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating schema", plan.Name.ValueString(), err)...)
		return
	}

	r.mergeSchemaResponse(ctx, schemaResp, &plan, &resp.Diagnostics, false)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Created schema", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "name": plan.Name.ValueString()})
}

func (r *SchemaResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SchemaResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading schema", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "name": state.Name.ValueString()})

	schemaResp, err := r.client.GetSchema(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading schema", state.Name.ValueString(), err)...)
		return
	}

	r.mergeSchemaResponse(ctx, schemaResp, &state, &resp.Diagnostics, true)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SchemaResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SchemaResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating schema", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "name": state.Name.ValueString()})

	// Only `properties` can be updated in place: metalake, catalog, name and
	// comment are RequiresReplace. The Gravitino schema update API accepts
	// nothing else (SchemaUpdateRequest oneOf setProperty/removeProperty).
	var updates []interface{}

	if !plan.Properties.IsUnknown() && !plan.Properties.Equal(state.Properties) {
		oldProps := propertyMap(ctx, state.Properties, &resp.Diagnostics)
		newProps := propertyMap(ctx, plan.Properties, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}

		for k := range oldProps {
			if _, exists := newProps[k]; !exists {
				updates = append(updates, models.NewRemoveSchemaPropertyRequest(k))
			}
		}
		for k, v := range newProps {
			if oldVal, exists := oldProps[k]; !exists || oldVal != v {
				updates = append(updates, models.NewSetSchemaPropertyRequest(k, v))
			}
		}
	}

	if len(updates) == 0 {
		// Nothing to send. audit has no UseStateForUnknown plan modifier (the server
		// refreshes it on every update), so its unknown plan value is filled from the
		// prior state here. Name, comment and properties are always known.
		if plan.Audit.IsUnknown() {
			plan.Audit = state.Audit
		}
		if plan.ID.IsUnknown() {
			plan.ID = state.ID
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		tflog.Debug(ctx, "Updated schema", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "name": plan.Name.ValueString()})
		return
	}

	schemaResp, err := r.client.UpdateSchema(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Name.ValueString(), updates)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("updating schema", state.Name.ValueString(), err)...)
		return
	}

	r.mergeSchemaResponse(ctx, schemaResp, &plan, &resp.Diagnostics, false)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Updated schema", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "name": plan.Name.ValueString()})
}

func (r *SchemaResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SchemaResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting schema", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DropSchema(ctx, state.Metalake.ValueString(), state.Catalog.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: the delete is idempotent.
			tflog.Debug(ctx, "Schema already deleted", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting schema", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted schema", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "name": state.Name.ValueString()})
}

func (r *SchemaResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 3)
	if len(parts) != 3 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format 'metalake.catalog.schema', got: %q", req.ID),
		)
		return
	}
	for _, part := range parts {
		if part == "" {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				fmt.Sprintf("The metalake, catalog and schema segments must not be empty, got: %q", req.ID),
			)
			return
		}
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// propertyMap converts a Terraform map attribute into a plain Go map, appending
// any conversion diagnostics. Null and unknown maps yield an empty map.
func propertyMap(ctx context.Context, v types.Map, diags *diag.Diagnostics) map[string]string {
	out := make(map[string]string)
	if v.IsNull() || v.IsUnknown() {
		return out
	}
	diags.Append(v.ElementsAs(ctx, &out, false)...)
	return out
}

// mergeSchemaResponse copies an API schema response into the Terraform model.
//
// In apply mode (refresh == false) the values that were already known in the plan
// win: Terraform rejects an applied state that differs from the plan, while the
// server is free to normalise or extend what it stores. Only the computed
// attributes (id and audit), which are unknown in the plan, are taken from the
// response. Read (refresh == true) adopts the server's values so that drift is
// visible.
func (r *SchemaResource) mergeSchemaResponse(ctx context.Context, schemaResp *models.SchemaResponse, m *SchemaResourceModel, diags *diag.Diagnostics, refresh bool) {
	schema := schemaResp.Schema

	if refresh {
		m.Name = types.StringValue(schema.Name)
		// An empty string/empty map is not the same as null in Terraform: a
		// configuration that sets comment = "" or properties = {} plans a known empty
		// value, so the refreshed state must stay empty rather than become null.
		if schema.Comment != "" {
			m.Comment = types.StringValue(schema.Comment)
		} else if m.Comment.IsNull() {
			m.Comment = types.StringNull()
		} else {
			m.Comment = types.StringValue("")
		}
		if len(schema.Properties) > 0 {
			props, d := types.MapValueFrom(ctx, types.StringType, schema.Properties)
			diags.Append(d...)
			m.Properties = props
		} else if m.Properties.IsNull() {
			m.Properties = types.MapNull(types.StringType)
		} else {
			empty, d := types.MapValueFrom(ctx, types.StringType, map[string]string{})
			diags.Append(d...)
			m.Properties = empty
		}
	} else if m.Name.IsUnknown() || m.Name.IsNull() || m.Name.ValueString() == "" {
		m.Name = types.StringValue(schema.Name)
	}

	m.ID = types.StringValue(fmt.Sprintf("%s.%s.%s", m.Metalake.ValueString(), m.Catalog.ValueString(), m.Name.ValueString()))

	auditObj, d := auditToObject(schema.Audit)
	diags.Append(d...)
	m.Audit = auditObj
}

func auditToObject(audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(auditAttrTypes), nil
	}

	creator := types.StringNull()
	if audit.Creator != "" {
		creator = types.StringValue(audit.Creator)
	}

	createTime := types.StringNull()
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format(time.RFC3339))
	}

	lastModifier := types.StringNull()
	if audit.LastModifier != "" {
		lastModifier = types.StringValue(audit.LastModifier)
	}

	lastModifiedTime := types.StringNull()
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format(time.RFC3339))
	}

	return types.ObjectValue(auditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}
