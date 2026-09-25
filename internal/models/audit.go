package models

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// AuditAttrTypes defines the framework attribute types for the shared
// audit object used across resources and data sources.
var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

// AuditToObjectValue converts a models.Audit into a framework object value
// using the shared AuditAttrTypes. It returns a null object when audit is nil.
func AuditToObjectValue(ctx context.Context, audit *Audit) (types.Object, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
	}

	attrs := map[string]attr.Value{
		"creator":            types.StringValue(audit.Creator),
		"last_modifier":      types.StringValue(audit.LastModifier),
		"create_time":        auditTimeToString(audit.CreateTime),
		"last_modified_time": auditTimeToString(audit.LastModifiedTime),
	}

	return types.ObjectValue(AuditAttrTypes, attrs)
}

// AuditToBasetypesObjectValue converts a models.Audit into a basetypes object
// value, mirroring AuditToObjectValue. Data sources may use this directly.
func AuditToBasetypesObjectValue(ctx context.Context, audit *Audit) (basetypes.ObjectValue, diag.Diagnostics) {
	obj, diags := AuditToObjectValue(ctx, audit)
	return obj, diags
}

func auditTimeToString(t *time.Time) types.String {
	if t == nil {
		return types.StringNull()
	}
	return types.StringValue(t.Format(time.RFC3339))
}
