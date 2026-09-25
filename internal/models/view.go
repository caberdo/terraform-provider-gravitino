package models

import (
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
