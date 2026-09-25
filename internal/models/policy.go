package models

import (
	"encoding/json"
	"strings"
)

// PolicyObjectTypes are the object types of CustomPolicyContent's
// `supportedObjectTypes` (policies.yaml).
var PolicyObjectTypes = []string{"CATALOG", "SCHEMA", "TABLE", "FILESET", "TOPIC", "MODEL"}

// NormalizePolicyObjectTypes maps the object types as returned by the API
// (measured on Gravitino 1.3.0: lower-cased, e.g. "catalog") back to the
// canonical upper-case enum values of the spec.
func NormalizePolicyObjectTypes(objectTypes []string) []string {
	if len(objectTypes) == 0 {
		return objectTypes
	}
	normalized := make([]string, len(objectTypes))
	for i, objectType := range objectTypes {
		normalized[i] = strings.ToUpper(objectType)
	}
	return normalized
}

// PolicyContent mirrors `CustomPolicyContent`. The v1.3.0 spec allows arbitrary
// JSON values in `customRules`, so values are decoded into their Terraform
// string representation: JSON strings are unwrapped, any other JSON value keeps
// its compact JSON encoding (e.g. the number 123 becomes "123").
type PolicyContent struct {
	SupportedObjectTypes []string          `json:"supportedObjectTypes"`
	Properties           map[string]string `json:"properties,omitempty"`
	CustomRules          map[string]string `json:"customRules,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler so that `customRules` values of any
// JSON type (the spec's own examples use numbers) do not fail decoding.
func (c *PolicyContent) UnmarshalJSON(data []byte) error {
	var raw struct {
		SupportedObjectTypes []string                   `json:"supportedObjectTypes"`
		Properties           map[string]string          `json:"properties,omitempty"`
		CustomRules          map[string]json.RawMessage `json:"customRules,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	c.SupportedObjectTypes = raw.SupportedObjectTypes
	c.Properties = raw.Properties
	c.CustomRules = nil
	if len(raw.CustomRules) > 0 {
		c.CustomRules = make(map[string]string, len(raw.CustomRules))
		for key, value := range raw.CustomRules {
			rule, err := customRuleToString(value)
			if err != nil {
				return err
			}
			c.CustomRules[key] = rule
		}
	}

	return nil
}

// customRuleToString renders a custom rule value as the string used by the
// Terraform schema.
func customRuleToString(raw json.RawMessage) (string, error) {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	if str, ok := value.(string); ok {
		return str, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

type Policy struct {
	Name       string         `json:"name"`
	Comment    string         `json:"comment,omitempty"`
	PolicyType string         `json:"policyType"`
	Enabled    bool           `json:"enabled"`
	Content    *PolicyContent `json:"content,omitempty"`
	Inherited  *bool          `json:"inherited,omitempty"`
	Audit      *Audit         `json:"audit,omitempty"`
}

type PolicyListResponse struct {
	Code     int32    `json:"code"`
	Policies []Policy `json:"policies"`
}

type PolicyResponse struct {
	Code   int32  `json:"code"`
	Policy Policy `json:"policy"`
}

type PolicyCreateRequest struct {
	Name       string         `json:"name"`
	Comment    string         `json:"comment,omitempty"`
	PolicyType string         `json:"policyType"`
	Enabled    bool           `json:"enabled"`
	Content    *PolicyContent `json:"content,omitempty"`
}

// PolicyUpdatesRequest mirrors `PolicyUpdatesRequest`, the body of
// PUT /metalakes/{metalake}/policies/{policy}.
type PolicyUpdatesRequest struct {
	Updates []interface{} `json:"updates"`
}

// RenamePolicyRequest mirrors `RenamePolicyRequest`.
type RenamePolicyRequest struct {
	Type    string `json:"@type"`
	NewName string `json:"newName"`
}

// UpdatePolicyCommentRequest mirrors `UpdatePolicyCommentRequest`.
type UpdatePolicyCommentRequest struct {
	Type       string `json:"@type"`
	NewComment string `json:"newComment"`
}

// UpdatePolicyContentRequest mirrors `UpdatePolicyContentRequest`.
type UpdatePolicyContentRequest struct {
	Type       string         `json:"@type"`
	PolicyType string         `json:"policyType"`
	NewContent *PolicyContent `json:"newContent"`
}

// PolicySetRequest mirrors `PolicySetRequest`, the body of
// PATCH /metalakes/{metalake}/policies/{policy}. The field is `enable`, not
// `enabled`.
type PolicySetRequest struct {
	Enable bool `json:"enable"`
}

func NewRenamePolicyRequest(newName string) RenamePolicyRequest {
	return RenamePolicyRequest{Type: "rename", NewName: newName}
}

func NewUpdatePolicyCommentRequest(newComment string) UpdatePolicyCommentRequest {
	return UpdatePolicyCommentRequest{Type: "updateComment", NewComment: newComment}
}

func NewUpdatePolicyContentRequest(policyType string, newContent *PolicyContent) UpdatePolicyContentRequest {
	return UpdatePolicyContentRequest{Type: "updateContent", PolicyType: policyType, NewContent: newContent}
}

type PolicyAssociationRequest struct {
	PoliciesToAdd    []string `json:"policiesToAdd,omitempty"`
	PoliciesToRemove []string `json:"policiesToRemove,omitempty"`
}
