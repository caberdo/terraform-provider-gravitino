package models

import "strings"

// SetOwnerRequest is the body of PUT /metalakes/{metalake}/owners/{type}/{fullName}
// (owners.yaml#/OwnerSetRequest): exactly `name` and `type`.
type SetOwnerRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type OwnerResponse struct {
	Code  int32 `json:"code"`
	Owner Owner `json:"owner"`
}

type Owner struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type SetOwnerResponse struct {
	Code int32 `json:"code"`
	Set  bool  `json:"set"`
}

// NormalizeOwnerType maps the owner type as echoed by Gravitino onto the
// canonical uppercase value of the owners.yaml enum.
//
// The API accepts "USER"/"GROUP" case-insensitively but returns the lowercase
// form ("user"/"group"), so responses must be normalised before they end up in
// Terraform state.
func NormalizeOwnerType(ownerType string) string {
	return strings.ToUpper(ownerType)
}
