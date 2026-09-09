package metalake

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func propertiesToMapDS(ctx context.Context, props map[string]string, diags *diag.Diagnostics) types.Map {
	if len(props) == 0 {
		return types.MapNull(types.StringType)
	}
	result, d := types.MapValueFrom(ctx, types.StringType, props)
	if d.HasError() {
		*diags = append(*diags, d...)
		return types.MapNull(types.StringType)
	}
	return result
}
