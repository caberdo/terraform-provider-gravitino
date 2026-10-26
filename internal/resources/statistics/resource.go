// Package statistics implements the gravitino_statistics and
// gravitino_partition_statistics resources.
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

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

var _ resource.Resource = (*StatisticsResource)(nil)
var _ resource.ResourceWithImportState = (*StatisticsResource)(nil)
var _ resource.ResourceWithConfigure = (*StatisticsResource)(nil)

// StatisticsResource manages the modifiable statistics of one metadata object.
//
// Gravitino exposes statistics through updateStatistics (PUT) and
// dropStatistics (POST with the names to drop); there is no DELETE endpoint and
// no request that replaces the whole collection. The resource therefore reads
// the current statistics on every create/update, sends the entries that differ
// as one update request and the entries that disappeared as one drop request.
type StatisticsResource struct {
	providerbase.Resource
}

// StatisticsResourceModel is the Terraform model of gravitino_statistics.
type StatisticsResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Metalake       types.String `tfsdk:"metalake"`
	ObjectType     types.String `tfsdk:"object_type"`
	ObjectFullName types.String `tfsdk:"object_full_name"`
	Statistics     types.Map    `tfsdk:"statistics"`
}

// New returns the gravitino_statistics resource.
func New() resource.Resource {
	return &StatisticsResource{}
}

func (r *StatisticsResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_statistics"
}

func (r *StatisticsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages the modifiable (custom) statistics of one Gravitino metadata object. " +
			"The resource owns the whole modifiable set: a statistic that exists on the server but not in this " +
			"map is dropped on the next apply, and a statistic that disappeared on the server is written again. " +
			"Reserved, catalog-supplied statistics (for example Iceberg's row count) are ignored by Read and never dropped. " +
			"Gravitino 1.3.x implements statistics for TABLE objects only.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					models.CompoundID("metalake", "object_type", "object_full_name"),
				},
				Description: "Composite identifier in the format 'metalake.object_type.object_full_name'.",
			},
			"metalake": schema.StringAttribute{
				Required:    true,
				Description: "The metalake the object belongs to. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_type": schema.StringAttribute{
				Required: true,
				Description: fmt.Sprintf("The metadata object type, one of: %s. Gravitino 1.3.x only implements statistics for TABLE and rejects every other type with 400. "+
					"VIEW and FUNCTION require Gravitino >= 1.3.1 and are rejected on an older server with an explicit diagnostic. Changing this forces replacement.",
					strings.Join(models.StatisticsObjectTypes, ", ")),
				Validators: []validator.String{
					stringvalidator.OneOf(models.StatisticsObjectTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"object_full_name": schema.StringAttribute{
				Required: true,
				Description: "The full name of the metadata object relative to the metalake, for example 'my_catalog' for a CATALOG, " +
					"'my_catalog.my_schema' for a SCHEMA and 'my_catalog.my_schema.my_table' for a TABLE. " +
					"Do not prefix it with the metalake name. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"statistics": schema.MapAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Custom statistics as a map of statistic name to value. Names must start with 'custom-' and be at most 128 characters, " +
					"which is the only shape Gravitino 1.3.x accepts on write. Gravitino's update request is untyped JSON " +
					"(`additionalProperties: {}`), but the provider writes every value as a JSON string and renders the value read back as a string: " +
					"a numeric, boolean or structured value written by another client appears as its JSON encoding and is rewritten as a string on the next apply. " +
					"Set it to {} to drop every statistic this resource manages while keeping the resource.",
				Validators: []validator.Map{
					statisticNameValidator{},
				},
			},
		},
	}
}

func (r *StatisticsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan StatisticsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	objectType := plan.ObjectType.ValueString()
	objectFullName := plan.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Creating statistics", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectType(ctx, objectType, &resp.Diagnostics) {
		return
	}

	desired, diags := statisticsFromModel(ctx, plan.Statistics)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, metalake, objectType, objectFullName, desired); err != nil {
		resp.Diagnostics.Append(client.NewStatisticsError("creating statistics of", objectFullName, err)...)
		return
	}

	resp.Diagnostics.Append(setStatisticsState(ctx, &plan, desired)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Created statistics", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *StatisticsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state StatisticsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	objectType := state.ObjectType.ValueString()
	objectFullName := state.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Reading statistics", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectType(ctx, objectType, &resp.Diagnostics) {
		return
	}

	current, err := r.currentStatistics(ctx, metalake, objectType, objectFullName)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewStatisticsError("reading statistics of", objectFullName, err)...)
		return
	}

	resp.Diagnostics.Append(setStatisticsState(ctx, &state, current)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read statistics", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *StatisticsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan StatisticsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	objectType := plan.ObjectType.ValueString()
	objectFullName := plan.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Updating statistics", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	if !r.checkObjectType(ctx, objectType, &resp.Diagnostics) {
		return
	}

	desired, diags := statisticsFromModel(ctx, plan.Statistics)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.apply(ctx, metalake, objectType, objectFullName, desired); err != nil {
		resp.Diagnostics.Append(client.NewStatisticsError("updating statistics of", objectFullName, err)...)
		return
	}

	resp.Diagnostics.Append(setStatisticsState(ctx, &plan, desired)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Updated statistics", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *StatisticsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state StatisticsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	objectType := state.ObjectType.ValueString()
	objectFullName := state.ObjectFullName.ValueString()

	tflog.Debug(ctx, "Deleting statistics", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})

	managed, diags := statisticsFromModel(ctx, state.Statistics)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	names := make([]string, 0, len(managed))
	for name := range managed {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) > 0 {
		if _, err := r.Client.DeleteStatistics(ctx, metalake, objectType, objectFullName, &models.StatisticsDropRequest{Names: names}); err != nil {
			if client.IsNotFoundError(err) {
				// The object is gone, so its statistics are gone with it.
				tflog.Debug(ctx, "Statistics already deleted", map[string]interface{}{
					"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
				})
				return
			}
			resp.Diagnostics.Append(client.NewStatisticsError("deleting statistics of", objectFullName, err)...)
			return
		}
	}

	tflog.Debug(ctx, "Deleted statistics", map[string]interface{}{
		"metalake": metalake, "object_type": objectType, "object_full_name": objectFullName,
	})
}

func (r *StatisticsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts, ok := resourceutil.SplitImportID(req, resp, 3, "metalake.object_type.object_full_name")
	if !ok {
		return
	}

	metalake, objectType, objectFullName := parts[0], parts[1], parts[2]
	if !containsString(models.StatisticsObjectTypes, objectType) {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Unknown object type %q in import ID %q; expected one of: %s.",
				objectType, req.ID, strings.Join(models.StatisticsObjectTypes, ", ")),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), metalake)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_type"), objectType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_full_name"), objectFullName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), statisticsID(metalake, objectType, objectFullName))...)
}

// apply makes the server's modifiable statistics match desired: differing and
// missing entries go out as one updateStatistics request, entries that are no
// longer configured as one dropStatistics request.
func (r *StatisticsResource) apply(ctx context.Context, metalake, objectType, objectFullName string, desired map[string]string) error {
	current, err := r.currentStatistics(ctx, metalake, objectType, objectFullName)
	if err != nil {
		return err
	}

	updates := make(map[string]string)
	for name, value := range desired {
		if existing, ok := current[name]; !ok || existing != value {
			updates[name] = value
		}
	}

	drops := make([]string, 0)
	for name := range current {
		if _, ok := desired[name]; !ok {
			drops = append(drops, name)
		}
	}
	sort.Strings(drops)

	if len(updates) > 0 {
		tflog.Debug(ctx, "Updating statistic values", map[string]interface{}{"object_full_name": objectFullName, "statistics": sortedKeys(updates)})
		if _, err := r.Client.UpdateStatistics(ctx, metalake, objectType, objectFullName, &models.StatisticsUpdateRequest{Updates: updates}); err != nil {
			return err
		}
	}

	if len(drops) > 0 {
		tflog.Debug(ctx, "Dropping statistics", map[string]interface{}{"object_full_name": objectFullName, "statistics": drops})
		if _, err := r.Client.DeleteStatistics(ctx, metalake, objectType, objectFullName, &models.StatisticsDropRequest{Names: drops}); err != nil {
			return err
		}
	}

	return nil
}

// currentStatistics returns the modifiable statistics the server reports, keyed
// by name and rendered as strings. Reserved statistics belong to the underlying
// catalog and cannot be written or dropped through Gravitino, so they are
// ignored.
func (r *StatisticsResource) currentStatistics(ctx context.Context, metalake, objectType, objectFullName string) (map[string]string, error) {
	result, err := r.Client.ListStatistics(ctx, metalake, objectType, objectFullName)
	if err != nil {
		return nil, err
	}

	current := make(map[string]string, len(result.Statistics))
	for _, stat := range result.Statistics {
		s := stat
		if s.Reserved || !s.Modifiable {
			continue
		}
		value, ok := s.ValueString()
		if !ok {
			tflog.Warn(ctx, "Skipping a modifiable statistic without a value", map[string]interface{}{
				"object_full_name": objectFullName, "statistic": s.Name,
			})
			continue
		}
		current[s.Name] = value
	}
	return current, nil
}

// checkObjectType rejects the 1.3.1-only VIEW/FUNCTION object types on an older
// server before any statistics request is sent.
func (r *StatisticsResource) checkObjectType(ctx context.Context, objectType string, diags *diag.Diagnostics) bool {
	if err := r.Client.CheckMetadataObjectTypeSupported(ctx, objectType); err != nil {
		diags.AddAttributeError(path.Root("object_type"), "Unsupported object_type value", err.Error())
		return false
	}
	return true
}

func setStatisticsState(ctx context.Context, model *StatisticsResourceModel, statistics map[string]string) diag.Diagnostics {
	model.ID = types.StringValue(statisticsID(
		model.Metalake.ValueString(),
		model.ObjectType.ValueString(),
		model.ObjectFullName.ValueString(),
	))

	if statistics == nil {
		statistics = map[string]string{}
	}

	value, diags := types.MapValueFrom(ctx, types.StringType, statistics)
	if !diags.HasError() {
		model.Statistics = value
	}
	return diags
}

func statisticsFromModel(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return map[string]string{}, nil
	}

	statistics := make(map[string]string)
	diags := value.ElementsAs(ctx, &statistics, false)
	return statistics, diags
}

func statisticsID(metalake, objectType, objectFullName string) string {
	return fmt.Sprintf("%s.%s.%s", metalake, objectType, objectFullName)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// statisticNameValidator enforces the only names Gravitino accepts on write
// (StatisticOperations.validateStatisticName): the "custom-" prefix and at most
// 128 characters.
type statisticNameValidator struct{}

func (v statisticNameValidator) Description(context.Context) string {
	return fmt.Sprintf("statistic names must start with %q and be at most %d characters", models.StatisticCustomPrefix, models.StatisticMaxNameLength)
}

func (v statisticNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v statisticNameValidator) ValidateMap(_ context.Context, req validator.MapRequest, resp *validator.MapResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	for name := range req.ConfigValue.Elements() {
		validateStatisticName(name, req.Path.AtMapKey(name), &resp.Diagnostics)
	}
}

// validateStatisticName reports a name that Gravitino would reject with an
// opaque 400 IllegalStatisticNameException.
func validateStatisticName(name string, attrPath path.Path, diags *diag.Diagnostics) {
	if !strings.HasPrefix(name, models.StatisticCustomPrefix) {
		diags.AddAttributeError(attrPath, "Invalid statistic name",
			fmt.Sprintf("Gravitino only accepts statistic names that start with %q, got %q.", models.StatisticCustomPrefix, name))
		return
	}
	if len(name) > models.StatisticMaxNameLength {
		diags.AddAttributeError(attrPath, "Invalid statistic name",
			fmt.Sprintf("Statistic names must be at most %d characters, %q has %d.", models.StatisticMaxNameLength, name, len(name)))
	}
}
