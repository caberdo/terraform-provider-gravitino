package function

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// functionDefinitionsAttribute is the schema of the function definitions, shared
// by the gravitino_function and gravitino_functions data sources so the two
// surfaces cannot drift apart.
func functionDefinitionsAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "The definitions of the function, including their implementations.",
		Computed:    true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"parameters": schema.ListNestedAttribute{
					Description: "The parameters of the definition.",
					Computed:    true,
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Description: "The name of the parameter.",
								Computed:    true,
							},
							"data_type": schema.StringAttribute{
								Description: "The Gravitino data type of the parameter.",
								Computed:    true,
							},
							"comment": schema.StringAttribute{
								Description: "The comment of the parameter.",
								Computed:    true,
							},
							"default_value": schema.StringAttribute{
								Description: "The default value expression of the parameter.",
								Computed:    true,
							},
						},
					},
				},
				"return_type": schema.StringAttribute{
					Description: "The return type of the definition (SCALAR and AGGREGATE functions).",
					Computed:    true,
				},
				"return_columns": schema.ListNestedAttribute{
					Description: "The return columns of the definition (TABLE functions).",
					Computed:    true,
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Description: "The name of the return column.",
								Computed:    true,
							},
							"data_type": schema.StringAttribute{
								Description: "The Gravitino data type of the return column.",
								Computed:    true,
							},
							"comment": schema.StringAttribute{
								Description: "The comment of the return column.",
								Computed:    true,
							},
						},
					},
				},
				"impls": schema.ListNestedAttribute{
					Description: "The implementations of the definition.",
					Computed:    true,
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"language": schema.StringAttribute{
								Description: "The implementation language (SQL, JAVA or PYTHON).",
								Computed:    true,
							},
							"runtime": schema.StringAttribute{
								Description: "The runtime of the implementation (SPARK or TRINO).",
								Computed:    true,
							},
							"sql": schema.StringAttribute{
								Description: "The SQL expression of a SQL implementation.",
								Computed:    true,
							},
							"class_name": schema.StringAttribute{
								Description: "The class name of a JAVA implementation.",
								Computed:    true,
							},
							"handler": schema.StringAttribute{
								Description: "The handler of a PYTHON implementation.",
								Computed:    true,
							},
							"code_block": schema.StringAttribute{
								Description: "The code block of a PYTHON implementation.",
								Computed:    true,
							},
							"resources": schema.SingleNestedAttribute{
								Description: "External resources required by the implementation.",
								Computed:    true,
								Attributes: map[string]schema.Attribute{
									"jars": schema.ListAttribute{
										Description: "JAR file URIs.",
										Computed:    true,
										ElementType: types.StringType,
									},
									"files": schema.ListAttribute{
										Description: "File URIs.",
										Computed:    true,
										ElementType: types.StringType,
									},
									"archives": schema.ListAttribute{
										Description: "Archive URIs.",
										Computed:    true,
										ElementType: types.StringType,
									},
								},
							},
							"properties": schema.MapAttribute{
								Description: "Additional properties of the implementation.",
								Computed:    true,
								ElementType: types.StringType,
							},
						},
					},
				},
			},
		},
	}
}
