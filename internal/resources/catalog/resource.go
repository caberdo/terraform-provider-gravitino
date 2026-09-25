package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &CatalogResource{}
var _ resource.ResourceWithImportState = &CatalogResource{}
var _ resource.ResourceWithConfigure = &CatalogResource{}
var _ resource.ResourceWithModifyPlan = &CatalogResource{}

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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The catalog name.",
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "The catalog type. Must be one of: relational, fileset, messaging, model. Changing the type forces a new catalog: the Gravitino API has no update request for it.",
				Validators: []validator.String{
					stringvalidator.OneOf("relational", "fileset", "messaging", "model"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog_provider": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The catalog provider. Must be one of: hive, lakehouse-iceberg, lakehouse-paimon, lakehouse-hudi, jdbc-mysql, jdbc-postgresql, jdbc-doris, jdbc-oceanbase, kafka, fileset, model. Changing the provider forces a new catalog: the Gravitino API has no update request for it.",
				Validators: []validator.String{
					stringvalidator.OneOf("hive", "lakehouse-iceberg", "lakehouse-paimon", "lakehouse-hudi", "jdbc-mysql", "jdbc-postgresql", "jdbc-doris", "jdbc-oceanbase", "kafka", "fileset", "model"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "A comment or description for the catalog. The Gravitino API can only replace an existing comment (updateComment), never remove it.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				ElementType: types.StringType,
				Description: "A map of key-value properties for the catalog. May contain credentials (for example JDBC passwords or S3 keys), therefore it is marked sensitive. The reserved 'in-use' key is managed by Gravitino (PATCH .../catalogs/{name}) and cannot be configured here.",
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.NoneOf("in-use")),
				},
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				Description:    "Audit information for the catalog.",
				// No UseStateForUnknown: the server rewrites lastModifier and
				// lastModifiedTime on every update, so the plan must stay
				// unknown here, otherwise the applied value would differ from
				// the planned value. Every code path writes a known audit.
			},
		},
	}
}

// ModifyPlan marks the id as unknown when the catalog is renamed in place: the
// identifier embeds the name, so pinning the planned id to the prior state
// would make the applied id differ from the planned one ("Provider produced
// inconsistent result after apply").
func (r *CatalogResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Nothing to pin down while creating or destroying.
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan, state CatalogResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), types.StringUnknown())...)
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

	result, err := r.client.CreateCatalog(ctx, plan.Metalake.ValueString(), createReq)
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

	result, err := r.client.GetCatalog(ctx, state.Metalake.ValueString(), state.Name.ValueString())
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

	// The update request targets the *current* name; a rename is sent as an
	// update request whose effect is visible in the response.
	metalake := state.Metalake.ValueString()
	currentName := state.Name.ValueString()

	var updates []interface{}

	if !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameCatalogRequest(plan.Name.ValueString()))
	}

	if !plan.Comment.IsUnknown() && !plan.Comment.Equal(state.Comment) {
		updates = append(updates, models.NewUpdateCatalogCommentRequest(plan.Comment.ValueString()))
	}

	if !plan.Properties.IsUnknown() {
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
	}

	if len(updates) > 0 {
		result, err := r.client.UpdateCatalog(ctx, metalake, currentName, updates)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating catalog", currentName, err)...)
			return
		}
		setStateFromCatalog(ctx, &resp.Diagnostics, metalake, &result.Catalog, &plan)
	} else {
		// Nothing changed: keep the planned values. The audit is server-managed
		// and therefore unknown in the plan, so it must be filled with a known
		// value explicitly.
		plan.Audit = state.Audit
		setStateFromCatalog(ctx, &resp.Diagnostics, metalake, nil, &plan)
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

	_, err := r.client.DropCatalog(ctx, state.Metalake.ValueString(), state.Name.ValueString(), true)
	if err != nil {
		if client.IsNotFoundError(err) {
			// Already gone: deleting twice is not an error.
			tflog.Debug(ctx, "Catalog already deleted", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting catalog", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted catalog", map[string]interface{}{"metalake": state.Metalake.ValueString(), "name": state.Name.ValueString()})
}

func (r *CatalogResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idx := strings.LastIndex(req.ID, ".")
	if idx <= 0 || idx == len(req.ID)-1 {
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

// mergeCatalogProperties stores exactly the properties of the plan: server
// values are used for the keys the plan contains, but server-only keys (for
// example "in-use", which Gravitino adds to every catalog) are never added.
//
// properties is Optional+Computed: the applied value must equal the planned
// value, otherwise Terraform fails the apply with "Provider produced
// inconsistent result after apply". A configured empty map stays an empty map
// (only a null plan stays null).
func mergeCatalogProperties(ctx context.Context, diags *diag.Diagnostics, model *CatalogResourceModel, serverProps map[string]string) {
	if model.Properties.IsUnknown() {
		model.Properties = types.MapNull(types.StringType)
		return
	}

	if model.Properties.IsNull() {
		// No configured properties (for example while importing): adopt the
		// server's properties so that an import reconstructs the resource,
		// minus the keys Gravitino manages itself.
		serverOnly := make(map[string]string)
		for k, v := range serverProps {
			if !reservedCatalogProperties[k] {
				serverOnly[k] = v
			}
		}
		if len(serverOnly) == 0 {
			model.Properties = types.MapNull(types.StringType)
			return
		}
		props, d := types.MapValueFrom(ctx, types.StringType, serverOnly)
		diags.Append(d...)
		if !diags.HasError() {
			model.Properties = props
		}
		return
	}

	planned := make(map[string]string)
	diags.Append(model.Properties.ElementsAs(ctx, &planned, false)...)
	if diags.HasError() {
		return
	}

	merged := make(map[string]string, len(planned))
	for k, v := range planned {
		if serverValue, ok := serverProps[k]; ok {
			merged[k] = serverValue
			continue
		}
		merged[k] = v
	}

	props, d := types.MapValueFrom(ctx, types.StringType, merged)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Properties = props
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
