package partition

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*PartitionResource)(nil)
var _ resource.ResourceWithImportState = (*PartitionResource)(nil)
var _ resource.ResourceWithConfigure = (*PartitionResource)(nil)
var _ resource.ResourceWithModifyPlan = (*PartitionResource)(nil)

// PartitionResource manages a single partition of a Gravitino table.
type PartitionResource struct {
	client *client.Client
}

// PartitionResourceModel is the Terraform model of a partition.
type PartitionResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Table      types.String `tfsdk:"table"`
	Type       types.String `tfsdk:"type"`
	Name       types.String `tfsdk:"name"`
	FieldNames types.List   `tfsdk:"field_names"`
	Values     types.List   `tfsdk:"values"`
	Upper      types.Object `tfsdk:"upper"`
	Lower      types.Object `tfsdk:"lower"`
	Lists      types.List   `tfsdk:"lists"`
	Properties types.Map    `tfsdk:"properties"`
}

// NewPartitionResource returns the gravitino_partition resource.
func NewPartitionResource() resource.Resource {
	return &PartitionResource{}
}

// SetClient sets the API client of the resource.
func (r *PartitionResource) SetClient(c *client.Client) {
	r.client = c
}

// Metadata sets the Terraform type name of the resource.
func (r *PartitionResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_partition"
}

// Schema defines the gravitino_partition schema.
//
// Gravitino v1.3.0 only exposes "add partitions" and "drop partition" for
// partitions (see partitions.yaml): there is no request that renames a
// partition or changes its field names, values or properties. Every attribute
// therefore forces a replacement, which is the only way to apply a change
// without silently ignoring it.
func (r *PartitionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a partition of a Gravitino table. Gravitino v1.3.0 has no API to modify an existing " +
			"partition, so every change to this resource replaces the partition.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The compound identifier in the format metalake.catalog.schema.table.partition.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					models.CompoundID("metalake", "catalog", "schema", "table", "name"),
				},
			},
			"metalake": schema.StringAttribute{
				Description: "The metalake the partition belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog the partition belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Description: "The schema the partition belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"table": schema.StringAttribute{
				Description: "The table the partition belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Description: "The partition type: identity, range or list.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(models.PartitionTypeIdentity, models.PartitionTypeRange, models.PartitionTypeList),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The partition name. Required for range and list partitions. The catalog derives the " +
					"name of an identity partition from the field names and values, so any value configured for an " +
					"identity partition is ignored and the name reported by Gravitino is stored instead.",
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"field_names": schema.ListAttribute{
				Description: "The identity partition fields, one entry per field, each entry holding the path segments " +
					"of the field. The values must be listed in the same order.",
				Optional:    true,
				ElementType: types.ListType{ElemType: types.StringType},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"values": schema.ListAttribute{
				Description: "The identity partition values, one literal per entry of field_names.",
				Optional:    true,
				ElementType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"upper": schema.ObjectAttribute{
				Description:    "The exclusive upper bound of a range partition.",
				Optional:       true,
				AttributeTypes: models.PartitionLiteralAttrTypes,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"lower": schema.ObjectAttribute{
				Description:    "The inclusive lower bound of a range partition.",
				Optional:       true,
				AttributeTypes: models.PartitionLiteralAttrTypes,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"lists": schema.ListAttribute{
				Description: "The value lists of a list partition, one entry per list, each entry holding the literals " +
					"of that list.",
				Optional:    true,
				ElementType: types.ListType{ElemType: types.ObjectType{AttrTypes: models.PartitionLiteralAttrTypes}},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"properties": schema.MapAttribute{
				Description: "Key-value properties of the partition. The catalog implementation may add its own " +
					"properties (Hive adds table statistics); those are only reported when no properties are managed " +
					"by this resource.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Configure sets the API client from the provider data.
func (r *PartitionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan marks the name of an identity partition as unknown, because the
// catalog derives it from the field names and values.
func (r *PartitionResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan PartitionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Type.ValueString() != models.PartitionTypeIdentity {
		return
	}
	if plan.Name.IsUnknown() {
		return
	}

	plan.Name = types.StringUnknown()
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

// Create adds the partition to the table.
func (r *PartitionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PartitionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Adding partition", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"table":    plan.Table.ValueString(),
		"type":     plan.Type.ValueString(),
	})

	validatePartitionSpec(&plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	partition := partitionFromModel(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.AddPartitions(
		ctx,
		plan.Metalake.ValueString(),
		plan.Catalog.ValueString(),
		plan.Schema.ValueString(),
		plan.Table.ValueString(),
		[]models.Partition{partition},
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("adding partition", plan.Name.ValueString(), err)...)
		return
	}
	if len(result.Partitions) == 0 {
		resp.Diagnostics.AddError(
			"Partition not returned",
			"Gravitino accepted the add partitions request but returned no partition.",
		)
		return
	}

	setPartitionState(ctx, &result.Partitions[0], &plan, plan.Properties, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Added partition", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"table":    plan.Table.ValueString(),
		"name":     plan.Name.ValueString(),
	})
}

// Read refreshes the partition and drops it from state when it no longer exists.
func (r *PartitionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PartitionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading partition", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"table":    state.Table.ValueString(),
		"name":     state.Name.ValueString(),
	})

	partitionResp, err := r.client.GetPartition(
		ctx,
		state.Metalake.ValueString(),
		state.Catalog.ValueString(),
		state.Schema.ValueString(),
		state.Table.ValueString(),
		state.Name.ValueString(),
	)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading partition", state.Name.ValueString(), err)...)
		return
	}

	setPartitionState(ctx, &partitionResp.Partition, &state, state.Properties, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read partition", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"table":    state.Table.ValueString(),
		"name":     state.Name.ValueString(),
	})
}

// Update is unreachable: every attribute of gravitino_partition uses
// RequiresReplace, because Gravitino v1.3.0 offers no request to modify an
// existing partition.
func (r *PartitionResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Debug(context.Background(), "Updating partition")
	resp.Diagnostics.AddError(
		"Partition update not supported",
		"Gravitino v1.3.0 only exposes add partitions and drop partition; a partition cannot be modified. "+
			"Every attribute of gravitino_partition forces a replacement, so reaching this code path is a provider bug.",
	)
}

// Delete drops the partition. A partition that is already gone is treated as
// deleted.
func (r *PartitionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PartitionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Dropping partition", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"table":    state.Table.ValueString(),
		"name":     state.Name.ValueString(),
	})

	_, err := r.client.DropPartition(
		ctx,
		state.Metalake.ValueString(),
		state.Catalog.ValueString(),
		state.Schema.ValueString(),
		state.Table.ValueString(),
		state.Name.ValueString(),
	)
	if err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.Append(client.NewResourceError("dropping partition", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Dropped partition", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"table":    state.Table.ValueString(),
		"name":     state.Name.ValueString(),
	})
}

// ImportState imports a partition from a metalake.catalog.schema.table.partition id.
func (r *PartitionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 5)
	if len(parts) != 5 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format 'metalake.catalog.schema.table.partition', got %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("table"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[4])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// validatePartitionSpec reports the fields required by the configured partition
// type, and rejects fields that belong to another partition type.
func validatePartitionSpec(m *PartitionResourceModel, diags *diag.Diagnostics) {
	switch m.Type.ValueString() {
	case models.PartitionTypeIdentity:
		if m.FieldNames.IsNull() {
			diags.AddAttributeError(path.Root("field_names"), "Missing field_names",
				"An identity partition requires field_names.")
		}
		if m.Values.IsNull() {
			diags.AddAttributeError(path.Root("values"), "Missing values",
				"An identity partition requires values.")
		}
		if !m.Upper.IsNull() || !m.Lower.IsNull() {
			diags.AddAttributeError(path.Root("type"), "Invalid partition attributes",
				"upper and lower belong to a range partition.")
		}
		if !m.Lists.IsNull() {
			diags.AddAttributeError(path.Root("type"), "Invalid partition attributes",
				"lists belongs to a list partition.")
		}
	case models.PartitionTypeRange:
		if m.Name.IsNull() {
			diags.AddAttributeError(path.Root("name"), "Missing name",
				"A range partition requires a name.")
		}
		if m.Upper.IsNull() || m.Lower.IsNull() {
			diags.AddAttributeError(path.Root("type"), "Missing range bounds",
				"A range partition requires upper and lower.")
		}
		if !m.FieldNames.IsNull() || !m.Values.IsNull() {
			diags.AddAttributeError(path.Root("type"), "Invalid partition attributes",
				"field_names and values belong to an identity partition.")
		}
		if !m.Lists.IsNull() {
			diags.AddAttributeError(path.Root("type"), "Invalid partition attributes",
				"lists belongs to a list partition.")
		}
	case models.PartitionTypeList:
		if m.Name.IsNull() {
			diags.AddAttributeError(path.Root("name"), "Missing name",
				"A list partition requires a name.")
		}
		if m.Lists.IsNull() {
			diags.AddAttributeError(path.Root("lists"), "Missing lists",
				"A list partition requires lists.")
		}
		if !m.FieldNames.IsNull() || !m.Values.IsNull() {
			diags.AddAttributeError(path.Root("type"), "Invalid partition attributes",
				"field_names and values belong to an identity partition.")
		}
		if !m.Upper.IsNull() || !m.Lower.IsNull() {
			diags.AddAttributeError(path.Root("type"), "Invalid partition attributes",
				"upper and lower belong to a range partition.")
		}
	}
}

// partitionFromModel converts the planned partition into the PartitionSpec of
// partitions.yaml.
func partitionFromModel(ctx context.Context, m *PartitionResourceModel, diags *diag.Diagnostics) models.Partition {
	partition := models.Partition{
		Type:       m.Type.ValueString(),
		Name:       m.Name.ValueString(),
		Properties: mapFromModel(ctx, m.Properties, diags),
	}

	// The catalog derives the name of an identity partition, so it must not be
	// part of the request.
	if partition.Type == models.PartitionTypeIdentity {
		partition.Name = ""
	}

	fieldNames, fieldNameDiags := models.FieldNamesFromList(ctx, m.FieldNames)
	diags.Append(fieldNameDiags...)

	values, valueDiags := models.LiteralsFromList(ctx, m.Values)
	diags.Append(valueDiags...)

	lists, listDiags := models.LiteralListsFromList(ctx, m.Lists)
	diags.Append(listDiags...)
	if diags.HasError() {
		return partition
	}

	partition.FieldNames = fieldNames
	partition.Values = values
	partition.Lists = lists
	partition.Upper = literalFromObject(ctx, m.Upper, diags)
	partition.Lower = literalFromObject(ctx, m.Lower, diags)

	return partition
}

func literalFromObject(ctx context.Context, value types.Object, diags *diag.Diagnostics) *models.Literal {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	literal, literalDiags := models.LiteralFromObjectValue(ctx, value)
	diags.Append(literalDiags...)
	if diags.HasError() {
		return nil
	}
	return &literal
}

func mapFromModel(ctx context.Context, value types.Map, diags *diag.Diagnostics) map[string]string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}

	properties := make(map[string]string)
	diags.Append(value.ElementsAs(ctx, &properties, false)...)
	if diags.HasError() {
		return nil
	}
	if len(properties) == 0 {
		return nil
	}
	return properties
}

// setPartitionState writes the partition reported by Gravitino into the model.
//
// desiredProperties holds the properties managed by the configuration (or by
// the state during a refresh). Keys the catalog adds on its own are only stored
// when no properties are managed at all, so that a catalog that annotates
// partitions with statistics does not produce permanent drift.
func setPartitionState(ctx context.Context, partition *models.Partition, m *PartitionResourceModel, desiredProperties types.Map, diags *diag.Diagnostics) {
	m.Type = types.StringValue(partition.Type)
	m.Name = types.StringValue(partition.Name)

	fieldNames, fieldNameDiags := models.FieldNamesToList(ctx, partition.FieldNames)
	diags.Append(fieldNameDiags...)

	values, valueDiags := models.LiteralsToList(ctx, partition.Values)
	diags.Append(valueDiags...)

	lists, listDiags := models.LiteralListsToList(ctx, partition.Lists)
	diags.Append(listDiags...)

	if diags.HasError() {
		return
	}

	m.FieldNames = fieldNames
	m.Values = values
	m.Lists = lists
	m.Upper = objectFromLiteral(ctx, partition.Upper, diags)
	m.Lower = objectFromLiteral(ctx, partition.Lower, diags)
	m.Properties = mergePartitionProperties(ctx, partition.Properties, desiredProperties, diags)

	m.ID = types.StringValue(partitionID(
		m.Metalake.ValueString(),
		m.Catalog.ValueString(),
		m.Schema.ValueString(),
		m.Table.ValueString(),
		partition.Name,
	))
}

func objectFromLiteral(ctx context.Context, literal *models.Literal, diags *diag.Diagnostics) types.Object {
	if literal == nil {
		return types.ObjectNull(models.PartitionLiteralAttrTypes)
	}

	object, objectDiags := models.LiteralToObjectValue(ctx, *literal)
	diags.Append(objectDiags...)
	if diags.HasError() {
		return types.ObjectNull(models.PartitionLiteralAttrTypes)
	}
	return object
}

func mergePartitionProperties(ctx context.Context, serverProperties map[string]string, desired types.Map, diags *diag.Diagnostics) types.Map {
	if desired.IsNull() || desired.IsUnknown() {
		if len(serverProperties) == 0 {
			return types.MapNull(types.StringType)
		}
		properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, serverProperties)
		diags.Append(propertyDiags...)
		return properties
	}

	managed := make(map[string]string)
	diags.Append(desired.ElementsAs(ctx, &managed, false)...)
	if diags.HasError() {
		return desired
	}
	if len(managed) == 0 {
		return desired
	}

	merged := make(map[string]string, len(managed))
	for key, value := range managed {
		if serverValue, ok := serverProperties[key]; ok {
			merged[key] = serverValue
			continue
		}
		merged[key] = value
	}

	properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, merged)
	diags.Append(propertyDiags...)
	return properties
}

func partitionID(metalake, catalog, schema, table, name string) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s", metalake, catalog, schema, table, name)
}
