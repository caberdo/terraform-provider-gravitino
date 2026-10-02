package models

// AuthMeResponse mirrors the `AuthMeResponse` schema returned by
// GET /authn/me (docs/open-api/authn.yaml#/components/schemas/AuthMeResponse):
//
//	{ "code": 0, "principal": "admin", "serviceAdmin": true }
//
// `principal` is the server-resolved principal name. `serviceAdmin` (added in
// Gravitino 1.3.1) reports whether the credential is a Gravitino service
// administrator; servers predating 1.3.1 omit the field, which decodes to
// false. The endpoint carries no roles or any other identity data.
type AuthMeResponse struct {
	Code         int    `json:"code"`
	Principal    string `json:"principal"`
	ServiceAdmin bool   `json:"serviceAdmin"`
}
