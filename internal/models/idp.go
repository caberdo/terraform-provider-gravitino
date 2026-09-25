package models

// The built-in IDP DTOs mirror docs/open-api/idp/idp.yaml (Gravitino v1.3.0).
//
// There is deliberately no `enabled` field on IdpUser and no `comment` field on
// IdpGroup: the IDP schemas expose only `name`, and (for a user) `groups`, so a
// field for either would always be empty in both directions.

// IdpUser mirrors components/schemas/IdpUser.
type IdpUser struct {
	Name   string   `json:"name"`
	Groups []string `json:"groups,omitempty"`
}

type IdpUserResponse struct {
	Code int32   `json:"code"`
	User IdpUser `json:"user"`
}

// IdpAddUserRequest mirrors components/schemas/AddUserRequest (POST /idp/users).
type IdpAddUserRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// IdpChangePasswordRequest mirrors components/schemas/ChangePasswordRequest,
// the body of PUT /idp/users/{user}. That operation can only change the
// password; group membership is managed through the group endpoints.
type IdpChangePasswordRequest struct {
	Password string `json:"password"`
}

// IdpGroup mirrors components/schemas/IdpGroup.
type IdpGroup struct {
	Name  string   `json:"name"`
	Users []string `json:"users,omitempty"`
}

type IdpGroupResponse struct {
	Code  int32    `json:"code"`
	Group IdpGroup `json:"group"`
}

// IdpAddGroupRequest mirrors components/schemas/AddGroupRequest
// (POST /idp/groups).
type IdpAddGroupRequest struct {
	Group string `json:"group"`
}

// IdpGroupMembershipChangeRequest mirrors
// components/schemas/GroupMembershipChangeRequest, the body of
// PUT /idp/groups/{group}/users.
type IdpGroupMembershipChangeRequest struct {
	UsersToAdd    []string `json:"usersToAdd,omitempty"`
	UsersToRemove []string `json:"usersToRemove,omitempty"`
}
