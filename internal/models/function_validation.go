package models

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

// FunctionExternalTypeDiagnostics reports every function parameter, return type
// or return column whose data type uses the external variant, naming the server
// version. serverSupportsExternalType is the result of the client version probe:
// nothing is returned when the connected server accepts the variant, or when the
// probe failed and the version is unknown. It mirrors
// ExternalTypeColumnDiagnostics for the function surface.
func FunctionExternalTypeDiagnostics(definitions []FunctionDefinition, serverVersion string, serverSupportsExternalType bool) diag.Diagnostics {
	if serverSupportsExternalType {
		return nil
	}

	var diags diag.Diagnostics

	server := serverVersion
	if server == "" {
		server = "unknown"
	}

	check := func(raw json.RawMessage, attrPath path.Path, label, name string) {
		if len(raw) == 0 {
			return
		}
		dataType, err := ParseDataType(DataTypeToString(raw))
		if err != nil || !dataType.UsesExternalType() {
			return
		}
		subject := label
		if name != "" {
			subject = fmt.Sprintf("%s %q", label, name)
		}
		diags.AddAttributeError(attrPath,
			"External function data type requires Gravitino 1.3.1 or later",
			fmt.Sprintf("The %s uses the external data type, which the connected Gravitino %s server does not accept. "+
				"Upgrade the server to 1.3.1 or later, or use a type Gravitino understands.", subject, server))
	}

	for i, definition := range definitions {
		defPath := path.Root("definitions").AtListIndex(i)
		for j, parameter := range definition.Parameters {
			check(parameter.DataType, defPath.AtName("parameters").AtListIndex(j).AtName("data_type"), "parameter", parameter.Name)
		}
		check(definition.ReturnType, defPath.AtName("return_type"), "return type", "")
		for j, column := range definition.ReturnColumns {
			check(column.DataType, defPath.AtName("return_columns").AtListIndex(j).AtName("data_type"), "return column", column.Name)
		}
	}

	return diags
}

// NormalizeFunctionType returns a function type in the spelling Terraform uses.
// Gravitino documents and accepts SCALAR/AGGREGATE/TABLE on the request, but its
// responses spell the type in lower case ("scalar", "table").
func NormalizeFunctionType(functionType string) string {
	return strings.ToUpper(functionType)
}
