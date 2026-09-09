package model_version

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

var _ resource.Resource = &ModelVersionResource{}
var _ resource.ResourceWithImportState = &ModelVersionResource{}
var _ resource.ResourceWithConfigure = &ModelVersionResource{}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

type ModelVersionResource struct {
	client *client.Client
}

func NewModelVersionResource() resource.Resource {
	return &ModelVersionResource{}
}

func (r *ModelVersionResource) SetClient(c *client.Client) {
	r.client = c
}

type ModelVersionResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Model      types.String `tfsdk:"model"`
	Version    types.String `tfsdk:"version"`
	URI        types.String `tfsdk:"uri"`
	Aliases    types.Set    `tfsdk:"aliases"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

func (r *ModelVersionResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_model_version"
}

func (r *ModelVersionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Gravitino model version within a metalake, catalog, schema, and model.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The compound identifier in the format metalake.catalog.schema.model.version.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"metalake": schema.StringAttribute{
				Description: "The metalake name.",
				Required:    true,
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog name.",
				Required:    true,
			},
			"schema": schema.StringAttribute{
				Description: "The schema name.",
				Required:    true,
			},
			"model": schema.StringAttribute{
				Description: "The model name.",
				Required:    true,
			},
			"version": schema.StringAttribute{
				Description: "The model version identifier.",
				Required:    true,
			},
			"uri": schema.StringAttribute{
				Description: "The URI of the model version artifact.",
				Optional:    true,
				Computed:    true,
			},
			"aliases": schema.SetAttribute{
				Description: "Aliases for this model version.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"comment": schema.StringAttribute{
				Description: "A comment describing the model version.",
				Optional:    true,
				Computed:    true,
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties for the model version.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information for the model version.",
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
			},
		},
	}
}

func (r *ModelVersionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ModelVersionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ModelVersionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating model version", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "model": plan.Model.ValueString(), "version": plan.Version.ValueString()})

	aliases, d := stringListFromTF(ctx, plan.Aliases)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	properties := make(map[string]string)
	if !plan.Properties.IsNull() && !plan.Properties.IsUnknown() {
		resp.Diagnostics.Append(plan.Properties.ElementsAs(ctx, &properties, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	createReq := &models.ModelVersionLinkRequest{
		Version:    plan.Version.ValueString(),
		URI:        plan.URI.ValueString(),
		Aliases:    aliases,
		Comment:    plan.Comment.ValueString(),
		Properties: properties,
	}

	result, err := r.client.LinkModelVersion(plan.Metalake.ValueString(), plan.Catalog.ValueString(), plan.Schema.ValueString(), plan.Model.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating model version", plan.Version.ValueString(), err)...)
		return
	}

	r.readModelVersionToState(ctx, result, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created model version", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "model": plan.Model.ValueString(), "version": plan.Version.ValueString()})
}

func (r *ModelVersionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ModelVersionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueString()})

	result, err := r.client.GetModelVersion(state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Model.ValueString(), state.Version.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading model version", state.Version.ValueString(), err)...)
		return
	}

	r.readModelVersionToState(ctx, result, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *ModelVersionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ModelVersionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueString()})

	planAliases, d := stringListFromTF(ctx, plan.Aliases)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	properties := make(map[string]string)
	if !plan.Properties.IsNull() && !plan.Properties.IsUnknown() {
		resp.Diagnostics.Append(plan.Properties.ElementsAs(ctx, &properties, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	needsUpdate := !plan.URI.Equal(state.URI) ||
		!plan.Comment.Equal(state.Comment) ||
		!plan.Aliases.Equal(state.Aliases) ||
		!plan.Properties.Equal(state.Properties)

	if needsUpdate {
		updateReq := &models.ModelVersionLinkRequest{
			URI:        plan.URI.ValueString(),
			Aliases:    planAliases,
			Comment:    plan.Comment.ValueString(),
			Properties: properties,
		}

		result, err := r.client.UpdateModelVersion(state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Model.ValueString(), state.Version.ValueString(), updateReq)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating model version", state.Version.ValueString(), err)...)
			return
		}
		r.readModelVersionToState(ctx, result, &plan, &resp.Diagnostics)
	} else {
		r.readModelVersionToState(ctx, nil, &plan, &resp.Diagnostics)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated model version", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "model": plan.Model.ValueString(), "version": plan.Version.ValueString()})
}

func (r *ModelVersionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ModelVersionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueString()})

	_, err := r.client.DeleteModelVersion(state.Metalake.ValueString(), state.Catalog.ValueString(), state.Schema.ValueString(), state.Model.ValueString(), state.Version.ValueString())
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("deleting model version", state.Version.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted model version", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "model": state.Model.ValueString(), "version": state.Version.ValueString()})
}

func (r *ModelVersionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 5)
	if len(parts) != 5 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format: metalake.catalog.schema.model.version")
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("model"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("version"), parts[4])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (r *ModelVersionResource) readModelVersionToState(ctx context.Context, mvResp *models.ModelVersionResponse, m *ModelVersionResourceModel, diags *diag.Diagnostics) {
	m.Version = types.StringValue(m.Version.ValueString())
	m.Metalake = types.StringValue(m.Metalake.ValueString())
	m.Catalog = types.StringValue(m.Catalog.ValueString())
	m.Schema = types.StringValue(m.Schema.ValueString())
	m.Model = types.StringValue(m.Model.ValueString())

	if mvResp != nil {
		mv := mvResp.ModelVersion
		m.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s.%s", m.Metalake.ValueString(), m.Catalog.ValueString(), m.Schema.ValueString(), m.Model.ValueString(), mv.Version))
		m.Version = types.StringValue(mv.Version)
		m.URI = types.StringValue(mv.URI)
		m.Comment = types.StringValue(mv.Comment)

		if len(mv.Aliases) > 0 {
			aliasesSet, d := types.SetValueFrom(ctx, types.StringType, mv.Aliases)
			diags.Append(d...)
			m.Aliases = aliasesSet
		} else {
			m.Aliases = types.SetNull(types.StringType)
		}

		if len(mv.Properties) > 0 {
			props, d := types.MapValueFrom(ctx, types.StringType, mv.Properties)
			diags.Append(d...)
			m.Properties = props
		} else {
			m.Properties = types.MapNull(types.StringType)
		}

		auditObj, d := auditToObject(mv.Audit)
		diags.Append(d...)
		m.Audit = auditObj
	} else {
		m.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s.%s", m.Metalake.ValueString(), m.Catalog.ValueString(), m.Schema.ValueString(), m.Model.ValueString(), m.Version.ValueString()))
	}
}

func auditToObject(audit *models.Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
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

	return types.ObjectValue(AuditAttrTypes, map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	})
}

func stringListFromTF(ctx context.Context, list types.Set) ([]string, diag.Diagnostics) {
	if list.IsNull() || list.IsUnknown() {
		return nil, nil
	}
	var result []string
	diags := list.ElementsAs(ctx, &result, false)
	return result, diags
}
