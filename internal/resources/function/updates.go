package function

import (
	"context"
	"sort"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Gravitino supports exactly six function update types (the FunctionUpdateRequest
// discriminator in docs/open-api/functions.yaml): updateComment, addDefinition,
// removeDefinition, addImpl, updateImpl and removeImpl. Every other attribute of a
// function is immutable and forces replacement in the schema.

// buildFunctionUpdates returns the update requests that turn state into plan.
func buildFunctionUpdates(ctx context.Context, plan, state FunctionResourceModel) ([]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	var updates []any

	planComment := commentValue(plan.Comment)
	stateComment := commentValue(state.Comment)
	if !optionalStringEqual(planComment, stateComment) {
		// newComment is nullable, which is how Gravitino clears a comment.
		updates = append(updates, models.NewUpdateFunctionCommentRequest(planComment))
	}

	planDefinitions, d := models.FunctionDefinitionsFromTF(ctx, plan.Definitions)
	diags.Append(d...)
	stateDefinitions, d := models.FunctionDefinitionsFromTF(ctx, state.Definitions)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	return append(updates, definitionUpdates(planDefinitions, stateDefinitions)...), diags
}

// definitionUpdates diffs definitions by their parameters, the identity Gravitino
// uses for removeDefinition/addImpl/updateImpl/removeImpl. A definition whose
// parameters are unchanged but whose return type or return columns changed is
// replaced, because the API has no "update definition" request.
func definitionUpdates(plan, state []models.FunctionDefinition) []any {
	var updates []any

	planBySignature := indexDefinitions(plan)
	stateBySignature := indexDefinitions(state)

	for _, stateDefinition := range state {
		planDefinition, ok := planBySignature[stateDefinition.Signature()]
		if !ok || planDefinition.Shape() != stateDefinition.Shape() {
			updates = append(updates, models.NewRemoveFunctionDefinitionRequest(stateDefinition.Parameters))
		}
	}

	for _, planDefinition := range plan {
		stateDefinition, ok := stateBySignature[planDefinition.Signature()]
		if !ok || stateDefinition.Shape() != planDefinition.Shape() {
			updates = append(updates, models.NewAddFunctionDefinitionRequest(planDefinition))
			continue
		}
		updates = append(updates, implementationUpdates(planDefinition, stateDefinition)...)
	}

	return updates
}

// implementationUpdates diffs the implementations of one definition. They are
// matched by runtime and language (see models.FunctionImplKey); the requests carry
// the runtime, which is what addImpl/updateImpl/removeImpl identify.
func implementationUpdates(planDefinition, stateDefinition models.FunctionDefinition) []any {
	var updates []any

	planImpls := indexImpls(planDefinition.Impls)
	stateImpls := indexImpls(stateDefinition.Impls)

	for _, key := range sortedImplKeys(stateImpls) {
		stateImpl := stateImpls[key]
		planImpl, ok := planImpls[key]
		if !ok {
			updates = append(updates, models.NewRemoveFunctionImplRequest(planDefinition.Parameters, stateImpl.Runtime))
			continue
		}
		if !models.FunctionImplEqual(planImpl, stateImpl) {
			updates = append(updates, models.NewUpdateFunctionImplRequest(planDefinition.Parameters, planImpl.Runtime, planImpl))
		}
	}

	for _, key := range sortedImplKeys(planImpls) {
		if _, ok := stateImpls[key]; !ok {
			updates = append(updates, models.NewAddFunctionImplRequest(planDefinition.Parameters, planImpls[key]))
		}
	}

	return updates
}

func indexDefinitions(definitions []models.FunctionDefinition) map[string]models.FunctionDefinition {
	indexed := make(map[string]models.FunctionDefinition, len(definitions))
	for _, definition := range definitions {
		signature := definition.Signature()
		if _, ok := indexed[signature]; !ok {
			indexed[signature] = definition
		}
	}
	return indexed
}

func indexImpls(impls []models.FunctionImpl) map[string]models.FunctionImpl {
	indexed := make(map[string]models.FunctionImpl, len(impls))
	for _, impl := range impls {
		key := models.FunctionImplKey(impl)
		if _, ok := indexed[key]; !ok {
			indexed[key] = impl
		}
	}
	return indexed
}

func sortedImplKeys(impls map[string]models.FunctionImpl) []string {
	keys := make([]string, 0, len(impls))
	for key := range impls {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// commentValue normalises an optional comment: Terraform null and the empty string
// both mean "no comment" (Gravitino stores no comment for an empty value), which
// is sent as newComment: null.
func commentValue(value types.String) *string {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return nil
	}
	comment := value.ValueString()
	return &comment
}

func optionalStringEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
