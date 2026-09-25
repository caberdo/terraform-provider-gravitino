package table

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*tableResource)(nil)
var _ resource.ResourceWithImportState = (*tableResource)(nil)
var _ resource.ResourceWithConfigure = (*tableResource)(nil)
var _ resource.ResourceWithModifyPlan = (*tableResource)(nil)
var _ resource.ResourceWithConfigValidators = (*tableResource)(nil)

type tableResource struct {
	client *client.Client
}

// NewTableResource returns the gravitino_table resource.
func NewTableResource() resource.Resource {
	return &tableResource{}
}

func (r *tableResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *tableResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_table"
}

func (r *tableResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a table in a Gravitino schema. Tables are updated in place with the update requests of " +
			"tables.yaml; attributes Gravitino cannot update in place force a replacement, which is documented on each " +
			"of those attributes.",
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Description: "The metalake the table belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Description: "The catalog the table belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Description: "The schema the table belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the table. Renaming a table in place is supported through the rename update " +
					"request of tables.yaml; the compound id of the resource follows the new name.",
				Required: true,
			},
			"comment": schema.StringAttribute{
				Description: "The comment of the table.",
				Optional:    true,
			},
			"properties": schema.MapAttribute{
				Description: "A map of key-value properties. The reserved 'in-use' property is managed by Gravitino and " +
					"is filtered out. Gravitino creates a table with at least 1000 rows on the first write, which " +
					"resets the properties of the table; the provider reapplies the configured properties on every " +
					"update, and properties the catalog adds on its own are only reported when no properties are " +
					"managed by this resource.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"id": schema.StringAttribute{
				Description: "The compound identifier in the format metalake.catalog.schema.table.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					models.CompoundID("metalake", "catalog", "schema", "name"),
				},
			},
			"audit": schema.ObjectAttribute{
				Description:    "Audit information of the table, reported by Gravitino.",
				Computed:       true,
				AttributeTypes: models.AuditAttrTypes,
			},
		},
		Blocks: map[string]schema.Block{
			"column": schema.ListNestedBlock{
				Description: "A column of the table. The type of a primitive column is a Gravitino primitive type name " +
					"such as \"integer\" or \"varchar(255)\"; struct, list, map, union and unparsed columns are written " +
					"as a JSON object, for example jsonencode({type = \"struct\", fields = [...]}). Type, comment, " +
					"nullable, auto_increment, default_value and position changes of an existing column are applied in " +
					"place with the updateColumnType, updateColumnComment, updateColumnNullability, " +
					"updateColumnDefaultValue and updateColumnPosition requests of tables.yaml. Any change to the set of " +
					"column names replaces the table: Gravitino cannot tell a renamed column from a deleted and added " +
					"one, and an in-place implementation would silently drop the data of the renamed column. Removing " +
					"the default_value of a column that has one also replaces the table, because Gravitino v1.3.0 " +
					"rejects a null newDefaultValue and has no other way to clear a column default.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the column.",
							Required:    true,
						},
						"type": schema.StringAttribute{
							Description: "The Gravitino data type of the column: a primitive type name such as " +
								"\"integer\", \"varchar(255)\", \"decimal(10,2)\", \"timestamp(3)\", \"byte unsigned\" " +
								"or \"binary\", or a JSON object for the structured types struct, list, map, union and " +
								"unparsed.",
							Required: true,
							Validators: []validator.String{
								dataTypeStringValidator{},
							},
						},
						"comment": schema.StringAttribute{
							Description: "The comment of the column.",
							Optional:    true,
						},
						"nullable": schema.BoolAttribute{
							Description: "Whether the column is nullable. Gravitino defaults this to true, so the " +
								"provider always sends it: omitting it would turn a NOT NULL column nullable again.",
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(true),
						},
						"auto_increment": schema.BoolAttribute{
							Description: "Whether the column auto increments.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"default_value": schema.StringAttribute{
							Description: "The default value of the column as the string form of a Gravitino literal. " +
								"The data type of the literal is taken from the column type. Omit the attribute for a " +
								"column without a default value.",
							Optional: true,
						},
					},
				},
			},
			"sort_order": schema.ListNestedBlock{
				Description: "A sort order of the table. Gravitino v1.3.0 defines no update request for the sort orders " +
					"of an existing table, so changing this block replaces the table.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"field_name": schema.ListAttribute{
							Description: "The name of the field to sort by.",
							Required:    true,
							ElementType: types.StringType,
						},
						"direction": schema.StringAttribute{
							Description: "The sort direction.",
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString("asc"),
							Validators: []validator.String{
								stringvalidator.OneOf("asc", "desc"),
							},
						},
						"null_ordering": schema.StringAttribute{
							Description: "Where null values are sorted. Gravitino defaults this to nulls_first for an " +
								"ascending and to nulls_last for a descending sort order; when the value is omitted the " +
								"provider resolves it from the direction, so the state matches what Gravitino reports.",
							Optional: true,
							// Computed: the resolved value (explicit, or derived from direction) is read
							// back from the server, so the attribute must be allowed to become known
							// during apply instead of being pinned to a null plan value.
							Computed: true,
							Validators: []validator.String{
								stringvalidator.OneOf("nulls_first", "nulls_last"),
							},
						},
					},
				},
			},
			"distribution": schema.SingleNestedBlock{
				Description: "How the data of the table is distributed. Gravitino v1.3.0 defines no update request for " +
					"the distribution of an existing table, so changing this block replaces the table.",
				Attributes: map[string]schema.Attribute{
					"strategy": schema.StringAttribute{
						Description: "The distribution strategy.",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("hash"),
						Validators: []validator.String{
							stringvalidator.OneOf("hash", "range", "even"),
						},
					},
					"number": schema.Int64Attribute{
						Description: "The number of buckets to distribute the data over. Required by tables.yaml " +
							"whenever a distribution is configured; the provider validates that.",
						Optional: true,
					},
					"func_args": schema.ListAttribute{
						Description: "The distribution arguments as dotted field paths, for example [\"id\"] or " +
							"[\"info\", \"contact\"]. Gravitino also accepts literal and function arguments, which this " +
							"provider cannot express; only field arguments are supported.",
						Optional:    true,
						ElementType: types.StringType,
					},
				},
			},
			"partitioning": schema.ListNestedBlock{
				Description: "A partitioning strategy of the table. Gravitino v1.3.0 defines no update request for the " +
					"partitioning of an existing table, so changing this block replaces the table. The pre-assigned " +
					"list and range partitions of a partitioning strategy are not supported.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"strategy": schema.StringAttribute{
							Description: "The partitioning strategy.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("identity", "year", "month", "day", "hour", "bucket", "truncate", "list", "range", "function"),
							},
						},
						"field_name": schema.ListAttribute{
							Description: "The field to partition by, as a path of segments.",
							Optional:    true,
							ElementType: types.StringType,
						},
						"field_names": schema.ListAttribute{
							Description: "The fields to partition by, each entry holding the path segments of a field.",
							Optional:    true,
							ElementType: types.ListType{ElemType: types.StringType},
						},
						"num_buckets": schema.Int64Attribute{
							Description: "The number of buckets of a bucket partitioning.",
							Optional:    true,
						},
						"width": schema.Int64Attribute{
							Description: "The width of a truncate partitioning.",
							Optional:    true,
						},
						"func_name": schema.StringAttribute{
							Description: "The name of the function of a function partitioning.",
							Optional:    true,
						},
						"func_args": schema.ListAttribute{
							Description: "The function arguments as dotted field paths. Only field arguments are " +
								"supported.",
							Optional:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
			"index": schema.ListNestedBlock{
				Description: "An index of the table. Gravitino v1.3.0 defines no update request for the indexes of an " +
					"existing table, so changing this block replaces the table.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"index_type": schema.StringAttribute{
							Description: "The type of the index.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf("primary_key", "unique_key"),
							},
						},
						"name": schema.StringAttribute{
							Description: "The name of the index. Catalogs may assign a name when it is omitted; the name " +
								"reported by Gravitino is stored in that case.",
							Optional: true,
							Computed: true,
						},
						"field_names": schema.ListAttribute{
							Description: "The indexed fields, each entry holding the path segments of a field.",
							Required:    true,
							ElementType: types.ListType{ElemType: types.StringType},
						},
					},
				},
			},
		},
	}
}

// ConfigValidators validates the parts of the configuration Terraform cannot
// express in the schema.
func (r *tableResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{distributionNumberValidator{}}
}

// distributionNumberValidator requires the number of a configured distribution:
// tables.yaml marks it as required, but it cannot be a required attribute of a
// nested block, because Terraform then refuses a configuration without the
// distribution block at all.
type distributionNumberValidator struct{}

func (v distributionNumberValidator) Description(_ context.Context) string {
	return "the number of a distribution must be set when a distribution block is configured"
}

func (v distributionNumberValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v distributionNumberValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config models.TableResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Distribution == nil || config.Distribution.Number.IsUnknown() || !config.Distribution.Number.IsNull() {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("distribution").AtName("number"),
		"Missing distribution number",
		"tables.yaml requires the number of a distribution, and it must be greater than 0.",
	)
}

// ModifyPlan canonicalises the planned column types and comments, and forces a
// replacement for the changes Gravitino cannot apply in place.
func (r *tableResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan, state models.TableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	hasState := !req.State.Raw.IsNull()
	if hasState {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	canonicalisePlan(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !hasState {
		return
	}

	if sortOrdersChanged(plan.SortOrders, state.SortOrders) ||
		distributionChanged(plan.Distribution, state.Distribution) ||
		partitioningChanged(plan.Partitioning, state.Partitioning) ||
		indexesChanged(plan.Indexes, state.Indexes) {
		for _, name := range []string{"sort_order", "distribution", "partitioning", "index"} {
			resp.RequiresReplace = append(resp.RequiresReplace, path.Root(name))
		}
	}

	if columnsReplaceRequired(plan.Columns, state.Columns) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("column"))
	}
}

// canonicalisePlan makes the plan deterministic before it is compared with the
// state: column types are rewritten to their canonical form and empty comments
// become unset, matching the values Gravitino reports back.
func canonicalisePlan(ctx context.Context, plan *models.TableResourceModel, diags *diag.Diagnostics) {
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() && plan.Comment.ValueString() == "" {
		plan.Comment = types.StringNull()
	}

	for i := range plan.Columns {
		column := &plan.Columns[i]

		if !column.Comment.IsNull() && !column.Comment.IsUnknown() && column.Comment.ValueString() == "" {
			column.Comment = types.StringNull()
		}

		if column.Type.IsNull() || column.Type.IsUnknown() {
			continue
		}

		dataType, err := models.ParseDataType(column.Type.ValueString())
		if err != nil {
			diags.AddAttributeError(
				path.Root("column").AtListIndex(i).AtName("type"),
				"Invalid column type",
				err.Error(),
			)
			continue
		}

		if canonical := dataType.String(); canonical != column.Type.ValueString() {
			column.Type = types.StringValue(canonical)
		}
	}
}

// Create creates the table.
func (r *tableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.TableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating table", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})

	createReq, diags := r.buildCreateRequest(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	tableResp, err := r.client.CreateTable(
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &created)...)

	tflog.Debug(ctx, "Created table", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})
}

// Read refreshes the table and drops it from state when it no longer exists.
func (r *tableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.TableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

	tableResp, err := r.client.GetTable(
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	tflog.Debug(ctx, "Read table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})
}

// Update applies the table updates of tables.yaml.
func (r *tableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state models.TableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

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

	propertyUpdates, propertyDiags := tablePropertyUpdates(ctx, plan.Properties, state.Properties)
	resp.Diagnostics.Append(propertyDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updates = append(updates, propertyUpdates...)

	columnUpdates, columnDiags := tableColumnUpdates(plan.Columns, state.Columns)
	resp.Diagnostics.Append(columnDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updates = append(updates, columnUpdates...)

	if len(updates) > 0 {
		if _, err := r.client.UpdateTable(
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

	tableResp, err := r.client.GetTable(
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &updated)...)

	tflog.Debug(ctx, "Updated table", map[string]interface{}{
		"metalake": plan.Metalake.ValueString(),
		"catalog":  plan.Catalog.ValueString(),
		"schema":   plan.Schema.ValueString(),
		"name":     plan.Name.ValueString(),
	})
}

// Delete drops the table. A table that is already gone is treated as deleted.
func (r *tableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.TableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Dropping table", map[string]interface{}{
		"metalake": state.Metalake.ValueString(),
		"catalog":  state.Catalog.ValueString(),
		"schema":   state.Schema.ValueString(),
		"name":     state.Name.ValueString(),
	})

	_, err := r.client.DropTable(
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
func (r *tableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ".", 4)
	if len(parts) != 4 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format 'metalake.catalog.schema.table', got %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("catalog"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("schema"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// buildCreateRequest converts the planned table into a TableCreateRequest.
func (r *tableResource) buildCreateRequest(ctx context.Context, plan *models.TableResourceModel) (*models.TableCreateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	properties, propertyDiags := models.TablePropertiesFromModel(ctx, plan.Properties)
	diags.Append(propertyDiags...)

	columns, columnDiags := models.TableColumnsFromModel(plan.Columns)
	diags.Append(columnDiags...)

	sortOrders, sortOrderDiags := models.TableSortOrdersFromModel(ctx, plan.SortOrders)
	diags.Append(sortOrderDiags...)

	distribution, distributionDiags := models.TableDistributionFromModel(ctx, plan.Distribution)
	diags.Append(distributionDiags...)

	partitioning, partitioningDiags := models.TablePartitioningFromModel(ctx, plan.Partitioning)
	diags.Append(partitioningDiags...)

	indexes, indexDiags := models.TableIndexesFromModel(ctx, plan.Indexes)
	diags.Append(indexDiags...)
	if diags.HasError() {
		return nil, diags
	}

	return &models.TableCreateRequest{
		Name:         plan.Name.ValueString(),
		Comment:      plan.Comment.ValueString(),
		Properties:   filterReservedProperties(properties),
		Columns:      columns,
		SortOrders:   sortOrders,
		Distribution: distribution,
		Partitioning: partitioning,
		Indexes:      indexes,
	}, diags
}

// tableStateFromServer refreshes the model from a table reported by Gravitino.
// preferServerProperties selects whether the properties reported by Gravitino
// or the properties already in the model win for keys that both know.
func tableStateFromServer(ctx context.Context, table *models.Table, state *models.TableResourceModel, preferServerProperties bool, diags *diag.Diagnostics) models.TableResourceModel {
	refreshed := *state

	refreshed.Name = types.StringValue(table.Name)
	if table.Comment == "" {
		refreshed.Comment = types.StringNull()
	} else {
		refreshed.Comment = types.StringValue(table.Comment)
	}

	audit, auditDiags := models.AuditToObjectValue(ctx, table.Audit)
	diags.Append(auditDiags...)

	properties, propertyDiags := mergeTableProperties(ctx, table.Properties, state.Properties, preferServerProperties)
	diags.Append(propertyDiags...)
	if diags.HasError() {
		return refreshed
	}

	refreshed.Audit = audit
	refreshed.Properties = properties
	refreshed.Columns = models.TableColumnsToModel(ctx, table.Columns, diags)
	refreshed.SortOrders = models.TableSortOrdersToModel(ctx, table.SortOrders, diags)
	refreshed.Distribution = models.TableDistributionToModel(ctx, table.Distribution, diags)
	refreshed.Partitioning = models.TablePartitioningToModel(ctx, table.Partitioning, diags)
	refreshed.Indexes = models.TableIndexesToModel(ctx, table.Indexes, diags)
	refreshed.ID = types.StringValue(tableID(
		refreshed.Metalake.ValueString(),
		refreshed.Catalog.ValueString(),
		refreshed.Schema.ValueString(),
		table.Name,
	))

	return refreshed
}

// mergeTableProperties combines the properties reported by Gravitino with the
// properties the configuration manages.
//
// A catalog may add its own properties (Hive reports location, table-type and
// serde information). Those are only surfaced when the configuration manages no
// properties at all, so that they cannot turn into permanent drift.
func mergeTableProperties(ctx context.Context, serverProperties map[string]string, desired types.Map, preferServer bool) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics

	filtered := filterReservedProperties(serverProperties)

	if desired.IsNull() || desired.IsUnknown() {
		if len(filtered) == 0 {
			return types.MapNull(types.StringType), diags
		}
		properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, filtered)
		diags.Append(propertyDiags...)
		return properties, diags
	}

	managed := make(map[string]string)
	diags.Append(desired.ElementsAs(ctx, &managed, false)...)
	if diags.HasError() {
		return desired, diags
	}
	if len(managed) == 0 {
		return desired, diags
	}

	merged := make(map[string]string, len(managed))
	for key, value := range managed {
		if serverValue, ok := filtered[key]; ok && preferServer {
			merged[key] = serverValue
			continue
		}
		merged[key] = value
	}

	properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, merged)
	diags.Append(propertyDiags...)
	return properties, diags
}

// tablePropertyUpdates diffs the managed properties into setProperty and
// removeProperty updates. The configuration is the complete set of managed
// properties, so a key that is no longer configured is removed.
func tablePropertyUpdates(ctx context.Context, plan, state types.Map) ([]models.TableUpdateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	planned, planDiags := models.TablePropertiesFromModel(ctx, plan)
	diags.Append(planDiags...)

	current, stateDiags := models.TablePropertiesFromModel(ctx, state)
	diags.Append(stateDiags...)
	if diags.HasError() {
		return nil, diags
	}

	var updates []models.TableUpdateRequest

	for key, value := range planned {
		if reservedProperties[key] {
			continue
		}
		if oldValue, exists := current[key]; !exists || oldValue != value {
			updates = append(updates, models.NewSetTablePropertyRequest(key, value))
		}
	}

	for key := range current {
		if reservedProperties[key] {
			continue
		}
		if _, exists := planned[key]; !exists {
			updates = append(updates, models.NewRemoveTablePropertyRequest(key))
		}
	}

	sort.Slice(updates, func(i, j int) bool {
		if updates[i].Property != updates[j].Property {
			return updates[i].Property < updates[j].Property
		}
		return updates[i].Type < updates[j].Type
	})

	return updates, diags
}

// tableColumnUpdates builds the column updates of an existing column.
//
// Columns are matched by name: a change to the set of names needs a new table
// (see columnsReplaceRequired), so only the updatable properties of a column
// are turned into requests here.
func tableColumnUpdates(plan, state []models.ColumnTFSDK) ([]models.TableUpdateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	planned := make(map[string]models.ColumnTFSDK, len(plan))
	for _, column := range plan {
		planned[column.Name.ValueString()] = column
	}

	var updates []models.TableUpdateRequest

	for _, column := range state {
		name := column.Name.ValueString()
		desired, exists := planned[name]
		if !exists {
			continue
		}

		desiredType, err := models.ParseDataType(desired.Type.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root("column"), "Invalid column type", err.Error())
			return nil, diags
		}

		currentType, err := models.ParseDataType(column.Type.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root("column"), "Invalid column type", err.Error())
			return nil, diags
		}

		if desiredType.String() != currentType.String() {
			updates = append(updates, models.NewUpdateTableColumnTypeRequest([]string{name}, desiredType))
		}

		if !desired.Comment.Equal(column.Comment) {
			updates = append(updates, models.NewUpdateTableColumnCommentRequest([]string{name}, desired.Comment.ValueString()))
		}

		if !desired.Nullable.Equal(column.Nullable) {
			updates = append(updates, models.NewUpdateTableColumnNullabilityRequest([]string{name}, desired.Nullable.ValueBool()))
		}

		if !desired.DefaultValue.Equal(column.DefaultValue) {
			updates = append(updates, models.NewUpdateTableColumnDefaultValueRequest([]string{name}, models.ColumnDefaultValue(desired, desiredType)))
		}
	}

	updates = append(updates, tableColumnPositionUpdates(plan, state)...)

	return updates, diags
}

// tableColumnPositionUpdates reorders the columns when their relative order
// changed. Gravitino applies the requests in order, so placing every column
// after its predecessor yields exactly the planned order.
func tableColumnPositionUpdates(plan, state []models.ColumnTFSDK) []models.TableUpdateRequest {
	stateOrder := make([]string, 0, len(state))
	for _, column := range state {
		stateOrder = append(stateOrder, column.Name.ValueString())
	}

	planOrder := make([]string, 0, len(plan))
	for _, column := range plan {
		planOrder = append(planOrder, column.Name.ValueString())
	}

	if reflect.DeepEqual(stateOrder, planOrder) {
		return nil
	}

	updates := make([]models.TableUpdateRequest, 0, len(planOrder))
	for i, name := range planOrder {
		if i == 0 {
			updates = append(updates, models.NewUpdateTableColumnPositionRequest([]string{name}, models.FirstColumnPosition()))
			continue
		}
		updates = append(updates, models.NewUpdateTableColumnPositionRequest([]string{name}, models.AfterColumnPosition(planOrder[i-1])))
	}

	return updates
}

// columnsReplaceRequired reports whether the column change needs a new table:
// the set of column names changed, or a default value was removed. Gravitino
// cannot tell a renamed column from a deleted and added one, and it cannot
// clear a column default value.
func columnsReplaceRequired(plan, state []models.ColumnTFSDK) bool {
	planned := make(map[string]models.ColumnTFSDK, len(plan))
	for _, column := range plan {
		if column.Name.IsUnknown() {
			return false
		}
		planned[column.Name.ValueString()] = column
	}

	if len(planned) != len(state) {
		return true
	}

	for _, column := range state {
		desired, exists := planned[column.Name.ValueString()]
		if !exists {
			return true
		}

		hasDefault := !column.DefaultValue.IsNull() && !column.DefaultValue.IsUnknown()
		keepsDefault := !desired.DefaultValue.IsNull() && !desired.DefaultValue.IsUnknown()
		if hasDefault && !keepsDefault {
			return true
		}
	}

	return false
}

// immutableBlockChange reports the first block Gravitino cannot update in place.
func immutableBlockChange(plan, state models.TableResourceModel) string {
	if sortOrdersChanged(plan.SortOrders, state.SortOrders) {
		return "sort_order"
	}
	if distributionChanged(plan.Distribution, state.Distribution) {
		return "distribution"
	}
	if partitioningChanged(plan.Partitioning, state.Partitioning) {
		return "partitioning"
	}
	if indexesChanged(plan.Indexes, state.Indexes) {
		return "index"
	}
	return ""
}

// The changed helpers compare the blocks through the API model, so that a
// cosmetic difference in the Terraform representation is not treated as a
// change. Unknown planned values are not compared: they are resolved during
// apply and guarded by the Update method.

func sortOrdersChanged(plan, state []models.SortOrderTFSDK) bool {
	if sortOrderUnknown(plan) {
		return false
	}

	planned, planDiags := models.TableSortOrdersFromModel(context.Background(), plan)
	current, stateDiags := models.TableSortOrdersFromModel(context.Background(), state)
	if planDiags.HasError() || stateDiags.HasError() {
		return false
	}
	return !reflect.DeepEqual(planned, current)
}

func distributionChanged(plan, state *models.DistributionTFSDK) bool {
	if distributionUnknown(plan) {
		return false
	}

	planned, planDiags := models.TableDistributionFromModel(context.Background(), plan)
	current, stateDiags := models.TableDistributionFromModel(context.Background(), state)
	if planDiags.HasError() || stateDiags.HasError() {
		return false
	}
	if planned == nil || current == nil {
		return planned != current
	}
	return !reflect.DeepEqual(*planned, *current)
}

func partitioningChanged(plan, state []models.PartitioningTFSDK) bool {
	if partitioningUnknown(plan) {
		return false
	}

	planned, planDiags := models.TablePartitioningFromModel(context.Background(), plan)
	current, stateDiags := models.TablePartitioningFromModel(context.Background(), state)
	if planDiags.HasError() || stateDiags.HasError() {
		return false
	}
	return !reflect.DeepEqual(planned, current)
}

func indexesChanged(plan, state []models.IndexTFSDK) bool {
	if indexUnknown(plan) {
		return false
	}

	planned, planDiags := models.TableIndexesFromModel(context.Background(), plan)
	current, stateDiags := models.TableIndexesFromModel(context.Background(), state)
	if planDiags.HasError() || stateDiags.HasError() {
		return false
	}
	return !reflect.DeepEqual(planned, current)
}

func sortOrderUnknown(orders []models.SortOrderTFSDK) bool {
	for _, order := range orders {
		if order.FieldName.IsUnknown() || order.Direction.IsUnknown() || order.NullOrdering.IsUnknown() {
			return true
		}
	}
	return false
}

func distributionUnknown(distribution *models.DistributionTFSDK) bool {
	if distribution == nil {
		return false
	}
	return distribution.Strategy.IsUnknown() || distribution.Number.IsUnknown() || distribution.FuncArgs.IsUnknown()
}

func partitioningUnknown(parts []models.PartitioningTFSDK) bool {
	for _, part := range parts {
		if part.Strategy.IsUnknown() || part.FieldName.IsUnknown() || part.FieldNames.IsUnknown() ||
			part.NumBuckets.IsUnknown() || part.Width.IsUnknown() || part.FuncName.IsUnknown() || part.FuncArgs.IsUnknown() {
			return true
		}
	}
	return false
}

func indexUnknown(indexes []models.IndexTFSDK) bool {
	for _, index := range indexes {
		if index.IndexType.IsUnknown() || index.Name.IsUnknown() || index.FieldNames.IsUnknown() {
			return true
		}
	}
	return false
}

func tableID(metalake, catalog, schema, name string) string {
	return fmt.Sprintf("%s.%s.%s.%s", metalake, catalog, schema, name)
}

// dataTypeStringValidator validates that a column type is a primitive type
// name or a structured data type that Gravitino understands.
type dataTypeStringValidator struct{}

func (v dataTypeStringValidator) Description(_ context.Context) string {
	return "the value must be a Gravitino primitive type name or a JSON object describing a structured data type"
}

func (v dataTypeStringValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v dataTypeStringValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if _, err := models.ParseDataType(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid column type", err.Error())
	}
}
