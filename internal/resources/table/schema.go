package table

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

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
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
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
					"such as \"integer\" or \"varchar(255)\"; struct, list, map, union, unparsed and external columns " +
					"are written as a JSON object, for example jsonencode({type = \"struct\", fields = [...]}). Type, " +
					"comment, nullable, auto_increment, default_value and position changes of an existing column are " +
					"applied in place with the updateColumnType, updateColumnComment, updateColumnNullability, " +
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
								"or \"binary\", or a JSON object for the structured types struct, list, map, union, " +
								"unparsed and external. The external type " +
								"jsonencode({type = \"external\", catalogString = \"<catalog type>\"}) requires " +
								"Gravitino 1.3.1 or later.",
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
							Description: "The type of the index. The data_skipping_minmax, " +
								"data_skipping_bloom_filter and data_skipping_set types require Gravitino v1.3.1.",
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf(
									"primary_key",
									"unique_key",
									"data_skipping_minmax",
									"data_skipping_bloom_filter",
									"data_skipping_set",
								),
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
						"properties": schema.MapAttribute{
							Description: "Extra index properties, for example the granularity of a ClickHouse " +
								"data-skipping index. Requires Gravitino v1.3.1. Only the configured keys are " +
								"tracked; properties the catalog reports on its own are ignored.",
							Optional:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

// ConfigValidators validates the parts of the configuration Terraform cannot
// express in the schema.

// ConfigValidators validates the parts of the configuration Terraform cannot
// express in the schema.
func (r *tableResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{distributionNumberValidator{}}
}

// distributionNumberValidator requires the number of a configured distribution:
// tables.yaml marks it as required, but it cannot be a required attribute of a
// nested block, because Terraform then refuses a configuration without the
// distribution block at all.

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
	var config models.TableResourceTFModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	distribution := models.TableDistributionFromObject(ctx, config.Distribution, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if distribution == nil || distribution.Number.IsUnknown() || !distribution.Number.IsNull() {
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
