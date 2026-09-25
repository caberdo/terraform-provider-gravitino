package job_template

import (
	"context"
	"fmt"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &JobTemplateResource{}
var _ resource.ResourceWithImportState = &JobTemplateResource{}
var _ resource.ResourceWithConfigure = &JobTemplateResource{}
var _ resource.ResourceWithValidateConfig = &JobTemplateResource{}
var _ resource.ResourceWithModifyPlan = &JobTemplateResource{}

// jobTypes are the two job template variants Gravitino supports. They are
// distinct object types on the wire (ShellJobTemplate/SparkJobTemplate), which is
// why job_type cannot be changed in place.
var jobTypes = []string{"shell", "spark"}

type JobTemplateResource struct {
	client *client.Client
}

func NewJobTemplateResource() resource.Resource {
	return &JobTemplateResource{}
}

func (r *JobTemplateResource) SetClient(c *client.Client) {
	r.client = c
}

type JobTemplateResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Metalake     types.String `tfsdk:"metalake"`
	Name         types.String `tfsdk:"name"`
	JobType      types.String `tfsdk:"job_type"`
	Comment      types.String `tfsdk:"comment"`
	Executable   types.String `tfsdk:"executable"`
	Arguments    types.List   `tfsdk:"arguments"`
	Environments types.Map    `tfsdk:"environments"`
	CustomFields types.Map    `tfsdk:"custom_fields"`
	Scripts      types.List   `tfsdk:"scripts"`
	ClassName    types.String `tfsdk:"class_name"`
	Jars         types.List   `tfsdk:"jars"`
	Files        types.List   `tfsdk:"files"`
	Archives     types.List   `tfsdk:"archives"`
	Configs      types.Map    `tfsdk:"configs"`
	Audit        types.Object `tfsdk:"audit"`
}

var AuditAttrTypes = map[string]attr.Type{
	"creator":            types.StringType,
	"create_time":        types.StringType,
	"last_modifier":      types.StringType,
	"last_modified_time": types.StringType,
}

func (r *JobTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *JobTemplateResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "gravitino_job_template"
}

func (r *JobTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Gravitino job template. Job templates come in two variants selected by " +
			"job_type: shell uses scripts, spark uses class_name/jars/files/archives/configs. " +
			"Gravitino refuses to delete a template that still has active job runs (409 InUseException), " +
			"so destroy the gravitino_job resources that use it first.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Composite identifier in the format 'metalake.job_template_name'.",
			},
			"metalake": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Description: "The metalake name.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The job template name.",
			},
			"job_type": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(jobTypes...),
				},
				Description: fmt.Sprintf("The job template type. One of: %s.", strings.Join(jobTypes, ", ")),
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Description: "A comment about the job template.",
			},
			"executable": schema.StringAttribute{
				Required:    true,
				Description: "The executable command of the job template.",
			},
			"arguments": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "The arguments of the job template.",
			},
			"environments": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Environment variables for the job template.",
			},
			"custom_fields": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Custom fields for the job template.",
			},
			"scripts": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "The scripts of the job template. Only valid for job_type = \"shell\".",
			},
			"class_name": schema.StringAttribute{
				Optional:    true,
				Description: "The main class of the spark job. Only valid for job_type = \"spark\".",
			},
			"jars": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "The jars of the spark job. Only valid for job_type = \"spark\".",
			},
			"files": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "The files of the spark job. Only valid for job_type = \"spark\".",
			},
			"archives": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "The archives of the spark job. Only valid for job_type = \"spark\".",
			},
			"configs": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "The spark configurations. Only valid for job_type = \"spark\".",
			},
			"audit": schema.ObjectAttribute{
				Computed:       true,
				AttributeTypes: AuditAttrTypes,
				// No UseStateForUnknown: the server rewrites audit on every update.
				Description: "Audit information for the job template.",
			},
		},
	}
}

// ModifyPlan marks the composite id as unknown when the template is renamed: the id is
// derived from the name, so reusing the prior state value (UseStateForUnknown) would
// conflict with the id the provider writes after a successful rename.
func (r *JobTemplateResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	var plan, state JobTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("id"), types.StringUnknown())...)
	}
}

// ValidateConfig rejects spark-only attributes on shell templates and vice versa,
// because the API models them as two distinct object types and would otherwise
// fail with an opaque deserialization error.
func (r *JobTemplateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config JobTemplateResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.JobType.IsNull() || config.JobType.IsUnknown() {
		return
	}

	shellOnly := map[string]types.List{"scripts": config.Scripts}
	sparkOnly := map[string]types.List{
		"jars": config.Jars, "files": config.Files, "archives": config.Archives,
	}
	sparkOnlyStr := map[string]types.String{"class_name": config.ClassName}
	sparkOnlyMap := map[string]types.Map{"configs": config.Configs}

	switch config.JobType.ValueString() {
	case "shell":
		for name, v := range sparkOnly {
			if !v.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid attribute for a shell job template",
					fmt.Sprintf("%q is only valid when job_type = \"spark\".", name))
			}
		}
		for name, v := range sparkOnlyStr {
			if !v.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid attribute for a shell job template",
					fmt.Sprintf("%q is only valid when job_type = \"spark\".", name))
			}
		}
		for name, v := range sparkOnlyMap {
			if !v.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid attribute for a shell job template",
					fmt.Sprintf("%q is only valid when job_type = \"spark\".", name))
			}
		}
	case "spark":
		for name, v := range shellOnly {
			if !v.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid attribute for a spark job template",
					fmt.Sprintf("%q is only valid when job_type = \"shell\".", name))
			}
		}
	}
}

func (r *JobTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan JobTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := plan.Metalake.ValueString()
	name := plan.Name.ValueString()

	tflog.Debug(ctx, "Registering job template", map[string]interface{}{"metalake": metalake, "name": name})

	registerReq, d := jobTemplateRegisterRequestFromPlan(ctx, &plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.RegisterJobTemplate(ctx, metalake, registerReq); err != nil {
		resp.Diagnostics.Append(client.NewResourceError("creating job template", name, err)...)
		return
	}

	// Registering answers with a bare BaseResponse, so read the template back to
	// learn the server-assigned audit information.
	result, err := r.client.GetJobTemplate(ctx, metalake, name)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("reading job template after create", name, err)...)
		return
	}

	setStateFromJobTemplate(ctx, &resp.Diagnostics, metalake, &result.JobTemplate, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Registered job template", map[string]interface{}{"metalake": metalake, "name": name})
}

func (r *JobTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state JobTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	name := state.Name.ValueString()

	tflog.Debug(ctx, "Reading job template", map[string]interface{}{"metalake": metalake, "name": name})

	result, err := r.client.GetJobTemplate(ctx, metalake, name)
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("reading job template", name, err)...)
		return
	}

	setStateFromJobTemplate(ctx, &resp.Diagnostics, metalake, &result.JobTemplate, &state)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	tflog.Debug(ctx, "Read job template", map[string]interface{}{"metalake": metalake, "name": name})
}

func (r *JobTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state JobTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	currentName := state.Name.ValueString()

	tflog.Debug(ctx, "Updating job template", map[string]interface{}{"metalake": metalake, "name": currentName})

	var updates []interface{}

	// A rename must be the first update, because every other update targets the
	// template by its current name.
	if !plan.Name.Equal(state.Name) {
		updates = append(updates, models.NewJobTemplateRenameRequest(plan.Name.ValueString()))
	}

	if !plan.Comment.Equal(state.Comment) {
		updates = append(updates, models.NewJobTemplateUpdateCommentRequest(plan.Comment.ValueString()))
	}

	contentUpdate, hasContentChange := jobTemplateContentUpdate(ctx, &plan, &state)
	if hasContentChange {
		updates = append(updates, models.NewJobTemplateUpdateContentRequest(contentUpdate))
	}

	if len(updates) == 0 {
		setStateFromJobTemplate(ctx, &resp.Diagnostics, metalake, nil, &plan)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}

	result, err := r.client.UpdateJobTemplate(ctx, metalake, currentName, updates)
	if err != nil {
		resp.Diagnostics.Append(client.NewResourceError("updating job template", currentName, err)...)
		return
	}

	setStateFromJobTemplate(ctx, &resp.Diagnostics, metalake, &result.JobTemplate, &plan)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	tflog.Debug(ctx, "Updated job template", map[string]interface{}{"metalake": metalake, "name": plan.Name.ValueString()})
}

func (r *JobTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state JobTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	metalake := state.Metalake.ValueString()
	name := state.Name.ValueString()

	tflog.Debug(ctx, "Deleting job template", map[string]interface{}{"metalake": metalake, "name": name})

	if _, err := r.client.DeleteJobTemplate(ctx, metalake, name); err != nil {
		if client.IsNotFoundError(err) {
			// Already gone.
			return
		}
		resp.Diagnostics.Append(client.NewResourceError("deleting job template", name, err)...)
		return
	}

	tflog.Debug(ctx, "Deleted job template", map[string]interface{}{"metalake": metalake, "name": name})
}

func (r *JobTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idx := strings.LastIndex(req.ID, ".")
	if idx == -1 || idx == 0 || idx == len(req.ID)-1 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'metalake.job_template_name', got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("metalake"), req.ID[:idx])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID[idx+1:])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func jobTemplateRegisterRequestFromPlan(ctx context.Context, plan *JobTemplateResourceModel) (*models.JobTemplateRegisterRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	arguments, d := stringListFromTF(ctx, plan.Arguments)
	diags.Append(d...)
	scripts, d := stringListFromTF(ctx, plan.Scripts)
	diags.Append(d...)
	jars, d := stringListFromTF(ctx, plan.Jars)
	diags.Append(d...)
	files, d := stringListFromTF(ctx, plan.Files)
	diags.Append(d...)
	archives, d := stringListFromTF(ctx, plan.Archives)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	template := models.JobTemplate{
		Name:         plan.Name.ValueString(),
		JobType:      plan.JobType.ValueString(),
		Comment:      plan.Comment.ValueString(),
		Executable:   plan.Executable.ValueString(),
		Arguments:    arguments,
		Environments: mapFromTF(plan.Environments),
		CustomFields: mapFromTF(plan.CustomFields),
	}

	if plan.JobType.ValueString() == "shell" {
		template.Scripts = scripts
	} else {
		template.ClassName = plan.ClassName.ValueString()
		template.Jars = jars
		template.Files = files
		template.Archives = archives
		template.Configs = mapFromTF(plan.Configs)
	}

	return &models.JobTemplateRegisterRequest{JobTemplate: template}, diags
}

// jobTemplateContentUpdate returns the updateTemplate payload and whether any
// template content changed. Only changed fields are sent, so untouched settings
// keep their server-side values.
func jobTemplateContentUpdate(ctx context.Context, plan, state *JobTemplateResourceModel) (*models.JobTemplateContentUpdate, bool) {
	update := &models.JobTemplateContentUpdate{Type: plan.JobType.ValueString()}
	changed := false

	if !plan.Executable.Equal(state.Executable) {
		v := plan.Executable.ValueString()
		update.NewExecutable = &v
		changed = true
	}
	if !plan.Arguments.Equal(state.Arguments) {
		v := stringsFromTF(ctx, plan.Arguments)
		update.NewArguments = &v
		changed = true
	}
	if !plan.Environments.Equal(state.Environments) {
		v := mapFromTF(plan.Environments)
		update.NewEnvironments = &v
		changed = true
	}
	if !plan.CustomFields.Equal(state.CustomFields) {
		v := mapFromTF(plan.CustomFields)
		update.NewCustomFields = &v
		changed = true
	}

	if plan.JobType.ValueString() == "shell" {
		if !plan.Scripts.Equal(state.Scripts) {
			v := stringsFromTF(ctx, plan.Scripts)
			update.NewScripts = &v
			changed = true
		}
		return update, changed
	}

	if !plan.ClassName.Equal(state.ClassName) {
		v := plan.ClassName.ValueString()
		update.NewClassName = &v
		changed = true
	}
	if !plan.Jars.Equal(state.Jars) {
		v := stringsFromTF(ctx, plan.Jars)
		update.NewJars = &v
		changed = true
	}
	if !plan.Files.Equal(state.Files) {
		v := stringsFromTF(ctx, plan.Files)
		update.NewFiles = &v
		changed = true
	}
	if !plan.Archives.Equal(state.Archives) {
		v := stringsFromTF(ctx, plan.Archives)
		update.NewArchives = &v
		changed = true
	}
	if !plan.Configs.Equal(state.Configs) {
		v := mapFromTF(plan.Configs)
		update.NewConfigs = &v
		changed = true
	}

	return update, changed
}

// setStateFromJobTemplate writes every schema attribute. With a nil template
// (nothing changed server-side) the plan values are re-established so no
// Computed attribute is left unknown after apply.
func setStateFromJobTemplate(ctx context.Context, diags *diag.Diagnostics, metalake string, template *models.JobTemplate, model *JobTemplateResourceModel) {
	model.Metalake = types.StringValue(metalake)
	model.ID = types.StringValue(metalake + "." + model.Name.ValueString())

	if template == nil {
		return
	}

	model.Name = types.StringValue(template.Name)
	model.JobType = types.StringValue(template.JobType)
	// Optional attributes must fall back to null, not to the empty string: Gravitino
	// omits them entirely (a shell template has no class_name, and a template without a
	// comment returns no comment field), and "" against a null plan fails the apply.
	model.Comment = optionalStringToTF(template.Comment)
	model.Executable = types.StringValue(template.Executable)
	model.ID = types.StringValue(metalake + "." + template.Name)

	arguments, d := stringListToTF(ctx, template.Arguments)
	diags.Append(d...)
	scripts, d := stringListToTF(ctx, template.Scripts)
	diags.Append(d...)
	jars, d := stringListToTF(ctx, template.Jars)
	diags.Append(d...)
	files, d := stringListToTF(ctx, template.Files)
	diags.Append(d...)
	archives, d := stringListToTF(ctx, template.Archives)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Arguments = arguments
	model.Scripts = scripts
	model.Jars = jars
	model.Files = files
	model.Archives = archives

	model.ClassName = optionalStringToTF(template.ClassName)
	model.Environments = mapToTF(template.Environments)
	model.CustomFields = mapToTF(template.CustomFields)
	model.Configs = mapToTF(template.Configs)

	auditObj, d := auditToObjectValue(template.Audit)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	model.Audit = auditObj
}

func auditToObjectValue(audit *models.Audit) (types.Object, diag.Diagnostics) {
	if audit == nil {
		return types.ObjectNull(AuditAttrTypes), nil
	}

	var createTime, lastModifiedTime types.String
	if audit.CreateTime != nil {
		createTime = types.StringValue(audit.CreateTime.Format(timeFormat))
	} else {
		createTime = types.StringNull()
	}
	if audit.LastModifiedTime != nil {
		lastModifiedTime = types.StringValue(audit.LastModifiedTime.Format(timeFormat))
	} else {
		lastModifiedTime = types.StringNull()
	}

	attrs := map[string]attr.Value{
		"creator":            types.StringValue(audit.Creator),
		"create_time":        createTime,
		"last_modifier":      types.StringValue(audit.LastModifier),
		"last_modified_time": lastModifiedTime,
	}

	return types.ObjectValue(AuditAttrTypes, attrs)
}
