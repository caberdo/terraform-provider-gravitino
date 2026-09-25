package models

// Credential mirrors the `Credential` schema of the Gravitino v1.3.0 credentials
// API (docs/open-api/credentials.yaml#/components/schemas/Credential):
//
//	{ "credentialType": "s3-token", "expireTimeInMs": 1735891948411,
//	  "credentialInfo": { "s3-access-key-id": "value1" } }
type Credential struct {
	// CredentialType is the type of the credential, for example s3-token,
	// s3-secret-key, oss-token, oss-secret-key, gcs-token, adls-token,
	// azure-account-key.
	CredentialType string `json:"credentialType"`
	// ExpireTimeInMs is the expiration time in milliseconds since the epoch.
	// 0 means the credential does not expire.
	ExpireTimeInMs int64 `json:"expireTimeInMs"`
	// CredentialInfo carries the credential type specific key/value pairs.
	CredentialInfo map[string]string `json:"credentialInfo"`
}

// CredentialResponse mirrors the `CredentialResponse` schema
// (docs/open-api/credentials.yaml#/components/responses/CredentialResponse).
// The list of credentials is carried under the key `credentials`.
type CredentialResponse struct {
	Code        int          `json:"code"`
	Credentials []Credential `json:"credentials"`
}
