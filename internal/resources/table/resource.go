package table

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = (*tableResource)(nil)
var _ resource.ResourceWithImportState = (*tableResource)(nil)
var _ resource.ResourceWithConfigure = (*tableResource)(nil)

type tableResource struct {
	client *client.Client
}

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
		Attributes: map[string]schema.Attribute{
			"metalake": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"catalog": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"schema": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"comment": schema.StringAttribute{
				Optional: true,
			},
			"properties": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
			"id": schema.StringAttribute{
				Computed: true,
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: models.AuditAttrTypes,
			},
		},
		Blocks: map[string]schema.Block{
			"column": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
						"type": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf("boolean", "int", "long", "float", "double", "decimal", "fixed", "string", "varchar", "binary", "date", "time", "timestamp", "uuid", "struct", "list", "map", "union"),
							},
						},
						"length": schema.Int64Attribute{
							Optional: true,
							Computed: true,
							Default:  int64default.StaticInt64(0),
						},
						"precision": schema.Int64Attribute{
							Optional: true,
							Computed: true,
							Default:  int64default.StaticInt64(0),
						},
						"scale": schema.Int64Attribute{
							Optional: true,
							Computed: true,
							Default:  int64default.StaticInt64(0),
						},
						"comment": schema.StringAttribute{
							Optional: true,
						},
						"nullable": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(true),
						},
						"auto_increment": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
						},
						"default_value": schema.StringAttribute{
							Optional: true,
						},
					},
				},
			},
			"sort_order": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"field_name": schema.ListAttribute{
							Required:    true,
							ElementType: types.StringType,
						},
						"direction": schema.StringAttribute{
							Optional: true,
							Computed: true,
							Default:  stringdefault.StaticString("asc"),
							Validators: []validator.String{
								stringvalidator.OneOf("asc", "desc"),
							},
						},
						"null_ordering": schema.StringAttribute{
							Optional: true,
							Validators: []validator.String{
								stringvalidator.OneOf("nulls_first", "nulls_last"),
							},
						},
					},
				},
			},
			"distribution": schema.SingleNestedBlock{
				Attributes: map[string]schema.Attribute{
					"strategy": schema.StringAttribute{
						Optional: true,
						Computed: true,
						Default:  stringdefault.StaticString("hash"),
						Validators: []validator.String{
							stringvalidator.OneOf("hash", "range", "even"),
						},
					},
					"number": schema.Int64Attribute{
						Required: true,
					},
					"func_args": schema.ListAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
				},
			},
			"partitioning": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"strategy": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf("identity", "year", "month", "day", "hour", "bucket", "truncate", "list", "range", "function"),
							},
						},
						"field_name": schema.ListAttribute{
							Optional:    true,
							ElementType: types.StringType,
						},
						"field_names": schema.ListAttribute{
							Optional:    true,
							ElementType: types.ListType{ElemType: types.StringType},
						},
						"num_buckets": schema.Int64Attribute{
							Optional: true,
						},
						"width": schema.Int64Attribute{
							Optional: true,
						},
						"func_name": schema.StringAttribute{
							Optional: true,
						},
						"func_args": schema.ListAttribute{
							Optional:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
			"index": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"index_type": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf("primary_key", "unique_key"),
							},
						},
						"name": schema.StringAttribute{
							Optional: true,
						},
						"field_names": schema.ListAttribute{
							Required:    true,
							ElementType: types.ListType{ElemType: types.StringType},
						},
					},
				},
			},
		},
	}
}

func (r *tableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.TableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Creating table", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})

	createReq := &models.TableCreateRequest{
		Name:    plan.Name.ValueString(),
		Comment: plan.Comment.ValueString(),
	}

	if !plan.Properties.IsNull() {
		props := make(map[string]string)
		resp.Diagnostics.Append(plan.Properties.ElementsAs(ctx, &props, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.Properties = props
	}

	createReq.Columns = r.buildColumns(ctx, plan.Columns, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq.SortOrders = r.buildSortOrders(ctx, plan.SortOrders, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Distribution != nil {
		d := r.buildDistribution(ctx, plan.Distribution, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.Distribution = &d
	}

	createReq.Partitioning = r.buildPartitioning(ctx, plan.Partitioning, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq.Indexes = r.buildIndexes(ctx, plan.Indexes, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tableResp, err := r.client.CreateTable(
		plan.Metalake.ValueString(),
		plan.Catalog.ValueString(),
		plan.Schema.ValueString(),
		createReq,
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating table", plan.Name.ValueString(), err)...)
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s",
		plan.Metalake.ValueString(),
		plan.Catalog.ValueString(),
		plan.Schema.ValueString(),
		plan.Name.ValueString(),
	))

	stateDiags := mapTableResponseToState(ctx, tableResp, &plan)
	resp.Diagnostics.Append(stateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Created table", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *tableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.TableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Reading table", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	tableResp, err := r.client.GetTable(
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

	diags := mapTableResponseToState(ctx, tableResp, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *tableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state models.TableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Updating table", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	var updates []interface{}

	if !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewRenameTableRequest(plan.Name.ValueString()))
	}

	if !plan.Comment.Equal(state.Comment) {
		updates = append(updates, models.NewUpdateTableCommentRequest(plan.Comment.ValueString()))
	}

	oldProps := make(map[string]string)
	newProps := make(map[string]string)
	if !state.Properties.IsNull() {
		resp.Diagnostics.Append(state.Properties.ElementsAs(ctx, &oldProps, false)...)
	}
	if !plan.Properties.IsNull() {
		resp.Diagnostics.Append(plan.Properties.ElementsAs(ctx, &newProps, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	for k, v := range newProps {
		oldV, exists := oldProps[k]
		if !exists || oldV != v {
			updates = append(updates, models.NewSetTablePropertyRequest(k, v))
		}
	}
	for k := range oldProps {
		if _, exists := newProps[k]; !exists {
			updates = append(updates, models.NewRemoveTablePropertyRequest(k))
		}
	}

	oldName := state.Name.ValueString()

	if len(updates) > 0 {
		_, err := r.client.UpdateTable(
			state.Metalake.ValueString(),
			state.Catalog.ValueString(),
			state.Schema.ValueString(),
			oldName,
			updates,
		)
		if err != nil {
			resp.Diagnostics.Append(client.NewResourceError("updating table", state.Name.ValueString(), err)...)
			return
		}
	}

	tableResp, err := r.client.GetTable(
		plan.Metalake.ValueString(),
		plan.Catalog.ValueString(),
		plan.Schema.ValueString(),
		plan.Name.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading table after update", state.Name.ValueString(), err)...)
		return
	}

	diags := mapTableResponseToState(ctx, tableResp, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	tflog.Debug(ctx, "Updated table", map[string]interface{}{"metalake": plan.Metalake.ValueString(), "catalog": plan.Catalog.ValueString(), "schema": plan.Schema.ValueString(), "name": plan.Name.ValueString()})
}

func (r *tableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.TableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleting table", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})

	_, err := r.client.DropTable(
		state.Metalake.ValueString(),
		state.Catalog.ValueString(),
		state.Schema.ValueString(),
		state.Name.ValueString(),
		true,
		false,
	)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("deleting table", state.Name.ValueString(), err)...)
		return
	}

	tflog.Debug(ctx, "Deleted table", map[string]interface{}{"metalake": state.Metalake.ValueString(), "catalog": state.Catalog.ValueString(), "schema": state.Schema.ValueString(), "name": state.Name.ValueString()})
}

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

func (r *tableResource) buildColumns(ctx context.Context, cols []models.ColumnTFSDK, diags *diag.Diagnostics) []models.Column {
	result := make([]models.Column, 0, len(cols))
	for _, col := range cols {
		c := models.Column{
			Name:    col.Name.ValueString(),
			Type:    typeNameToDataType(col.Type.ValueString(), col.Length.ValueInt64(), col.Precision.ValueInt64(), col.Scale.ValueInt64()),
			Comment: col.Comment.ValueString(),
		}

		if !col.Nullable.IsNull() {
			c.Nullable = col.Nullable.ValueBool()
		}
		if !col.AutoIncrement.IsNull() {
			c.AutoIncrement = col.AutoIncrement.ValueBool()
		}

		if !col.DefaultValue.IsNull() && col.DefaultValue.ValueString() != "" {
			var dv interface{}
			if err := json.Unmarshal([]byte(col.DefaultValue.ValueString()), &dv); err == nil {
				c.DefaultValue = dv
			}
		}

		result = append(result, c)
	}
	return result
}

func (r *tableResource) buildSortOrders(ctx context.Context, orders []models.SortOrderTFSDK, diags *diag.Diagnostics) []models.SortOrder {
	result := make([]models.SortOrder, 0, len(orders))
	for _, o := range orders {
		var fieldName []string
		if !o.FieldName.IsNull() {
			diags.Append(o.FieldName.ElementsAs(ctx, &fieldName, false)...)
		}

		so := models.SortOrder{
			SortTerm: models.Expression{
				Type:      "field",
				FieldName: fieldName,
			},
			Direction: o.Direction.ValueString(),
		}
		if !o.NullOrdering.IsNull() {
			so.NullOrdering = o.NullOrdering.ValueString()
		}
		result = append(result, so)
	}
	return result
}

func (r *tableResource) buildDistribution(ctx context.Context, d *models.DistributionTFSDK, diags *diag.Diagnostics) models.Distribution {
	dist := models.Distribution{
		Strategy: d.Strategy.ValueString(),
		Number:   int32(d.Number.ValueInt64()),
	}

	if !d.FuncArgs.IsNull() {
		var args []string
		diags.Append(d.FuncArgs.ElementsAs(ctx, &args, false)...)
		for _, arg := range args {
			dist.FuncArgs = append(dist.FuncArgs, models.Expression{
				Type:      "field",
				FieldName: strings.Split(arg, "."),
			})
		}
	}

	return dist
}

func (r *tableResource) buildPartitioning(ctx context.Context, parts []models.PartitioningTFSDK, diags *diag.Diagnostics) []models.Partitioning {
	result := make([]models.Partitioning, 0, len(parts))
	for _, p := range parts {
		part := models.Partitioning{
			Strategy: p.Strategy.ValueString(),
		}

		if !p.FieldName.IsNull() {
			diags.Append(p.FieldName.ElementsAs(ctx, &part.FieldName, false)...)
		}

		if !p.FieldNames.IsNull() {
			var rawLists []types.List
			diags.Append(p.FieldNames.ElementsAs(ctx, &rawLists, false)...)
			for _, list := range rawLists {
				var innerFields []string
				diags.Append(list.ElementsAs(ctx, &innerFields, false)...)
				part.FieldNames = append(part.FieldNames, innerFields)
			}
		}

		if !p.NumBuckets.IsNull() {
			part.NumBuckets = int(p.NumBuckets.ValueInt64())
		}
		if !p.Width.IsNull() {
			part.Width = int(p.Width.ValueInt64())
		}
		if !p.FuncName.IsNull() {
			part.FuncName = p.FuncName.ValueString()
		}

		if !p.FuncArgs.IsNull() {
			var args []string
			diags.Append(p.FuncArgs.ElementsAs(ctx, &args, false)...)
			for _, arg := range args {
				part.FuncArgs = append(part.FuncArgs, models.Expression{
					Type:      "field",
					FieldName: strings.Split(arg, "."),
				})
			}
		}

		result = append(result, part)
	}
	return result
}

func (r *tableResource) buildIndexes(ctx context.Context, indexes []models.IndexTFSDK, diags *diag.Diagnostics) []models.Index {
	result := make([]models.Index, 0, len(indexes))
	for _, idx := range indexes {
		index := models.Index{
			IndexType: idx.IndexType.ValueString(),
		}
		if !idx.Name.IsNull() {
			index.Name = idx.Name.ValueString()
		}

		if !idx.FieldNames.IsNull() {
			var rawLists []types.List
			diags.Append(idx.FieldNames.ElementsAs(ctx, &rawLists, false)...)
			for _, list := range rawLists {
				var innerFields []string
				diags.Append(list.ElementsAs(ctx, &innerFields, false)...)
				index.FieldNames = append(index.FieldNames, innerFields)
			}
		}

		result = append(result, index)
	}
	return result
}

func mapTableResponseToState(ctx context.Context, resp *models.TableResponse, state *models.TableResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	t := resp.Table

	state.Name = types.StringValue(t.Name)
	state.Comment = types.StringValue(t.Comment)

	if len(t.Properties) > 0 {
		props, d := types.MapValueFrom(ctx, types.StringType, t.Properties)
		diags.Append(d...)
		state.Properties = props
	}

	state.ID = types.StringValue(fmt.Sprintf("%s.%s.%s.%s",
		state.Metalake.ValueString(),
		state.Catalog.ValueString(),
		state.Schema.ValueString(),
		state.Name.ValueString(),
	))

	auditObj, d := models.AuditToObjectValue(ctx, t.Audit)
	diags.Append(d...)
	state.Audit = auditObj

	state.Columns = mapColumnsToState(ctx, t.Columns, &diags)
	state.SortOrders = mapSortOrdersToState(ctx, t.SortOrders, &diags)
	state.Distribution = mapDistributionToState(ctx, t.Distribution, &diags)
	state.Partitioning = mapPartitioningToState(ctx, t.Partitioning, &diags)
	state.Indexes = mapIndexesToState(ctx, t.Indexes, &diags)

	return diags
}

func mapColumnsToState(ctx context.Context, cols []models.Column, diags *diag.Diagnostics) []models.ColumnTFSDK {
	result := make([]models.ColumnTFSDK, 0, len(cols))
	for _, col := range cols {
		m := models.ColumnTFSDK{
			Name:          types.StringValue(col.Name),
			Type:          types.StringValue(columnTypeToString(col.Type)),
			Nullable:      types.BoolValue(col.Nullable),
			AutoIncrement: types.BoolValue(col.AutoIncrement),
		}

		if col.Comment != "" {
			m.Comment = types.StringValue(col.Comment)
		} else {
			m.Comment = types.StringNull()
		}

		m.Length = types.Int64Value(0)
		if col.Type.Length != nil {
			m.Length = types.Int64Value(*col.Type.Length)
		}
		m.Precision = types.Int64Value(0)
		if col.Type.Precision != nil {
			m.Precision = types.Int64Value(*col.Type.Precision)
		}
		m.Scale = types.Int64Value(0)
		if col.Type.Scale != nil {
			m.Scale = types.Int64Value(*col.Type.Scale)
		}

		if col.DefaultValue != nil {
			b, err := json.Marshal(col.DefaultValue)
			if err == nil {
				m.DefaultValue = types.StringValue(string(b))
			} else {
				m.DefaultValue = types.StringNull()
			}
		} else {
			m.DefaultValue = types.StringNull()
		}

		result = append(result, m)
	}
	return result
}

func mapSortOrdersToState(ctx context.Context, orders []models.SortOrder, diags *diag.Diagnostics) []models.SortOrderTFSDK {
	result := make([]models.SortOrderTFSDK, 0, len(orders))
	for _, o := range orders {
		m := models.SortOrderTFSDK{
			Direction: types.StringValue(o.Direction),
		}

		if o.NullOrdering != "" {
			m.NullOrdering = types.StringValue(o.NullOrdering)
		} else {
			m.NullOrdering = types.StringNull()
		}

		fieldVals := make([]attr.Value, 0, len(o.SortTerm.FieldName))
		for _, fn := range o.SortTerm.FieldName {
			fieldVals = append(fieldVals, types.StringValue(fn))
		}
		listVal, d := types.ListValue(types.StringType, fieldVals)
		diags.Append(d...)
		m.FieldName = listVal

		result = append(result, m)
	}
	return result
}

func mapDistributionToState(ctx context.Context, dist *models.Distribution, diags *diag.Diagnostics) *models.DistributionTFSDK {
	if dist == nil {
		return nil
	}

	m := &models.DistributionTFSDK{
		Strategy: types.StringValue(dist.Strategy),
		Number:   types.Int64Value(int64(dist.Number)),
	}

	if len(dist.FuncArgs) == 0 {
		m.FuncArgs = types.ListNull(types.StringType)
		return m
	}
	funcArgVals := make([]attr.Value, 0, len(dist.FuncArgs))
	for _, expr := range dist.FuncArgs {
		funcArgVals = append(funcArgVals, types.StringValue(strings.Join(expr.FieldName, ".")))
	}
	listVal, d := types.ListValue(types.StringType, funcArgVals)
	diags.Append(d...)
	m.FuncArgs = listVal

	return m
}

func mapPartitioningToState(ctx context.Context, parts []models.Partitioning, diags *diag.Diagnostics) []models.PartitioningTFSDK {
	result := make([]models.PartitioningTFSDK, 0, len(parts))
	for _, p := range parts {
		m := models.PartitioningTFSDK{
			Strategy: types.StringValue(p.Strategy),
		}

		if len(p.FieldName) > 0 {
			fieldVals := make([]attr.Value, 0, len(p.FieldName))
			for _, fn := range p.FieldName {
				fieldVals = append(fieldVals, types.StringValue(fn))
			}
			listVal, d := types.ListValue(types.StringType, fieldVals)
			diags.Append(d...)
			m.FieldName = listVal
		} else {
			m.FieldName = types.ListNull(types.StringType)
		}

		if len(p.FieldNames) > 0 {
			fieldNamesVals := make([]attr.Value, 0, len(p.FieldNames))
			for _, fns := range p.FieldNames {
				innerVals := make([]attr.Value, 0, len(fns))
				for _, fn := range fns {
					innerVals = append(innerVals, types.StringValue(fn))
				}
				innerList, d := types.ListValue(types.StringType, innerVals)
				diags.Append(d...)
				fieldNamesVals = append(fieldNamesVals, innerList)
			}
			listVal, d := types.ListValue(types.ListType{ElemType: types.StringType}, fieldNamesVals)
			diags.Append(d...)
			m.FieldNames = listVal
		} else {
			m.FieldNames = types.ListNull(types.ListType{ElemType: types.StringType})
		}

		if p.NumBuckets > 0 {
			m.NumBuckets = types.Int64Value(int64(p.NumBuckets))
		} else {
			m.NumBuckets = types.Int64Null()
		}

		if p.Width > 0 {
			m.Width = types.Int64Value(int64(p.Width))
		} else {
			m.Width = types.Int64Null()
		}

		if p.FuncName != "" {
			m.FuncName = types.StringValue(p.FuncName)
		} else {
			m.FuncName = types.StringNull()
		}

		if len(p.FuncArgs) > 0 {
			funcArgVals := make([]attr.Value, 0, len(p.FuncArgs))
			for _, expr := range p.FuncArgs {
				funcArgVals = append(funcArgVals, types.StringValue(strings.Join(expr.FieldName, ".")))
			}
			listVal, d := types.ListValue(types.StringType, funcArgVals)
			diags.Append(d...)
			m.FuncArgs = listVal
		} else {
			m.FuncArgs = types.ListNull(types.StringType)
		}

		result = append(result, m)
	}
	return result
}

func mapIndexesToState(ctx context.Context, indexes []models.Index, diags *diag.Diagnostics) []models.IndexTFSDK {
	result := make([]models.IndexTFSDK, 0, len(indexes))
	for _, idx := range indexes {
		m := models.IndexTFSDK{
			IndexType: types.StringValue(idx.IndexType),
		}

		if idx.Name != "" {
			m.Name = types.StringValue(idx.Name)
		} else {
			m.Name = types.StringNull()
		}

		fieldNamesVals := make([]attr.Value, 0, len(idx.FieldNames))
		for _, fns := range idx.FieldNames {
			innerVals := make([]attr.Value, 0, len(fns))
			for _, fn := range fns {
				innerVals = append(innerVals, types.StringValue(fn))
			}
			innerList, d := types.ListValue(types.StringType, innerVals)
			diags.Append(d...)
			fieldNamesVals = append(fieldNamesVals, innerList)
		}
		listVal, d := types.ListValue(types.ListType{ElemType: types.StringType}, fieldNamesVals)
		diags.Append(d...)
		m.FieldNames = listVal

		result = append(result, m)
	}
	return result
}
