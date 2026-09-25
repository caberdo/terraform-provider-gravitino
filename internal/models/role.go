package models

// RoleCreateRequest is the body of POST /metalakes/{metalake}/roles
// (roles.yaml#/components/schemas/RoleCreateRequest).
//
// SecurableObjects is deliberately not `omitempty`: the Gravitino server rejects a
// request without the field ("\"securableObjects\" can't null" in
// RoleCreateRequest.validate()), so the client always sends the array, empty or not.
type RoleCreateRequest struct {
	Name             string            `json:"name"`
	Properties       map[string]string `json:"properties,omitempty"`
	SecurableObjects []SecurableObject `json:"securableObjects"`
}

// RoleResponse is the response of every role endpoint
// (roles.yaml#/components/responses/RoleResponse).
type RoleResponse struct {
	Code int32 `json:"code"`
	Role Role  `json:"role"`
}

// Role is roles.yaml#/components/schemas/Role.
//
// The OpenAPI schema does not list `audit`, but the Gravitino v1.3.0 server always
// returns it (RoleDTO.audit, and RoleResponse.validate() rejects a role without
// audit info), so it is part of the wire model.
type Role struct {
	Name             string            `json:"name"`
	Properties       map[string]string `json:"properties,omitempty"`
	SecurableObjects []SecurableObject `json:"securableObjects"`
	Audit            *Audit            `json:"audit,omitempty"`
}

// SecurableObject is roles.yaml#/components/schemas/SecurableObject.
type SecurableObject struct {
	FullName   string      `json:"fullName"`
	Type       string      `json:"type"`
	Privileges []Privilege `json:"privileges"`
}

// Privilege is roles.yaml#/components/schemas/Privilege.
type Privilege struct {
	Name      string `json:"name"`
	Condition string `json:"condition"`
}

// PrivilegesRequest is the body of the grant and revoke privilege endpoints
// (permissions.yaml#/components/schemas/PrivilegeGrantRequest and
// PrivilegeRevokeRequest).
type PrivilegesRequest struct {
	Privileges []Privilege `json:"privileges"`
}

// PrivilegeOverrideRequest is the body of PUT /metalakes/{metalake}/permissions/roles/{role}
// (permissions.yaml#/components/schemas/PrivilegeOverrideRequest).
type PrivilegeOverrideRequest struct {
	Overrides []SecurableObject `json:"overrides"`
}
