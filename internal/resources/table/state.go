package table

import (
	"context"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// tableStateFromServer refreshes the model from a table reported by Gravitino.
// preferServerProperties selects whether the properties reported by Gravitino
// or the properties already in the model win for keys that both know.
func tableStateFromServer(ctx context.Context, table *models.Table, state *models.TableResourceModel, preferServerProperties bool, diags *diag.Diagnostics) models.TableResourceModel {
	refreshed := *state

	refreshed.Name = types.StringValue(table.Name)
	if table.Comment == "" {
		refreshed.Comment = types.StringNull()
	} else {
		refreshed.Comment = types.StringValue(table.Comment)
	}

	audit, auditDiags := models.AuditToObjectValue(ctx, table.Audit)
	diags.Append(auditDiags...)

	properties, propertyDiags := mergeTableProperties(ctx, table.Properties, state.Properties, preferServerProperties)
	diags.Append(propertyDiags...)
	if diags.HasError() {
		return refreshed
	}

	refreshed.Audit = audit
	refreshed.Properties = properties
	refreshed.Columns = models.TableColumnsToModel(ctx, table.Columns, diags)
	refreshed.SortOrders = models.TableSortOrdersToModel(ctx, table.SortOrders, diags)
	refreshed.Distribution = models.TableDistributionToModel(ctx, table.Distribution, diags)
	refreshed.Partitioning = models.TablePartitioningToModel(ctx, table.Partitioning, diags)
	refreshed.Indexes = mergeTableIndexes(ctx, table.Indexes, state.Indexes, preferServerProperties, diags)
	refreshed.ID = types.StringValue(tableID(
		refreshed.Metalake.ValueString(),
		refreshed.Catalog.ValueString(),
		refreshed.Schema.ValueString(),
		table.Name,
	))

	return refreshed
}

// mergeTableProperties combines the properties reported by Gravitino with the
// properties the configuration manages.
//
// A catalog may add its own properties (Hive reports location, table-type and
// serde information). Those are only surfaced when the configuration manages no
// properties at all, so that they cannot turn into permanent drift.

// mergeTableProperties combines the properties reported by Gravitino with the
// properties the configuration manages.
//
// A catalog may add its own properties (Hive reports location, table-type and
// serde information). Those are only surfaced when the configuration manages no
// properties at all, so that they cannot turn into permanent drift.
func mergeTableProperties(ctx context.Context, serverProperties map[string]string, desired types.Map, preferServer bool) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics

	filtered := filterReservedProperties(serverProperties)

	if desired.IsNull() || desired.IsUnknown() {
		if len(filtered) == 0 {
			return types.MapNull(types.StringType), diags
		}
		properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, filtered)
		diags.Append(propertyDiags...)
		return properties, diags
	}

	managed := make(map[string]string)
	diags.Append(desired.ElementsAs(ctx, &managed, false)...)
	if diags.HasError() {
		return desired, diags
	}
	if len(managed) == 0 {
		return desired, diags
	}

	merged := make(map[string]string, len(managed))
	for key, value := range managed {
		if serverValue, ok := filtered[key]; ok && preferServer {
			merged[key] = serverValue
			continue
		}
		merged[key] = value
	}

	properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, merged)
	diags.Append(propertyDiags...)
	return properties, diags
}

// mergeTableIndexes combines the indexes reported by Gravitino with the index
// properties the configuration manages, mirroring mergeTableProperties: a
// catalog may drop or rewrite an index property, and the configured properties
// must not turn into permanent drift (the block is immutable, so drift would
// replace the table on every plan).
//
// Indexes are matched by position. A catalog that reports a different number of
// indexes changed the block itself, which is a replacement the plan detects
// through the index type and field names, so the reported indexes are then
// returned as Gravitino sent them.

// mergeTableIndexes combines the indexes reported by Gravitino with the index
// properties the configuration manages, mirroring mergeTableProperties: a
// catalog may drop or rewrite an index property, and the configured properties
// must not turn into permanent drift (the block is immutable, so drift would
// replace the table on every plan).
//
// Indexes are matched by position. A catalog that reports a different number of
// indexes changed the block itself, which is a replacement the plan detects
// through the index type and field names, so the reported indexes are then
// returned as Gravitino sent them.
func mergeTableIndexes(ctx context.Context, server []models.Index, desired []models.IndexTFSDK, preferServer bool, diags *diag.Diagnostics) []models.IndexTFSDK {
	reported := models.TableIndexesToModel(ctx, server, diags)
	if diags.HasError() {
		return reported
	}
	if len(reported) != len(desired) {
		return reported
	}

	for i := range reported {
		reported[i].Properties = mergeIndexProperties(ctx, server[i].Properties, desired[i].Properties, preferServer, diags)
	}
	return reported
}

// mergeIndexProperties combines the properties Gravitino reports for an index
// with the properties its configuration manages, mirroring mergeTableProperties:
// a catalog may drop or rewrite an index property, and that must not turn into
// permanent drift. The block is immutable, so drift would replace the table on
// every plan.
//
// Only the configured keys are tracked. The keys a catalog reports on its own
// are not surfaced, because the attribute is not computed and the plan then
// holds none. An explicit empty map is kept as an empty map rather than dropped,
// because an empty map is a known planned value and the applied state has to
// match it.

// mergeIndexProperties combines the properties Gravitino reports for an index
// with the properties its configuration manages, mirroring mergeTableProperties:
// a catalog may drop or rewrite an index property, and that must not turn into
// permanent drift. The block is immutable, so drift would replace the table on
// every plan.
//
// Only the configured keys are tracked. The keys a catalog reports on its own
// are not surfaced, because the attribute is not computed and the plan then
// holds none. An explicit empty map is kept as an empty map rather than dropped,
// because an empty map is a known planned value and the applied state has to
// match it.
func mergeIndexProperties(ctx context.Context, serverProperties map[string]string, desired types.Map, preferServer bool, diags *diag.Diagnostics) types.Map {
	if desired.IsNull() || desired.IsUnknown() {
		return types.MapNull(types.StringType)
	}

	managed := make(map[string]string)
	diags.Append(desired.ElementsAs(ctx, &managed, false)...)
	if diags.HasError() || len(managed) == 0 {
		return desired
	}

	merged := make(map[string]string, len(managed))
	for key, value := range managed {
		if serverValue, ok := serverProperties[key]; ok && preferServer {
			merged[key] = serverValue
			continue
		}
		merged[key] = value
	}

	properties, propertyDiags := types.MapValueFrom(ctx, types.StringType, merged)
	diags.Append(propertyDiags...)
	return properties
}

// tablePropertyUpdates diffs the managed properties into setProperty and
// removeProperty updates. The configuration is the complete set of managed
// properties, so a key that is no longer configured is removed.
