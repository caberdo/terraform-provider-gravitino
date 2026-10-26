package statistics

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/providerbase"
	"github.com/gravitino/terraform-provider-gravitino/internal/resources/resourceutil"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*PartitionStatisticsResource)(nil)
var _ resource.ResourceWithImportState = (*PartitionStatisticsResource)(nil)
var _ resource.ResourceWithConfigure = (*PartitionStatisticsResource)(nil)

// PartitionStatisticsResource manages the modifiable statistics of every
// partition of one table.
//
// The API has no request that replaces the collection: updates go through
// updatePartitionStatistics (PUT) and removals through dropPartitionStatistics
// (POST with the partition names and statistic names). The resource reads the
// current partition statistics on every create/update, sends the entries that
// differ as one update request and the entries that disappeared as one drop
// request.
type PartitionStatisticsResource struct {
	providerbase.Resource
}

// PartitionStatisticsResourceModel is the Terraform model of
// gravitino_partition_statistics.
type PartitionStatisticsResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Metalake   types.String `tfsdk:"metalake"`
	Catalog    types.String `tfsdk:"catalog"`
	Schema     types.String `tfsdk:"schema"`
	Table      types.String `tfsdk:"table"`
	Partitions types.Map    `tfsdk:"partitions"`
}

// NewPartition returns the gravitino_partition_statistics resource.
func NewPartition() resource.Resource {
	return &PartitionStatisticsResource{}
}

func (r *PartitionStatisticsResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_partition_statistics"
}

func (r *PartitionStatisticsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the modifiable (custom) statistics of the partitions of one Gravitino table. " +
			"The resource owns the whole modifiable set: a partition statistic that exists on the server but not in " +
			"this map is dropped on the next apply, and a partition that is removed from the map loses every " +
			"statistic this resource manages. Gravitino 1.3.x implements partition statistics for TABLE objects only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					models.CompoundID("metalake", "catalog", "schema", "table"),
				},
				Description: "Composite identifier in the format 'metalake.catalog.schema.table'.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake the table belongs to. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Required:    true,
				Description: "The catalog the table belongs to. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Required:    true,
				Description: "The schema the table belongs to. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"table": schema.StringAttribute{
				Required:    true,
				Description: "The table whose partition statistics are managed. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"partitions": schema.MapAttribute{
				Required:    true,
				ElementType: types.MapType{ElemType: types.StringType},
				Description: "Partition statistics as a map of partition name to a map of statistic name and value. " +
					"Statistic names must start with 'custom-' and be at most 128 characters, which is the only shape Gravitino 1.3.x accepts on write. " +
					"Values cross the provider boundary as strings, like gravitino_statistics. " +
					"Set it to {} to drop every partition statistic this resource manages while keeping the resource.",
				Validators: []validator.Map{
					partitionStatisticsValidator{},
				},
			},
		},
	}
}

func (r *PartitionStatisticsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PartitionStatisticsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating partition statistics", tableLogFields(&plan))

	desired, diags := partitionsFromModel(ctx, plan.Partitions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, &plan, desired); err != nil {
		resp.Diagnostics.Append(client.NewStatisticsError("creating partition statistics of", tableFullName(&plan), err)...)
		return
	}

	resp.Diagnostics.Append(setPartitionStatisticsState(ctx, &plan, desired)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Created partition statistics", tableLogFields(&plan))
}

func (r *PartitionStatisticsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PartitionStatisticsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading partition statistics", tableLogFields(&state))

	current, err := r.currentPartitions(ctx, &state)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewStatisticsError("reading partition statistics of", tableFullName(&state), err)...)
		return
	}

	resp.Diagnostics.Append(setPartitionStatisticsState(ctx, &state, current)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read partition statistics", tableLogFields(&state))
}

func (r *PartitionStatisticsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PartitionStatisticsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating partition statistics", tableLogFields(&plan))

	desired, diags := partitionsFromModel(ctx, plan.Partitions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, &plan, desired); err != nil {
		resp.Diagnostics.Append(client.NewStatisticsError("updating partition statistics of", tableFullName(&plan), err)...)
		return
	}

	resp.Diagnostics.Append(setPartitionStatisticsState(ctx, &plan, desired)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Updated partition statistics", tableLogFields(&plan))
}

func (r *PartitionStatisticsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PartitionStatisticsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting partition statistics", tableLogFields(&state))

	managed, diags := partitionsFromModel(ctx, state.Partitions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	drops := partitionDrops(managed, nil)
	if len(drops) > 0 {
		if _, err := r.Client.DeletePartitionStatistics(ctx, state.Metalake.ValueString(), "TABLE", tableFullName(&state),
			&models.PartitionStatisticsDropRequest{Drops: drops}); err != nil {
			if client.IsNotFoundError(err) {
				// The table is gone, so its partition statistics are gone with it.
				tflog.Debug(ctx, "Partition statistics already deleted", tableLogFields(&state))
				return
			}
			resp.Diagnostics.Append(client.NewStatisticsError("deleting partition statistics of", tableFullName(&state), err)...)
			return
		}
	}

	tflog.Debug(ctx, "Deleted partition statistics", tableLogFields(&state))
}

func (r *PartitionStatisticsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := resourceutil.SplitImportID(req, resp, 4, "metalake.catalog.schema.table")
	if !ok {
		return
	}

	metalake, catalog, schemaName, table := parts[0], parts[1], parts[2], parts[3]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), catalog)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), schemaName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("table"), table)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), partitionStatisticsID(metalake, catalog, schemaName, table))...)
}

// apply makes the server's modifiable partition statistics match desired:
// differing and missing entries go out as one updatePartitionStatistics
// request, entries that are no longer configured as one dropPartitionStatistics
// request.
func (r *PartitionStatisticsResource) apply(ctx context.Context, model *PartitionStatisticsResourceModel, desired map[string]map[string]string) error {
	current, err := r.currentPartitions(ctx, model)
	if err != nil {
		return err
	}

	updates := make([]models.PartitionStatisticsUpdate, 0)
	for partition, statistics := range desired {
		changed := make(map[string]string)
		for name, value := range statistics {
			if existing, ok := current[partition][name]; !ok || existing != value {
				changed[name] = value
			}
		}
		if len(changed) > 0 {
			updates = append(updates, models.PartitionStatisticsUpdate{
				PartitionName: partition,
				Statistics:    changed,
			})
		}
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].PartitionName < updates[j].PartitionName })

	drops := partitionDrops(current, desired)

	metalake := model.Metalake.ValueString()
	fullName := tableFullName(model)

	if len(updates) > 0 {
		tflog.Debug(ctx, "Updating partition statistic values", map[string]interface{}{
			"object_full_name": fullName, "partitions": len(updates),
		})
		if _, err := r.Client.UpdatePartitionStatistics(ctx, metalake, "TABLE", fullName,
			&models.PartitionStatisticsUpdateRequest{Updates: updates}); err != nil {
			return err
		}
	}

	if len(drops) > 0 {
		tflog.Debug(ctx, "Dropping partition statistics", map[string]interface{}{
			"object_full_name": fullName, "partitions": len(drops),
		})
		if _, err := r.Client.DeletePartitionStatistics(ctx, metalake, "TABLE", fullName,
			&models.PartitionStatisticsDropRequest{Drops: drops}); err != nil {
			return err
		}
	}

	return nil
}

// partitionDrops lists, per partition, the statistic names the resource must
// drop: names the server holds that desired does not. A nil desired map drops
// every name, which is what Delete needs.
func partitionDrops(current, desired map[string]map[string]string) []models.PartitionStatisticsDrop {
	drops := make([]models.PartitionStatisticsDrop, 0)
	for partition, statistics := range current {
		desiredStatistics := desired[partition]

		names := make([]string, 0, len(statistics))
		for name := range statistics {
			if _, ok := desiredStatistics[name]; !ok {
				names = append(names, name)
			}
		}
		sort.Strings(names)

		if len(names) == 0 {
			continue
		}
		drops = append(drops, models.PartitionStatisticsDrop{
			PartitionName:  partition,
			StatisticNames: names,
		})
	}
	sort.Slice(drops, func(i, j int) bool { return drops[i].PartitionName < drops[j].PartitionName })
	return drops
}

// currentPartitions returns the modifiable statistics the server reports per
// partition, rendered as strings.
func (r *PartitionStatisticsResource) currentPartitions(ctx context.Context, model *PartitionStatisticsResourceModel) (map[string]map[string]string, error) {
	result, err := r.Client.ListPartitionStatistics(ctx, model.Metalake.ValueString(), "TABLE", tableFullName(model))
	if err != nil {
		return nil, err
	}

	current := make(map[string]map[string]string, len(result.Statistics))
	for _, partition := range result.Statistics {
		statistics := make(map[string]string, len(partition.Statistics))
		for _, stat := range partition.Statistics {
			s := stat
			if s.Reserved || !s.Modifiable {
				continue
			}
			value, ok := s.ValueString()
			if !ok {
				tflog.Warn(ctx, "Skipping a modifiable statistic without a value", map[string]interface{}{
					"object_full_name": tableFullName(model), "partition": partition.PartitionName, "statistic": s.Name,
				})
				continue
			}
			statistics[s.Name] = value
		}
		if len(statistics) > 0 {
			current[partition.PartitionName] = statistics
		}
	}
	return current, nil
}

func setPartitionStatisticsState(ctx context.Context, model *PartitionStatisticsResourceModel, partitions map[string]map[string]string) diag.Diagnostics {
	model.ID = types.StringValue(partitionStatisticsID(
		model.Metalake.ValueString(),
		model.Catalog.ValueString(),
		model.Schema.ValueString(),
		model.Table.ValueString(),
	))

	if partitions == nil {
		partitions = map[string]map[string]string{}
	}

	value, diags := partitionsToMap(ctx, partitions)
	if !diags.HasError() {
		model.Partitions = value
	}
	return diags
}

func partitionsFromModel(ctx context.Context, value types.Map) (map[string]map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsNull() || value.IsUnknown() {
		return map[string]map[string]string{}, diags
	}

	partitions := make(map[string]map[string]string, len(value.Elements()))
	for partition, element := range value.Elements() {
		statisticsValue, ok := element.(types.Map)
		if !ok {
			diags.AddError("Invalid partitions value",
				fmt.Sprintf("Expected partition %q to hold a map of statistics, got %T. Please report this issue to the provider developers.", partition, element))
			continue
		}

		statistics := make(map[string]string)
		diags.Append(statisticsValue.ElementsAs(ctx, &statistics, false)...)
		if diags.HasError() {
			return nil, diags
		}
		partitions[partition] = statistics
	}

	return partitions, diags
}

func partitionsToMap(ctx context.Context, partitions map[string]map[string]string) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics

	elementType := types.MapType{ElemType: types.StringType}
	elements := make(map[string]attr.Value, len(partitions))
	for partition, statistics := range partitions {
		value, valueDiags := types.MapValueFrom(ctx, types.StringType, statistics)
		diags.Append(valueDiags...)
		if diags.HasError() {
			return types.MapNull(elementType), diags
		}
		elements[partition] = value
	}

	return types.MapValue(elementType, elements)
}

func tableFullName(model *PartitionStatisticsResourceModel) string {
	return fmt.Sprintf("%s.%s.%s",
		model.Catalog.ValueString(),
		model.Schema.ValueString(),
		model.Table.ValueString(),
	)
}

func tableLogFields(model *PartitionStatisticsResourceModel) map[string]interface{} {
	return map[string]interface{}{
		"metalake": model.Metalake.ValueString(),
		"catalog":  model.Catalog.ValueString(),
		"schema":   model.Schema.ValueString(),
		"table":    model.Table.ValueString(),
	}
}

func partitionStatisticsID(metalake, catalog, schemaName, table string) string {
	return fmt.Sprintf("%s.%s.%s.%s", metalake, catalog, schemaName, table)
}

// partitionStatisticsValidator enforces the name rules on the nested statistic
// maps (the outer map's keys are partition names, which the server accepts as
// given).
type partitionStatisticsValidator struct{}

func (v partitionStatisticsValidator) Description(context.Context) string {
	return fmt.Sprintf("partition statistic names must start with %q and be at most %d characters", models.StatisticCustomPrefix, models.StatisticMaxNameLength)
}

func (v partitionStatisticsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v partitionStatisticsValidator) ValidateMap(_ context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	for partition, element := range req.ConfigValue.Elements() {
		if strings.TrimSpace(partition) == "" {
			resp.Diagnostics.AddAttributeError(req.Path.AtMapKey(partition), "Invalid partition name",
				"A partition statistics entry must name the partition it belongs to.")
			continue
		}

		statistics, ok := element.(types.Map)
		if !ok || statistics.IsNull() || statistics.IsUnknown() {
			continue
		}
		if len(statistics.Elements()) == 0 {
			resp.Diagnostics.AddAttributeError(req.Path.AtMapKey(partition), "Empty partition statistics",
				"A partition entry must list at least one statistic. Remove the partition key to drop every statistic this resource manages for that partition.")
			continue
		}
		for name := range statistics.Elements() {
			validateStatisticName(name, req.Path.AtMapKey(partition).AtMapKey(name), &resp.Diagnostics)
		}
	}
}
