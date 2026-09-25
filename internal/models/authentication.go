package models

// AuthMeResponse mirrors the `AuthMeResponse` schema returned by
// GET /authn/me (docs/open-api/authn.yaml#/components/schemas/AuthMeResponse):
//
//	{ "code": 0, "principal": "admin" }
//
// The endpoint returns the authenticated principal only; it carries no roles
// or any other identity data.
type AuthMeResponse struct {
	Code      int    `json:"code"`
	Principal string `json:"principal"`
}
