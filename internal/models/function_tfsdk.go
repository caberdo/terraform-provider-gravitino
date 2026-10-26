package models

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// ---------------------------------------------------------------------------
// Terraform models
// ---------------------------------------------------------------------------

// FunctionDefinitionTFSDK mirrors FunctionDefinition (and its nested types) as
// exposed to Terraform.
type FunctionDefinitionTFSDK struct {
	Parameters    types.List   `tfsdk:"parameters"`
	ReturnType    types.String `tfsdk:"return_type"`
	ReturnColumns types.List   `tfsdk:"return_columns"`
	Impls         types.List   `tfsdk:"impls"`
}

// FunctionParamTFSDK mirrors FunctionParam.
type FunctionParamTFSDK struct {
	Name         types.String `tfsdk:"name"`
	DataType     types.String `tfsdk:"data_type"`
	Comment      types.String `tfsdk:"comment"`
	DefaultValue types.String `tfsdk:"default_value"`
}

// FunctionColumnTFSDK mirrors FunctionColumn.
type FunctionColumnTFSDK struct {
	Name     types.String `tfsdk:"name"`
	DataType types.String `tfsdk:"data_type"`
	Comment  types.String `tfsdk:"comment"`
}

// FunctionImplTFSDK mirrors FunctionImpl.
type FunctionImplTFSDK struct {
	Language   types.String `tfsdk:"language"`
	Runtime    types.String `tfsdk:"runtime"`
	SQL        types.String `tfsdk:"sql"`
	ClassName  types.String `tfsdk:"class_name"`
	Handler    types.String `tfsdk:"handler"`
	CodeBlock  types.String `tfsdk:"code_block"`
	Resources  types.Object `tfsdk:"resources"`
	Properties types.Map    `tfsdk:"properties"`
}

// FunctionResourcesTFSDK mirrors FunctionResources.
type FunctionResourcesTFSDK struct {
	Jars     types.List `tfsdk:"jars"`
	Files    types.List `tfsdk:"files"`
	Archives types.List `tfsdk:"archives"`
}

// FunctionResourcesAttrTypes returns the Terraform attribute types of
// FunctionResourcesTFSDK.
func FunctionResourcesAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"jars":     types.ListType{ElemType: types.StringType},
		"files":    types.ListType{ElemType: types.StringType},
		"archives": types.ListType{ElemType: types.StringType},
	}
}

// FunctionImplAttrTypes returns the Terraform attribute types of
// FunctionImplTFSDK.
func FunctionImplAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"language":   types.StringType,
		"runtime":    types.StringType,
		"sql":        types.StringType,
		"class_name": types.StringType,
		"handler":    types.StringType,
		"code_block": types.StringType,
		"resources":  types.ObjectType{AttrTypes: FunctionResourcesAttrTypes()},
		"properties": types.MapType{ElemType: types.StringType},
	}
}

// FunctionParamAttrTypes returns the Terraform attribute types of
// FunctionParamTFSDK.
func FunctionParamAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":          types.StringType,
		"data_type":     types.StringType,
		"comment":       types.StringType,
		"default_value": types.StringType,
	}
}

// FunctionColumnAttrTypes returns the Terraform attribute types of
// FunctionColumnTFSDK.
func FunctionColumnAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":      types.StringType,
		"data_type": types.StringType,
		"comment":   types.StringType,
	}
}

// FunctionDefinitionAttrTypes returns the Terraform attribute types of
// FunctionDefinitionTFSDK.
func FunctionDefinitionAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"parameters":     types.ListType{ElemType: types.ObjectType{AttrTypes: FunctionParamAttrTypes()}},
		"return_type":    types.StringType,
		"return_columns": types.ListType{ElemType: types.ObjectType{AttrTypes: FunctionColumnAttrTypes()}},
		"impls":          types.ListType{ElemType: types.ObjectType{AttrTypes: FunctionImplAttrTypes()}},
	}
}

// FunctionDefinitionsListType is the Terraform type of a definitions list.
func FunctionDefinitionsListType() attr.Type {
	return types.ListType{ElemType: FunctionDefinitionObjectType()}
}

// FunctionDefinitionObjectType is the Terraform object type of a definition, the
// element type of a definitions list.
func FunctionDefinitionObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: FunctionDefinitionAttrTypes()}
}

// FunctionDefinitionsFromTF converts the Terraform definitions list into API
// definitions. It is used to build register and update requests.
func FunctionDefinitionsFromTF(ctx context.Context, list types.List) ([]FunctionDefinition, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return nil, diags
	}

	var tfDefinitions []FunctionDefinitionTFSDK
	diags.Append(list.ElementsAs(ctx, &tfDefinitions, false)...)
	if diags.HasError() {
		return nil, diags
	}

	definitions := make([]FunctionDefinition, 0, len(tfDefinitions))
	for i, tfDefinition := range tfDefinitions {
		definition, d := FunctionDefinitionFromTF(ctx, tfDefinition, fmt.Sprintf("definitions[%d]", i))
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		definitions = append(definitions, definition)
	}

	return definitions, diags
}

// FunctionDefinitionFromTF converts a single Terraform definition into the API
// model. path prefixes diagnostics so the failing attribute is identifiable.
func FunctionDefinitionFromTF(ctx context.Context, in FunctionDefinitionTFSDK, path string) (FunctionDefinition, diag.Diagnostics) {
	var diags diag.Diagnostics
	var definition FunctionDefinition

	if !in.Parameters.IsNull() && !in.Parameters.IsUnknown() {
		var tfParams []FunctionParamTFSDK
		diags.Append(in.Parameters.ElementsAs(ctx, &tfParams, false)...)
		if diags.HasError() {
			return definition, diags
		}

		parameters := make([]FunctionParam, 0, len(tfParams))
		for i, tfParam := range tfParams {
			parameterPath := fmt.Sprintf("%s.parameters[%d]", path, i)
			dataType, err := DataTypeFromString(tfParam.DataType.ValueString())
			if err != nil {
				diags.AddError("Invalid function data type",
					fmt.Sprintf("%s.data_type: %s.", parameterPath, err))
				return definition, diags
			}

			parameter := FunctionParam{
				Name:     tfParam.Name.ValueString(),
				DataType: dataType,
				Comment:  tfParam.Comment.ValueString(),
			}
			if !tfParam.DefaultValue.IsNull() && !tfParam.DefaultValue.IsUnknown() && tfParam.DefaultValue.ValueString() != "" {
				defaultValue, err := RawJSONFromString(tfParam.DefaultValue.ValueString())
				if err != nil {
					diags.AddError("Invalid default value",
						fmt.Sprintf("%s.default_value: %s (use jsonencode() for an argument document).", parameterPath, err))
					return definition, diags
				}
				parameter.DefaultValue = defaultValue
			}
			parameters = append(parameters, parameter)
		}
		definition.Parameters = parameters
	}

	if !in.ReturnType.IsNull() && !in.ReturnType.IsUnknown() && in.ReturnType.ValueString() != "" {
		returnType, err := DataTypeFromString(in.ReturnType.ValueString())
		if err != nil {
			diags.AddError("Invalid function return type", fmt.Sprintf("%s.return_type: %s.", path, err))
			return definition, diags
		}
		definition.ReturnType = returnType
	}

	if !in.ReturnColumns.IsNull() && !in.ReturnColumns.IsUnknown() {
		var tfColumns []FunctionColumnTFSDK
		diags.Append(in.ReturnColumns.ElementsAs(ctx, &tfColumns, false)...)
		if diags.HasError() {
			return definition, diags
		}

		columns := make([]FunctionColumn, 0, len(tfColumns))
		for i, tfColumn := range tfColumns {
			dataType, err := DataTypeFromString(tfColumn.DataType.ValueString())
			if err != nil {
				diags.AddError("Invalid return column data type",
					fmt.Sprintf("%s.return_columns[%d].data_type: %s.", path, i, err))
				return definition, diags
			}
			columns = append(columns, FunctionColumn{
				Name:     tfColumn.Name.ValueString(),
				DataType: dataType,
				Comment:  tfColumn.Comment.ValueString(),
			})
		}
		definition.ReturnColumns = columns
	}

	if !in.Impls.IsNull() && !in.Impls.IsUnknown() {
		var tfImpls []FunctionImplTFSDK
		diags.Append(in.Impls.ElementsAs(ctx, &tfImpls, false)...)
		if diags.HasError() {
			return definition, diags
		}

		impls := make([]FunctionImpl, 0, len(tfImpls))
		runtimes := make(map[string]bool, len(tfImpls))
		for i, tfImpl := range tfImpls {
			impl, d := FunctionImplFromTF(ctx, tfImpl, fmt.Sprintf("%s.impls[%d]", path, i))
			diags.Append(d...)
			if diags.HasError() {
				return definition, diags
			}
			// The server rejects this itself: "Cannot register function:
			// definition at index 0 has duplicate runtime 'SPARK'. Each definition
			// must have at most one implementation per runtime."
			if runtimes[impl.Runtime] {
				diags.AddError("Invalid function implementation",
					fmt.Sprintf("%s.impls[%d].runtime: a definition can hold at most one implementation per runtime, %q is used more than once.", path, i, impl.Runtime))
				return definition, diags
			}
			runtimes[impl.Runtime] = true
			impls = append(impls, impl)
		}
		definition.Impls = impls
	}

	return definition, diags
}

// FunctionImplFromTF converts a Terraform implementation into the API model.
func FunctionImplFromTF(ctx context.Context, in FunctionImplTFSDK, path string) (FunctionImpl, diag.Diagnostics) {
	var diags diag.Diagnostics
	impl := FunctionImpl{
		Language:  in.Language.ValueString(),
		Runtime:   in.Runtime.ValueString(),
		SQL:       in.SQL.ValueString(),
		ClassName: in.ClassName.ValueString(),
		Handler:   in.Handler.ValueString(),
		CodeBlock: in.CodeBlock.ValueString(),
	}

	if !in.Properties.IsNull() && !in.Properties.IsUnknown() {
		properties := make(map[string]string)
		diags.Append(in.Properties.ElementsAs(ctx, &properties, false)...)
		if diags.HasError() {
			return impl, diags
		}
		impl.Properties = properties
	}

	if !in.Resources.IsNull() && !in.Resources.IsUnknown() {
		var tfResources FunctionResourcesTFSDK
		diags.Append(in.Resources.As(ctx, &tfResources, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			return impl, diags
		}

		resources := &FunctionResources{
			Jars:     stringListFromTF(ctx, tfResources.Jars, &diags),
			Files:    stringListFromTF(ctx, tfResources.Files, &diags),
			Archives: stringListFromTF(ctx, tfResources.Archives, &diags),
		}
		if diags.HasError() {
			return impl, diags
		}
		if len(resources.Jars) > 0 || len(resources.Files) > 0 || len(resources.Archives) > 0 {
			impl.Resources = resources
		}
	}

	// The API discriminates implementations by `language` and requires `runtime`
	// on every implementation plus the language specific field (SQLImpl requires
	// `sql`, JavaImpl requires `className`; PythonImpl requires neither handler nor
	// codeBlock). Failing here yields a readable error instead of the server's
	// "Malformed json request".
	if impl.Runtime == "" {
		diags.AddError("Invalid function implementation",
			fmt.Sprintf("%s.runtime: every implementation requires a runtime (%s).", path, strings.Join(AllFunctionRuntimes, ", ")))
	}
	switch impl.Language {
	case FunctionLanguageSQL:
		if impl.SQL == "" {
			diags.AddError("Invalid SQL implementation",
				fmt.Sprintf("%s.sql: a SQL implementation requires the sql expression.", path))
		}
	case FunctionLanguageJava:
		if impl.ClassName == "" {
			diags.AddError("Invalid JAVA implementation",
				fmt.Sprintf("%s.class_name: a JAVA implementation requires the fully qualified class name.", path))
		}
	case FunctionLanguagePython:
		// handler and codeBlock are optional in the API.
	default:
		diags.AddError("Invalid function implementation",
			fmt.Sprintf("%s.language: must be one of %s.", path, strings.Join(AllFunctionLanguages, ", ")))
	}
	if diags.HasError() {
		return impl, diags
	}

	return impl, diags
}

// FunctionDefinitionsToTF converts API definitions into the Terraform list value of
// a data source: the server decides the content, the order and the spelling.
func FunctionDefinitionsToTF(ctx context.Context, definitions []FunctionDefinition) (types.List, diag.Diagnostics) {
	return mapFunctionDefinitions(ctx, definitions, nil, mappingFromServer)
}

// FunctionDefinitionsToTFRefreshed converts API definitions into the state of a
// refresh. previous holds the definitions of the state that is being refreshed: the
// server stays the source of truth (a definition it dropped disappears from state, a
// definition it added is included), but a definition the server echoed keeps the
// spelling of the state, so a jsonencode() data type document does not show up as a
// whitespace only diff on every plan.
func FunctionDefinitionsToTFRefreshed(ctx context.Context, definitions, previous []FunctionDefinition) (types.List, diag.Diagnostics) {
	return mapFunctionDefinitions(ctx, definitions, previous, mappingFromServer)
}

// FunctionDefinitionsToTFApplied converts API definitions into the state written
// after an apply. previous is the configuration that was applied: the order, the
// spelling and the set of definitions follow the plan, so that a server side extra
// cannot make apply fail with "provider produced inconsistent result".
func FunctionDefinitionsToTFApplied(ctx context.Context, definitions, previous []FunctionDefinition) (types.List, diag.Diagnostics) {
	return mapFunctionDefinitions(ctx, definitions, previous, mappingFromPlan)
}

// mappingMode selects which side is the reference while converting definitions.
type mappingMode bool

const (
	// mappingFromServer honours the server: unknown definitions are included and a
	// previous definition the server no longer returns is dropped. Only the spelling
	// of equivalent data type documents is taken from previous.
	mappingFromServer mappingMode = false
	// mappingFromPlan honours a plan: only the definitions of previous are written,
	// and a definition the server did not return falls back to the planned value.
	mappingFromPlan mappingMode = true
)

func mapFunctionDefinitions(ctx context.Context, definitions, previous []FunctionDefinition, mode mappingMode) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elementType := FunctionDefinitionObjectType()

	items := make([]attr.Value, 0, len(definitions)+len(previous))
	converted := make(map[string]bool, len(definitions))

	bySignature := make(map[string]FunctionDefinition, len(definitions))
	for _, definition := range definitions {
		if _, ok := bySignature[definition.Signature()]; !ok {
			bySignature[definition.Signature()] = definition
		}
	}

	for i := range previous {
		previousDefinition := previous[i]
		signature := previousDefinition.Signature()

		serverDefinition, ok := bySignature[signature]
		if !ok {
			if mode == mappingFromPlan {
				// The server did not return a definition that was just applied: keep
				// the configured value, the next refresh reports the real state.
				serverDefinition = previousDefinition
			} else {
				continue
			}
		} else if functionDefinitionEqual(previousDefinition, serverDefinition) {
			serverDefinition = previousDefinition
		}

		item, d := functionDefinitionToObject(ctx, serverDefinition, &previousDefinition, mode)
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(elementType), diags
		}
		items = append(items, item)
		converted[signature] = true
	}

	if mode == mappingFromServer {
		for _, definition := range definitions {
			if converted[definition.Signature()] {
				continue
			}
			item, d := functionDefinitionToObject(ctx, definition, nil, mode)
			diags.Append(d...)
			if diags.HasError() {
				return types.ListNull(elementType), diags
			}
			items = append(items, item)
			converted[definition.Signature()] = true
		}
	}

	if len(items) == 0 {
		return types.ListNull(elementType), diags
	}

	list, d := types.ListValue(elementType, items)
	diags.Append(d...)
	return list, diags
}

// functionDefinitionEqual reports whether the server returned exactly the
// definition that was configured.
func functionDefinitionEqual(a, b FunctionDefinition) bool {
	aJSON, errA := json.Marshal(a)
	bJSON, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}

func functionDefinitionToObject(ctx context.Context, definition FunctionDefinition, previous *FunctionDefinition, mode mappingMode) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	parameters, d := functionParamsToTF(ctx, definition.Parameters, previousParameters(previous))
	diags.Append(d...)

	returnColumns, d := functionColumnsToTF(ctx, definition.ReturnColumns, previousReturnColumns(previous))
	diags.Append(d...)

	impls, d := functionImplsToTF(ctx, definition.Impls, previousImpls(previous), mode)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(FunctionDefinitionAttrTypes()), diags
	}

	returnType := types.StringNull()
	if len(definition.ReturnType) > 0 {
		raw := definition.ReturnType
		if previous != nil {
			raw = preferRaw(previous.ReturnType, raw)
		}
		returnType = types.StringValue(DataTypeToString(raw))
	}

	object, d := types.ObjectValueFrom(ctx, FunctionDefinitionAttrTypes(), FunctionDefinitionTFSDK{
		Parameters:    parameters,
		ReturnType:    returnType,
		ReturnColumns: returnColumns,
		Impls:         impls,
	})
	diags.Append(d...)
	return object, diags
}

func previousParameters(previous *FunctionDefinition) map[string]FunctionParam {
	params := make(map[string]FunctionParam)
	if previous == nil {
		return params
	}
	for _, parameter := range previous.Parameters {
		params[parameter.Name] = parameter
	}
	return params
}

func previousReturnColumns(previous *FunctionDefinition) map[string]FunctionColumn {
	columns := make(map[string]FunctionColumn)
	if previous == nil {
		return columns
	}
	for _, column := range previous.ReturnColumns {
		columns[column.Name] = column
	}
	return columns
}

func previousImpls(previous *FunctionDefinition) []FunctionImpl {
	if previous == nil {
		return nil
	}
	return previous.Impls
}

func functionParamsToTF(ctx context.Context, parameters []FunctionParam, previous map[string]FunctionParam) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elementType := types.ObjectType{AttrTypes: FunctionParamAttrTypes()}
	if len(parameters) == 0 {
		return types.ListNull(elementType), diags
	}

	items := make([]attr.Value, 0, len(parameters))
	for _, parameter := range parameters {
		dataType := parameter.DataType
		defaultValue := parameter.DefaultValue
		if prior, ok := previous[parameter.Name]; ok {
			dataType = preferRaw(prior.DataType, dataType)
			defaultValue = preferRaw(prior.DefaultValue, defaultValue)
		}

		defaultValueValue := types.StringNull()
		if len(defaultValue) > 0 {
			defaultValueValue = types.StringValue(DataTypeToString(defaultValue))
		}

		object, d := types.ObjectValueFrom(ctx, FunctionParamAttrTypes(), FunctionParamTFSDK{
			Name:         types.StringValue(parameter.Name),
			DataType:     types.StringValue(DataTypeToString(dataType)),
			Comment:      stringValueOrNull(parameter.Comment),
			DefaultValue: defaultValueValue,
		})
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(elementType), diags
		}
		items = append(items, object)
	}

	list, d := types.ListValue(elementType, items)
	diags.Append(d...)
	return list, diags
}

func functionColumnsToTF(ctx context.Context, columns []FunctionColumn, previous map[string]FunctionColumn) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elementType := types.ObjectType{AttrTypes: FunctionColumnAttrTypes()}
	if len(columns) == 0 {
		return types.ListNull(elementType), diags
	}

	items := make([]attr.Value, 0, len(columns))
	for _, column := range columns {
		dataType := column.DataType
		if prior, ok := previous[column.Name]; ok {
			dataType = preferRaw(prior.DataType, dataType)
		}

		object, d := types.ObjectValueFrom(ctx, FunctionColumnAttrTypes(), FunctionColumnTFSDK{
			Name:     types.StringValue(column.Name),
			DataType: types.StringValue(DataTypeToString(dataType)),
			Comment:  stringValueOrNull(column.Comment),
		})
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(elementType), diags
		}
		items = append(items, object)
	}

	list, d := types.ListValue(elementType, items)
	diags.Append(d...)
	return list, diags
}

// functionImplsToTF converts the implementations of a definition. When previous is
// set (apply), implementations are returned in the configured order, an
// implementation the server returned unchanged keeps the configured values, and
// implementations the configuration does not declare are left out.
func functionImplsToTF(ctx context.Context, impls, previous []FunctionImpl, mode mappingMode) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elementType := types.ObjectType{AttrTypes: FunctionImplAttrTypes()}

	ordered := impls
	if len(previous) > 0 {
		serverImpls := make(map[string]FunctionImpl, len(impls))
		for _, impl := range impls {
			serverImpls[FunctionImplKey(impl)] = impl
		}

		ordered = make([]FunctionImpl, 0, len(previous)+len(impls))
		converted := make(map[string]bool, len(impls))
		for _, priorImpl := range previous {
			key := FunctionImplKey(priorImpl)
			serverImpl, ok := serverImpls[key]
			if !ok {
				if mode == mappingFromPlan {
					// The server did not return the implementation, keep the
					// configured value rather than failing the apply.
					ordered = append(ordered, priorImpl)
				}
				continue
			}
			if FunctionImplEqual(priorImpl, serverImpl) {
				serverImpl = priorImpl
			}
			ordered = append(ordered, serverImpl)
			converted[key] = true
		}

		if mode == mappingFromServer {
			for _, impl := range impls {
				key := FunctionImplKey(impl)
				if converted[key] {
					continue
				}
				ordered = append(ordered, impl)
				converted[key] = true
			}
		}
	}

	if len(ordered) == 0 {
		return types.ListNull(elementType), diags
	}

	items := make([]attr.Value, 0, len(ordered))
	for _, impl := range ordered {
		object, d := functionImplToObject(ctx, impl)
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(elementType), diags
		}
		items = append(items, object)
	}

	list, d := types.ListValue(elementType, items)
	diags.Append(d...)
	return list, diags
}

// FunctionImplEqual reports whether two implementations are identical on the wire.
func FunctionImplEqual(a, b FunctionImpl) bool {
	aJSON, errA := json.Marshal(a)
	bJSON, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}

// FunctionImplKey identifies an implementation within a definition by its runtime.
// addImpl/updateImpl/removeImpl identify an implementation by `parameters` plus
// `runtime`, and the server rejects a definition with two implementations on the
// same runtime ("Each definition must have at most one implementation per
// runtime"), so the runtime is the identity of an implementation.
func FunctionImplKey(impl FunctionImpl) string {
	return impl.Runtime
}

func functionImplToObject(ctx context.Context, impl FunctionImpl) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	properties := types.MapNull(types.StringType)
	if len(impl.Properties) > 0 {
		value, d := types.MapValueFrom(ctx, types.StringType, impl.Properties)
		diags.Append(d...)
		properties = value
	}

	resources := types.ObjectNull(FunctionResourcesAttrTypes())
	// Gravitino always answers with a resources object, even when none was sent
	// ({"jars":[],"files":[],"archives":[]}); an empty object means "no resources".
	if impl.Resources != nil && (len(impl.Resources.Jars) > 0 || len(impl.Resources.Files) > 0 || len(impl.Resources.Archives) > 0) {
		value, d := types.ObjectValueFrom(ctx, FunctionResourcesAttrTypes(), FunctionResourcesTFSDK{
			Jars:     stringListToTF(ctx, impl.Resources.Jars, &diags),
			Files:    stringListToTF(ctx, impl.Resources.Files, &diags),
			Archives: stringListToTF(ctx, impl.Resources.Archives, &diags),
		})
		diags.Append(d...)
		resources = value
	}
	if diags.HasError() {
		return types.ObjectNull(FunctionImplAttrTypes()), diags
	}

	return types.ObjectValueFrom(ctx, FunctionImplAttrTypes(), FunctionImplTFSDK{
		Language:   types.StringValue(impl.Language),
		Runtime:    types.StringValue(impl.Runtime),
		SQL:        stringValueOrNull(impl.SQL),
		ClassName:  stringValueOrNull(impl.ClassName),
		Handler:    stringValueOrNull(impl.Handler),
		CodeBlock:  stringValueOrNull(impl.CodeBlock),
		Resources:  resources,
		Properties: properties,
	})
}

func stringValueOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func stringListToTF(ctx context.Context, values []string, diags *diag.Diagnostics) types.List {
	if len(values) == 0 {
		return types.ListNull(types.StringType)
	}
	list, d := types.ListValueFrom(ctx, types.StringType, values)
	diags.Append(d...)
	return list
}

func stringListFromTF(ctx context.Context, list types.List, diags *diag.Diagnostics) []string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}

	var values []string
	diags.Append(list.ElementsAs(ctx, &values, false)...)
	if diags.HasError() {
		return nil
	}
	return values
}
