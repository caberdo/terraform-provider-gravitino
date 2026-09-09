package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &CatalogResource{}
var _ resource.ResourceWithImportState = &CatalogResource{}
var _ resource.ResourceWithConfigure = &CatalogResource{}

type CatalogResource struct {
	client *client.Client
}

func New() resource.Resource {
	return &CatalogResource{}
}

func (r *CatalogResource) SetClient(c *client.Client) {
	r.client = c
}

type CatalogResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Metalake   types.String `tfsdk:"metalake"`
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	Provider   types.String `tfsdk:"catalog_provider"`
	Comment    types.String `tfsdk:"comment"`
	Properties types.Map    `tfsdk:"properties"`
	Audit      types.Object `tfsdk:"audit"`
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (r *CatalogResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *CatalogResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_catalog"
}

func (r *CatalogResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Composite identifier in the format 'metalake.catalog_name'.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The catalog name.",
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "The catalog type. Must be one of: relational, fileset, messaging, model.",
				Validators: []validator.String{
					stringvalidator.OneOf("relational", "fileset", "messaging", "model"),
				},
			},
			"catalog_provider": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The catalog provider. Must be one of: hive, lakehouse-iceberg, lakehouse-paimon, lakehouse-hudi, jdbc-mysql, jdbc-postgresql, jdbc-doris, jdbc-oceanbase, kafka, fileset, model.",
				Validators: []validator.String{
					stringvalidator.OneOf("hive", "lakehouse-iceberg", "lakehouse-paimon", "lakehouse-hudi", "jdbc-mysql", "jdbc-postgresql", "jdbc-doris", "jdbc-oceanbase", "kafka", "fileset", "model"),
				},
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "A comment or description for the catalog.",
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "A map of key-value properties for the catalog.",
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the catalog.",
			},
		},
	}
}

func (r *CatalogResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan CatalogResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating catalog", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})

	createReq := &models.CatalogCreateRequest{
		Name:       plan.Name.ValueString(),
		Type:       plan.Type.ValueString(),
		Provider:   plan.Provider.ValueString(),
		Comment:    plan.Comment.ValueString(),
		Properties: mapFromTF(plan.Properties),
	}

	result, err := r.client.CreateCatalog(plan.Metalake.ValueString(), createReq)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating catalog", plan.Name.ValueString(), err)...)
		return
	}

	setStateFromCatalog(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), &result.Catalog, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Created catalog", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})
}

func (r *CatalogResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state CatalogResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	result, err := r.client.GetCatalog(state.Metalake.ValueString(), state.Name.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading catalog", state.Name.ValueString(), err)...)
		return
	}

	setStateFromCatalog(ctx, &resp.Diagnostics, state.Metalake.ValueString(), &result.Catalog, &state)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *CatalogResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state CatalogResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	var updates []interface{}

	if !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameCatalogRequest(plan.Name.ValueString()))
		state.Name = plan.Name
	}

	if !plan.Comment.Equal(state.Comment) {
		updates = append(updates, models.NewUpdateCatalogCommentRequest(plan.Comment.ValueString()))
		state.Comment = plan.Comment
	}

	oldProps := mapFromTF(state.Properties)
	newProps := mapFromTF(plan.Properties)

	for k, v := range newProps {
		oldV, exists := oldProps[k]
		if !exists || oldV != v {
			updates = append(updates, models.NewSetCatalogPropertyRequest(k, v))
		}
	}

	for k := range oldProps {
		if _, exists := newProps[k]; !exists {
			updates = append(updates, models.NewRemoveCatalogPropertyRequest(k))
		}
	}

	if len(updates) > 0 {
		result, err := r.client.UpdateCatalog(state.Metalake.ValueString(), state.Name.ValueString(), updates)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating catalog", state.Name.ValueString(), err)...)
			return
		}
		setStateFromCatalog(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), &result.Catalog, &plan)
	} else {
		setStateFromCatalog(ctx, &resp.Diagnostics, plan.Metalake.ValueString(), nil, &plan)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated catalog", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "name": plan.Name.ValueString()})
}

func (r *CatalogResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state CatalogResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DropCatalog(state.Metalake.ValueString(), state.Name.ValueString(), true)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("deleting catalog", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})
}

func (r *CatalogResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idx := strings.LastIndex(req.ID, ".")
	if idx == -1 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.catalog_name', got: %s", req.ID),
		)
		return
	}

	metalake := req.ID[:idx]
	name := req.ID[idx+1:]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func setStateFromCatalog(ctx context.Context, diags *diag.Diagnostics, metalake string, catalog *models.Catalog, model *CatalogResourceModel) {
	if catalog != nil {
		model.Name = types.StringValue(catalog.Name)
		model.Type = types.StringValue(catalog.Type)
		model.Provider = types.StringValue(catalog.Provider)
		model.Comment = types.StringValue(catalog.Comment)
		model.ID = types.StringValue(metalake + "." + catalog.Name)

		mergeCatalogProperties(ctx, diags, model, catalog.Properties)

		if catalog.Audit != nil {
			auditObj, d := auditToObjectValue(ctx, catalog.Audit)
			diags.Append(d...)
			if !diags.HasError() {
				model.Audit = auditObj
			}
		}
	} else {
		model.ID = types.StringValue(metalake + "." + model.Name.ValueString())
		mergeCatalogProperties(ctx, diags, model, nil)
	}

	model.Metalake = types.StringValue(metalake)
}

var reservedCatalogProperties = map[string]bool{
	"in-use": true,
}

func mergeCatalogProperties(ctx context.Context, diags *diag.Diagnostics, model *CatalogResourceModel, serverProps map[string]string) {
	wasNull := model.Properties.IsNull() || model.Properties.IsUnknown()

	base := make(map[string]string)
	if !wasNull {
		diags.Append(model.Properties.ElementsAs(ctx, &base, false)...)
	}

	merged := make(map[string]string)
	for k, v := range base {
		if !reservedCatalogProperties[k] {
			merged[k] = v
		}
	}
	for k, v := range serverProps {
		if reservedCatalogProperties[k] {
			continue
		}
		if _, ok := merged[k]; ok {
			merged[k] = v
		}
	}

	if len(merged) > 0 {
		props, d := types.MapValueFrom(ctx, types.StringType, merged)
		diags.Append(d...)
		if !diags.HasError() {
			model.Properties = props
		}
	} else if wasNull {
		model.Properties = types.MapNull(types.StringType)
	}
}

func auditToObjectValue(ctx context.Context, audit *models.Audit) (types.Object, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
	}

	creator := types.StringValue(audit.Creator)
	lastModifier := types.StringValue(audit.LastModifier)

	var createTime, lastModifiedTime types.String
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format("2006-01-02T15:04:05Z07:00"))
	} else {
		createTime = types.StringNull()
	}
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format("2006-01-02T15:04:05Z07:00"))
	} else {
		lastModifiedTime = types.StringNull()
	}

	attrs := map[string]attr.Value{
		"creator":            creator,
		"create_time":        createTime,
		"last_modifier":      lastModifier,
		"last_modified_time": lastModifiedTime,
	}

	return types.ObjectValue(AuditAttrTypes, attrs)
}

func mapFromTF(m types.Map) map[string]string {
	result := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return result
	}
	for k, v := range m.Elements() {
		if strVal, ok := v.(types.String); ok {
			result[k] = strVal.ValueString()
		}
	}
	return result
}
