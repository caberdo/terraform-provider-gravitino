package models

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ViewRepresentation mirrors views.yaml#/components/schemas/SQLRepresentation.
type ViewRepresentation struct {
	Type    string `json:"type"`
	Dialect string `json:"dialect"`
	SQL     string `json:"sql"`
}

// View mirrors views.yaml#/components/schemas/View.
type View struct {
	Name            string               `json:"name"`
	Comment         string               `json:"comment,omitempty"`
	Columns         []Column             `json:"columns"`
	Representations []ViewRepresentation `json:"representations"`
	DefaultCatalog  string               `json:"defaultCatalog,omitempty"`
	DefaultSchema   string               `json:"defaultSchema,omitempty"`
	Properties      map[string]string    `json:"properties,omitempty"`
	Audit           *Audit               `json:"audit,omitempty"`
}

type ViewResponse struct {
	Code int  `json:"code"`
	View View `json:"view"`
}

// ViewCreateRequest mirrors views.yaml#/components/schemas/ViewCreateRequest.
type ViewCreateRequest struct {
	Name            string               `json:"name"`
	Comment         string               `json:"comment,omitempty"`
	Columns         []Column             `json:"columns,omitempty"`
	Representations []ViewRepresentation `json:"representations"`
	DefaultCatalog  string               `json:"defaultCatalog,omitempty"`
	DefaultSchema   string               `json:"defaultSchema,omitempty"`
	Properties      map[string]string    `json:"properties,omitempty"`
}

type ViewUpdateRequest struct {
	Updates []interface{} `json:"updates"`
}

// NewRenameViewRequest builds the "rename" update of ViewUpdateRequest.
func NewRenameViewRequest(newName string) interface{} {
	return struct {
		Type    string `json:"@type"`
		NewName string `json:"newName"`
	}{Type: "rename", NewName: newName}
}

// NewSetViewPropertyRequest builds the "setProperty" update of ViewUpdateRequest.
func NewSetViewPropertyRequest(property, value string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
		Value    string `json:"value"`
	}{Type: "setProperty", Property: property, Value: value}
}

// NewRemoveViewPropertyRequest builds the "removeProperty" update of ViewUpdateRequest.
func NewRemoveViewPropertyRequest(property string) interface{} {
	return struct {
		Type     string `json:"@type"`
		Property string `json:"property"`
	}{Type: "removeProperty", Property: property}
}

// ViewRepresentationTFSDK is the framework representation of a view representation.
type ViewRepresentationTFSDK struct {
	Type    types.String `tfsdk:"type"`
	Dialect types.String `tfsdk:"dialect"`
	SQL     types.String `tfsdk:"sql"`
}

// ViewRepresentationsToAPI converts framework representations into the API representation.
func ViewRepresentationsToAPI(reps []ViewRepresentationTFSDK) []ViewRepresentation {
	result := make([]ViewRepresentation, 0, len(reps))
	for _, rep := range reps {
		result = append(result, ViewRepresentation{
			Type:    rep.Type.ValueString(),
			Dialect: rep.Dialect.ValueString(),
			SQL:     rep.SQL.ValueString(),
		})
	}
	return result
}

// ViewRepresentationsToState converts API representations into the framework representation.
func ViewRepresentationsToState(reps []ViewRepresentation) []ViewRepresentationTFSDK {
	result := make([]ViewRepresentationTFSDK, 0, len(reps))
	for _, rep := range reps {
		result = append(result, ViewRepresentationTFSDK{
			Type:    types.StringValue(rep.Type),
			Dialect: types.StringValue(rep.Dialect),
			SQL:     types.StringValue(rep.SQL),
		})
	}
	return result
}

// ViewRepresentationAttrTypes returns the attribute types of a view
// representation object.
func ViewRepresentationAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"type":    types.StringType,
		"dialect": types.StringType,
		"sql":     types.StringType,
	}
}

// ViewRepresentationsFromList decodes a representation list. Null and unknown
// lists yield no representations.
func ViewRepresentationsFromList(ctx context.Context, value types.List, diags *diag.Diagnostics) []ViewRepresentationTFSDK {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var representations []ViewRepresentationTFSDK
	diags.Append(value.ElementsAs(ctx, &representations, false)...)
	if diags.HasError() {
		return nil
	}
	return representations
}

// ViewRepresentationsToList encodes representations as a framework list. A nil
// slice yields a null list.
func ViewRepresentationsToList(ctx context.Context, representations []ViewRepresentationTFSDK, diags *diag.Diagnostics) types.List {
	elementType := types.ObjectType{AttrTypes: ViewRepresentationAttrTypes()}
	if representations == nil {
		return types.ListNull(elementType)
	}
	items := make([]attr.Value, 0, len(representations))
	for _, representation := range representations {
		object, objectDiags := types.ObjectValueFrom(ctx, ViewRepresentationAttrTypes(), representation)
		diags.Append(objectDiags...)
		if diags.HasError() {
			return types.ListNull(elementType)
		}
		items = append(items, object)
	}
	list, listDiags := types.ListValue(elementType, items)
	diags.Append(listDiags...)
	return list
}

// ViewResourceTFModel is the Plugin Framework representation of the
// gravitino_view resource. The nested collections use framework types because a
// Go slice cannot hold the unknown values Terraform produces during planning.
type ViewResourceTFModel struct {
	ID              types.String `tfsdk:"id"`
	Metalake        types.String `tfsdk:"metalake"`
	Catalog         types.String `tfsdk:"catalog"`
	Schema          types.String `tfsdk:"schema"`
	Name            types.String `tfsdk:"name"`
	Comment         types.String `tfsdk:"comment"`
	Columns         types.List   `tfsdk:"column"`
	Representations types.List   `tfsdk:"representation"`
	DefaultCatalog  types.String `tfsdk:"default_catalog"`
	DefaultSchema   types.String `tfsdk:"default_schema"`
	Properties      types.Map    `tfsdk:"properties"`
	Audit           types.Object `tfsdk:"audit"`
}

// ViewDataSourceTFModel is the Plugin Framework representation of the
// gravitino_view data source.
type ViewDataSourceTFModel struct {
	Metalake        types.String `tfsdk:"metalake"`
	Catalog         types.String `tfsdk:"catalog"`
	Schema          types.String `tfsdk:"schema"`
	Name            types.String `tfsdk:"name"`
	Comment         types.String `tfsdk:"comment"`
	Columns         types.List   `tfsdk:"column"`
	Representations types.List   `tfsdk:"representation"`
	DefaultCatalog  types.String `tfsdk:"default_catalog"`
	DefaultSchema   types.String `tfsdk:"default_schema"`
	Properties      types.Map    `tfsdk:"properties"`
	Audit           types.Object `tfsdk:"audit"`
}

// ViewListEntryTFModel is the Plugin Framework representation of one entry of
// the gravitino_views data source.
type ViewListEntryTFModel struct {
	Name            types.String `tfsdk:"name"`
	Comment         types.String `tfsdk:"comment"`
	Columns         types.List   `tfsdk:"column"`
	Representations types.List   `tfsdk:"representation"`
	DefaultCatalog  types.String `tfsdk:"default_catalog"`
	DefaultSchema   types.String `tfsdk:"default_schema"`
	Properties      types.Map    `tfsdk:"properties"`
	Audit           types.Object `tfsdk:"audit"`
}

// ViewsDataSourceTFModel is the Plugin Framework representation of the
// gravitino_views data source.
type ViewsDataSourceTFModel struct {
	Metalake types.String `tfsdk:"metalake"`
	Catalog  types.String `tfsdk:"catalog"`
	Schema   types.String `tfsdk:"schema"`
	Views    types.List   `tfsdk:"views"`
}

// ViewListEntryAttrTypes returns the attribute types of a views list entry.
func ViewListEntryAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":            types.StringType,
		"comment":         types.StringType,
		"column":          types.ListType{ElemType: types.ObjectType{AttrTypes: ColumnAttrTypes()}},
		"representation":  types.ListType{ElemType: types.ObjectType{AttrTypes: ViewRepresentationAttrTypes()}},
		"default_catalog": types.StringType,
		"default_schema":  types.StringType,
		"properties":      types.MapType{ElemType: types.StringType},
		"audit":           types.ObjectType{AttrTypes: AuditAttrTypes},
	}
}
