// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package securityhub

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/securityhub"
	awstypes "github.com/aws/aws-sdk-go-v2/service/securityhub/types"
	"github.com/hashicorp/aws-sdk-go-base/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/enum"
	"github.com/hashicorp/terraform-provider-aws/internal/errs"
	"github.com/hashicorp/terraform-provider-aws/internal/framework"
	"github.com/hashicorp/terraform-provider-aws/internal/framework/flex"
	fwtypes "github.com/hashicorp/terraform-provider-aws/internal/framework/types"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @FrameworkResource(name="Automation Rule")
// @Tags(identifierAttribute="arn")
func newAutomationRuleResource(_ context.Context) (resource.ResourceWithConfigure, error) {
	return &automationRuleResource{}, nil
}

const (
	ResNameAutomationRule = "Automation Rule"
)

type automationRuleResource struct {
	framework.ResourceWithConfigure
	framework.WithImportByID
}

func (r *automationRuleResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = "aws_securityhub_automation_rule"
}

func (r *automationRuleResource) Schema(ctx context.Context, request resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			names.AttrARN: framework.ARNAttributeComputedOnly(),
			"description": schema.StringAttribute{
				Required: true,
			},
			names.AttrID: framework.IDAttribute(),
			"is_terminal": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"rule_name": schema.StringAttribute{
				Required: true,
			},
			"rule_order": schema.Int64Attribute{
				Required: true,
			},
			"rule_status": schema.StringAttribute{
				Computed:   true,
				Optional:   true,
				Validators: []validator.String{enum.FrameworkValidate[awstypes.RuleStatus]()},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			names.AttrTags:    tftags.TagsAttribute(),
			names.AttrTagsAll: tftags.TagsAttributeComputedOnly(),
		},
		Blocks: map[string]schema.Block{
			"actions": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Optional:   true,
							Validators: []validator.String{enum.FrameworkValidate[awstypes.AutomationRulesActionType]()},
						},
					},
					Blocks: map[string]schema.Block{
						"finding_fields_update": schema.ListNestedBlock{
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"confidence": schema.Int64Attribute{
										Optional: true,
									},
									"criticality": schema.Int64Attribute{
										Optional: true,
									},
									"types": schema.ListAttribute{
										ElementType: types.StringType,
										Optional:    true,
									},
									"user_defined_fields": schema.MapAttribute{
										ElementType: types.StringType,
										Optional:    true,
									},
									"verification_state": schema.StringAttribute{
										Optional:   true,
										Validators: []validator.String{enum.FrameworkValidate[awstypes.VerificationState]()},
									},
								},
								Blocks: map[string]schema.Block{
									"note": schema.ListNestedBlock{
										Validators: []validator.List{
											listvalidator.SizeAtMost(1),
										},
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"text": schema.StringAttribute{
													Required: true,
												},
												"updated_by": schema.StringAttribute{
													Required: true,
												},
											},
										},
									},
									"related_findings": schema.SetNestedBlock{
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"id": schema.StringAttribute{
													Required: true,
												},
												"product_arn": schema.StringAttribute{
													CustomType: fwtypes.ARNType,
													Required:   true,
												},
											},
										},
									},
									"severity": schema.ListNestedBlock{
										Validators: []validator.List{
											listvalidator.SizeAtMost(1),
										},
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"label": schema.StringAttribute{
													Optional:   true,
													Computed:   true,
													Validators: []validator.String{enum.FrameworkValidate[awstypes.SeverityLabel]()},
												},
												"product": schema.Float64Attribute{
													Optional: true,
												},
											},
										},
									},
									"workflow": schema.ListNestedBlock{
										Validators: []validator.List{
											listvalidator.SizeAtMost(1),
										},
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"status": schema.StringAttribute{
													Optional:   true,
													Validators: []validator.String{enum.FrameworkValidate[awstypes.WorkflowStatus]()},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"criteria": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"aws_account_id":                     StringFilterSchema(),
						"aws_account_name":                   StringFilterSchema(),
						"company_name":                       StringFilterSchema(),
						"compliance_associated_standards_id": StringFilterSchema(),
						"compliance_security_control_id":     StringFilterSchema(),
						"compliance_status":                  StringFilterSchema(),
						"confidence":                         NumberFilterSchema(),
						"created_at":                         DateFilterSchema(),
						"criticality":                        NumberFilterSchema(),
						"description":                        StringFilterSchema(),
						"first_observed_at":                  DateFilterSchema(),
						"generator_id":                       StringFilterSchema(),
						"id":                                 StringFilterSchema(),
						"last_observed_at":                   DateFilterSchema(),
						"note_text":                          StringFilterSchema(),
						"note_updated_at":                    DateFilterSchema(),
						"note_updated_by":                    StringFilterSchema(),
						"product_arn":                        StringFilterSchema(),
						"product_name":                       StringFilterSchema(),
						"record_state":                       StringFilterSchema(),
						"related_findings_id":                StringFilterSchema(),
						"related_findings_product_arn":       StringFilterSchema(),
						"resource_application_arn":           StringFilterSchema(),
						"resource_application_name":          StringFilterSchema(),
						"resource_details_other":             MapFilterSchema(),
						"resource_id":                        StringFilterSchema(),
						"resource_partition":                 StringFilterSchema(),
						"resource_region":                    StringFilterSchema(),
						"resource_tags":                      MapFilterSchema(),
						"resource_type":                      StringFilterSchema(),
						"severity_label":                     StringFilterSchema(),
						"source_url":                         StringFilterSchema(),
						"title":                              StringFilterSchema(),
						"type":                               StringFilterSchema(),
						"updated_at":                         DateFilterSchema(),
						"user_defined_fields":                MapFilterSchema(),
						"verification_state":                 StringFilterSchema(),
						"workflow_status":                    StringFilterSchema(),
					},
				},
			},
		},
	}
}

func StringFilterSchema() schema.SetNestedBlock {
	return schema.SetNestedBlock{
		Validators: []validator.Set{
			setvalidator.SizeAtMost(20),
		},
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"comparison": schema.StringAttribute{
					Required:   true,
					Validators: []validator.String{enum.FrameworkValidate[awstypes.StringFilterComparison]()},
				},
				"value": schema.StringAttribute{
					Required: true,
				},
			},
		},
	}
}

func NumberFilterSchema() schema.SetNestedBlock {
	return schema.SetNestedBlock{
		Validators: []validator.Set{
			setvalidator.SizeAtMost(20),
		},
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"eq": schema.Float64Attribute{
					Optional: true,
				},
				"gte": schema.Float64Attribute{
					Optional: true,
				},
				"lte": schema.Float64Attribute{
					Optional: true,
				},
			},
		},
	}
}

func DateFilterSchema() schema.SetNestedBlock {
	return schema.SetNestedBlock{
		Validators: []validator.Set{
			setvalidator.SizeAtMost(20),
		},
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"end": schema.StringAttribute{
					Optional: true,
				},
				"start": schema.StringAttribute{
					Optional: true,
				},
			},
			Blocks: map[string]schema.Block{
				"date_range": schema.ListNestedBlock{
					Validators: []validator.List{
						listvalidator.SizeAtMost(1),
					},
					NestedObject: schema.NestedBlockObject{
						Attributes: map[string]schema.Attribute{
							"unit": schema.StringAttribute{
								Required:   true,
								Validators: []validator.String{enum.FrameworkValidate[awstypes.DateRangeUnit]()},
							},
							"value": schema.Int64Attribute{
								Required: true,
							},
						},
					},
				},
			},
		},
	}
}

func MapFilterSchema() schema.SetNestedBlock {
	return schema.SetNestedBlock{
		Validators: []validator.Set{
			setvalidator.SizeAtMost(20),
		},
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"comparison": schema.StringAttribute{
					Required:   true,
					Validators: []validator.String{enum.FrameworkValidate[awstypes.MapFilterComparison]()},
				},
				"key": schema.StringAttribute{
					Required: true,
				},
				"value": schema.StringAttribute{
					Required: true,
				},
			},
		},
	}
}

func (r *automationRuleResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var data automationRuleResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	conn := r.Meta().SecurityHubClient(ctx)

	in := &securityhub.CreateAutomationRuleInput{
		Description: aws.String(data.Description.ValueString()),
		IsTerminal:  aws.Bool(data.IsTerminal.ValueBool()),
		RuleName:    aws.String(data.RuleName.ValueString()),
		RuleOrder:   aws.Int32(int32(data.RuleOrder.ValueInt64())),
		Tags:        getTagsIn(ctx),
	}

	if !data.Actions.IsNull() {
		var tfList []actionsData
		response.Diagnostics.Append(data.Actions.ElementsAs(ctx, &tfList, false)...)
		if response.Diagnostics.HasError() {
			return
		}

		actions, d := expandActions(ctx, tfList)
		response.Diagnostics.Append(d...)
		if response.Diagnostics.HasError() {
			return
		}
		in.Actions = actions
	}

	if !data.Criteria.IsNull() {
		var tfList []criteriaData
		response.Diagnostics.Append(data.Criteria.ElementsAs(ctx, &tfList, false)...)
		if response.Diagnostics.HasError() {
			return
		}

		criteria, d := expandCriteria(ctx, tfList)
		response.Diagnostics.Append(d...)
		if response.Diagnostics.HasError() {
			return
		}
		in.Criteria = criteria
	}

	if !data.RuleStatus.IsNull() {
		in.RuleStatus = awstypes.RuleStatus(data.RuleStatus.ValueString())
	}

	out, err := conn.CreateAutomationRule(ctx, in)
	if err != nil {
		response.Diagnostics.AddError(
			create.ProblemStandardMessage(names.SecurityHub, create.ErrActionCreating, ResNameAutomationRule, data.RuleName.String(), err),
			err.Error(),
		)
		return
	}
	if out == nil {
		response.Diagnostics.AddError(
			create.ProblemStandardMessage(names.SecurityHub, create.ErrActionCreating, ResNameAutomationRule, data.RuleName.String(), nil),
			errors.New("empty output").Error(),
		)
		return
	}

	data.ARN = flex.StringToFramework(ctx, out.RuleArn)
	data.ID = flex.StringToFramework(ctx, out.RuleArn)

	// Read to get computed attributes omitted from create response
	readOut, err := findAutomationRuleByARN(ctx, conn, data.ARN.ValueString())
	if err != nil {
		response.Diagnostics.AddError(
			create.ProblemStandardMessage(names.SecurityHub, create.ErrActionReading, ResNameAutomationRule, data.ID.String(), err),
			err.Error(),
		)
		return
	}

	data.RuleStatus = flex.StringValueToFramework(ctx, readOut.RuleStatus)

	response.Diagnostics.Append(response.State.Set(ctx, data)...)
}

func (r *automationRuleResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var data automationRuleResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	conn := r.Meta().SecurityHubClient(ctx)

	out, err := findAutomationRuleByARN(ctx, conn, data.ID.ValueString())

	if tfresource.NotFound(err) {
		response.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		response.Diagnostics.AddError(
			create.ProblemStandardMessage(names.SecurityHub, create.ErrActionReading, ResNameAutomationRule, data.ID.String(), err),
			err.Error(),
		)
		return
	}

	data.ARN = flex.StringToFramework(ctx, out.RuleArn)
	data.Description = flex.StringToFramework(ctx, out.Description)
	data.ID = flex.StringToFramework(ctx, out.RuleArn)
	data.IsTerminal = flex.BoolToFramework(ctx, out.IsTerminal)
	data.RuleName = flex.StringToFramework(ctx, out.RuleName)
	data.RuleOrder = flex.Int32ToFramework(ctx, out.RuleOrder)
	data.RuleStatus = flex.StringValueToFramework(ctx, out.RuleStatus)

	actions, d := flattenActions(ctx, out.Actions)
	response.Diagnostics.Append(d...)
	data.Actions = actions

	criteria, d := flattenCriteria(ctx, out.Criteria)
	response.Diagnostics.Append(d...)
	data.Criteria = criteria

	response.Diagnostics.Append(response.State.Set(ctx, &data)...)
}

func (r *automationRuleResource) Update(ctx context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	var old, new automationRuleResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &new)...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(request.State.Get(ctx, &old)...)
	if response.Diagnostics.HasError() {
		return
	}

	conn := r.Meta().SecurityHubClient(ctx)

	if !new.Actions.Equal(old.Actions) ||
		!new.Criteria.Equal(old.Criteria) ||
		!new.Description.Equal(old.Description) ||
		!new.IsTerminal.Equal(old.IsTerminal) ||
		!new.RuleName.Equal(old.RuleName) ||
		!new.RuleOrder.Equal(old.RuleOrder) ||
		!new.RuleStatus.Equal(old.RuleStatus) {
		in := &securityhub.BatchUpdateAutomationRulesInput{}
		automationRuleItem := awstypes.UpdateAutomationRulesRequestItem{
			Description: aws.String(new.Description.ValueString()),
			IsTerminal:  aws.Bool(new.IsTerminal.ValueBool()),
			RuleArn:     aws.String(new.ARN.ValueString()),
			RuleName:    aws.String(new.RuleName.ValueString()),
			RuleOrder:   aws.Int32(int32(new.RuleOrder.ValueInt64())),
		}

		if !new.Actions.IsNull() {
			var tfList []actionsData
			response.Diagnostics.Append(new.Actions.ElementsAs(ctx, &tfList, false)...)
			if response.Diagnostics.HasError() {
				return
			}

			actions, d := expandActions(ctx, tfList)
			response.Diagnostics.Append(d...)
			if response.Diagnostics.HasError() {
				return
			}
			automationRuleItem.Actions = actions
		}

		if !new.Criteria.IsNull() {
			var tfList []criteriaData
			response.Diagnostics.Append(new.Criteria.ElementsAs(ctx, &tfList, false)...)
			if response.Diagnostics.HasError() {
				return
			}

			criteria, d := expandCriteria(ctx, tfList)
			response.Diagnostics.Append(d...)
			if response.Diagnostics.HasError() {
				return
			}
			automationRuleItem.Criteria = criteria
		}

		if !new.RuleStatus.IsNull() {
			automationRuleItem.RuleStatus = awstypes.RuleStatus(new.RuleStatus.ValueString())
		}

		in.UpdateAutomationRulesRequestItems = append(in.UpdateAutomationRulesRequestItems, automationRuleItem)

		out, err := conn.BatchUpdateAutomationRules(ctx, in)
		if err != nil {
			response.Diagnostics.AddError(
				create.ProblemStandardMessage(names.SecurityHub, create.ErrActionUpdating, ResNameAutomationRule, new.ID.String(), err),
				err.Error(),
			)
			return
		}
		if out == nil {
			response.Diagnostics.AddError(
				create.ProblemStandardMessage(names.SecurityHub, create.ErrActionUpdating, ResNameAutomationRule, new.ID.String(), nil),
				errors.New("empty output").Error(),
			)
			return
		}
	}

	response.Diagnostics.Append(response.State.Set(ctx, &new)...)
}

func (r *automationRuleResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var data automationRuleResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &data)...)
	if response.Diagnostics.HasError() {
		return
	}

	conn := r.Meta().SecurityHubClient(ctx)

	in := &securityhub.BatchDeleteAutomationRulesInput{
		AutomationRulesArns: []string{data.ARN.ValueString()},
	}

	_, err := conn.BatchDeleteAutomationRules(ctx, in)

	if errs.IsA[*awstypes.ResourceNotFoundException](err) {
		return
	}

	if err != nil {
		response.Diagnostics.AddError(
			create.ProblemStandardMessage(names.SecurityHub, create.ErrActionDeleting, ResNameAutomationRule, data.ID.String(), err),
			err.Error(),
		)
		return
	}
}

func (r *automationRuleResource) ModifyPlan(ctx context.Context, request resource.ModifyPlanRequest, response *resource.ModifyPlanResponse) {
	r.SetTagsAll(ctx, request, response)
}

func findAutomationRuleByARN(ctx context.Context, conn *securityhub.Client, arn string) (*awstypes.AutomationRulesConfig, error) {
	input := &securityhub.BatchGetAutomationRulesInput{
		AutomationRulesArns: []string{arn},
	}

	return findAutomationRule(ctx, conn, input)
}

func findAutomationRule(ctx context.Context, conn *securityhub.Client, input *securityhub.BatchGetAutomationRulesInput) (*awstypes.AutomationRulesConfig, error) {
	output, err := findAutomationRules(ctx, conn, input)

	if err != nil {
		return nil, err
	}

	return tfresource.AssertSingleValueResult(output)
}

func findAutomationRules(ctx context.Context, conn *securityhub.Client, input *securityhub.BatchGetAutomationRulesInput) ([]awstypes.AutomationRulesConfig, error) {
	output, err := conn.BatchGetAutomationRules(ctx, input)

	if tfawserr.ErrCodeEquals(err, errCodeResourceNotFoundException) || tfawserr.ErrMessageContains(err, errCodeInvalidAccessException, "not subscribed to AWS Security Hub") {
		return nil, &retry.NotFoundError{
			LastError:   err,
			LastRequest: input,
		}
	}

	if err != nil {
		return nil, err
	}

	if output == nil {
		return nil, tfresource.NewEmptyResultError(input)
	}

	return output.Rules, nil
}

func expandActions(ctx context.Context, tfList []actionsData) ([]awstypes.AutomationRulesAction, diag.Diagnostics) {
	var diags diag.Diagnostics

	if len(tfList) == 0 {
		return nil, diags
	}

	apiResult := []awstypes.AutomationRulesAction{}

	for _, action := range tfList {
		apiObject := awstypes.AutomationRulesAction{}
		if !action.FindingFieldsUpdate.IsNull() {
			var tfList []findingFieldsUpdateData
			diags.Append(action.FindingFieldsUpdate.ElementsAs(ctx, &tfList, false)...)
			if diags.HasError() {
				return nil, diags
			}

			findingFieldsUpdate, d := expandFindingFieldsUpdate(ctx, tfList)
			diags.Append(d...)
			if diags.HasError() {
				return nil, diags
			}
			apiObject.FindingFieldsUpdate = findingFieldsUpdate
		}

		if !action.Type.IsNull() {
			apiObject.Type = awstypes.AutomationRulesActionType(action.Type.ValueString())
		}

		apiResult = append(apiResult, apiObject)
	}

	return apiResult, diags
}

func expandFindingFieldsUpdate(ctx context.Context, tfList []findingFieldsUpdateData) (*awstypes.AutomationRulesFindingFieldsUpdate, diag.Diagnostics) {
	var diags diag.Diagnostics

	if len(tfList) == 0 {
		return nil, diags
	}

	tfObj := tfList[0]

	apiObject := awstypes.AutomationRulesFindingFieldsUpdate{}

	if !tfObj.Confidence.IsNull() {
		apiObject.Confidence = aws.Int32(int32(tfObj.Confidence.ValueInt64()))
	}

	if !tfObj.Criticality.IsNull() {
		apiObject.Criticality = aws.Int32(int32(tfObj.Criticality.ValueInt64()))
	}

	if !tfObj.Note.IsNull() {
		var tfList []noteData
		diags.Append(tfObj.Note.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Note = expandNote(tfList)
	}

	if !tfObj.RelatedFindings.IsNull() {
		var tfList []relatedFindingsData
		diags.Append(tfObj.RelatedFindings.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.RelatedFindings = expandRelatedFindings(tfList)
	}

	if !tfObj.Severity.IsNull() {
		var tfList []severityData
		diags.Append(tfObj.Severity.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Severity = expandSeverity(tfList)
	}

	if !tfObj.Types.IsNull() {
		apiObject.Types = flex.ExpandFrameworkStringValueList(ctx, tfObj.Types)
	}

	if !tfObj.UserDefinedFields.IsNull() {
		apiObject.UserDefinedFields = flex.ExpandFrameworkStringValueMap(ctx, tfObj.UserDefinedFields)
	}

	if !tfObj.VerificationState.IsNull() {
		apiObject.VerificationState = awstypes.VerificationState(tfObj.VerificationState.ValueString())
	}

	if !tfObj.Workflow.IsNull() {
		var tfList []workflowData
		diags.Append(tfObj.Workflow.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Workflow = expandWorkflow(tfList)
	}

	return &apiObject, diags
}

func expandNote(tfList []noteData) *awstypes.NoteUpdate {
	if len(tfList) == 0 {
		return nil
	}

	tfObj := tfList[0]

	apiObject := awstypes.NoteUpdate{
		Text:      aws.String(tfObj.Text.ValueString()),
		UpdatedBy: aws.String(tfObj.UpdatedBy.ValueString()),
	}

	return &apiObject
}

func expandRelatedFindings(tfList []relatedFindingsData) []awstypes.RelatedFinding {
	if len(tfList) == 0 {
		return nil
	}

	apiResult := []awstypes.RelatedFinding{}

	for _, relatedFinding := range tfList {
		apiObject := awstypes.RelatedFinding{
			Id:         aws.String(relatedFinding.Id.ValueString()),
			ProductArn: aws.String(relatedFinding.ProductARN.ValueString()),
		}

		apiResult = append(apiResult, apiObject)
	}

	return apiResult
}

func expandSeverity(tfList []severityData) *awstypes.SeverityUpdate {
	if len(tfList) == 0 {
		return nil
	}

	tfObj := tfList[0]

	apiObject := awstypes.SeverityUpdate{}

	if !tfObj.Label.IsNull() {
		apiObject.Label = awstypes.SeverityLabel(tfObj.Label.ValueString())
	}

	if !tfObj.Product.IsNull() {
		apiObject.Product = aws.Float64(tfObj.Product.ValueFloat64())
	}

	return &apiObject
}

func expandWorkflow(tfList []workflowData) *awstypes.WorkflowUpdate {
	if len(tfList) == 0 {
		return nil
	}

	tfObj := tfList[0]

	apiObject := awstypes.WorkflowUpdate{}

	if !tfObj.Status.IsNull() {
		apiObject.Status = awstypes.WorkflowStatus(tfObj.Status.ValueString())
	}

	return &apiObject
}

func expandCriteria(ctx context.Context, tfList []criteriaData) (*awstypes.AutomationRulesFindingFilters, diag.Diagnostics) {
	var diags diag.Diagnostics

	if len(tfList) == 0 {
		return nil, diags
	}

	tfObj := tfList[0]

	apiObject := awstypes.AutomationRulesFindingFilters{}

	if !tfObj.AWSAccountId.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.AWSAccountId.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.AwsAccountId = expandStringFilter(tfList)
	}

	if !tfObj.AWSAccountName.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.AWSAccountName.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.AwsAccountName = expandStringFilter(tfList)
	}

	if !tfObj.CompanyName.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.CompanyName.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.CompanyName = expandStringFilter(tfList)
	}

	if !tfObj.ComplianceAssociatedStandardsId.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ComplianceAssociatedStandardsId.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ComplianceAssociatedStandardsId = expandStringFilter(tfList)
	}

	if !tfObj.ComplianceSecurityControlId.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ComplianceSecurityControlId.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ComplianceSecurityControlId = expandStringFilter(tfList)
	}

	if !tfObj.ComplianceStatus.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ComplianceStatus.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ComplianceStatus = expandStringFilter(tfList)
	}

	if !tfObj.Confidence.IsNull() {
		var tfList []numberFilterData
		diags.Append(tfObj.Confidence.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Confidence = expandNumberFilter(tfList)
	}

	if !tfObj.CreatedAt.IsNull() {
		var tfList []dateFilterData
		diags.Append(tfObj.CreatedAt.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		createdAt, d := expandDateFilter(ctx, tfList)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		apiObject.CreatedAt = createdAt
	}

	if !tfObj.Criticality.IsNull() {
		var tfList []numberFilterData
		diags.Append(tfObj.Criticality.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Criticality = expandNumberFilter(tfList)
	}

	if !tfObj.Description.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.Description.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Description = expandStringFilter(tfList)
	}

	if !tfObj.FirstObservedAt.IsNull() {
		var tfList []dateFilterData
		diags.Append(tfObj.FirstObservedAt.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		firstObservedAt, d := expandDateFilter(ctx, tfList)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		apiObject.FirstObservedAt = firstObservedAt
	}

	if !tfObj.GeneratorId.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.GeneratorId.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.GeneratorId = expandStringFilter(tfList)
	}

	if !tfObj.Id.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.Id.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Id = expandStringFilter(tfList)
	}

	if !tfObj.LastObservedAt.IsNull() {
		var tfList []dateFilterData
		diags.Append(tfObj.LastObservedAt.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		lastObservedAt, d := expandDateFilter(ctx, tfList)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		apiObject.LastObservedAt = lastObservedAt
	}

	if !tfObj.NoteText.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.NoteText.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.NoteText = expandStringFilter(tfList)
	}

	if !tfObj.NoteUpdatedAt.IsNull() {
		var tfList []dateFilterData
		diags.Append(tfObj.NoteUpdatedAt.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		noteUpdatedAt, d := expandDateFilter(ctx, tfList)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		apiObject.NoteUpdatedAt = noteUpdatedAt
	}

	if !tfObj.NoteUpdatedBy.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.NoteUpdatedBy.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.NoteUpdatedBy = expandStringFilter(tfList)
	}

	if !tfObj.ProductARN.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ProductARN.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ProductArn = expandStringFilter(tfList)
	}

	if !tfObj.ProductName.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ProductName.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ProductName = expandStringFilter(tfList)
	}

	if !tfObj.RecordState.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.RecordState.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.RecordState = expandStringFilter(tfList)
	}

	if !tfObj.RelatedFindingsId.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.RelatedFindingsId.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.RelatedFindingsId = expandStringFilter(tfList)
	}

	if !tfObj.RelatedFindingsProductArn.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.RelatedFindingsProductArn.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.RelatedFindingsProductArn = expandStringFilter(tfList)
	}

	if !tfObj.ResourceApplicationArn.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ResourceApplicationArn.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ResourceApplicationArn = expandStringFilter(tfList)
	}

	if !tfObj.ResourceApplicationName.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ResourceApplicationName.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ResourceApplicationName = expandStringFilter(tfList)
	}

	if !tfObj.ResourceDetailsOther.IsNull() {
		var tfList []mapFilterData
		diags.Append(tfObj.ResourceDetailsOther.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ResourceDetailsOther = expandMapFilter(tfList)
	}

	if !tfObj.ResourceId.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ResourceId.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ResourceId = expandStringFilter(tfList)
	}

	if !tfObj.ResourcePartition.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ResourcePartition.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ResourcePartition = expandStringFilter(tfList)
	}

	if !tfObj.ResourceRegion.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ResourceRegion.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ResourceRegion = expandStringFilter(tfList)
	}

	if !tfObj.ResourceTags.IsNull() {
		var tfList []mapFilterData
		diags.Append(tfObj.ResourceTags.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ResourceTags = expandMapFilter(tfList)
	}

	if !tfObj.ResourceType.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.ResourceType.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.ResourceType = expandStringFilter(tfList)
	}

	if !tfObj.SeverityLabel.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.SeverityLabel.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.SeverityLabel = expandStringFilter(tfList)
	}

	if !tfObj.SourceUrl.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.SourceUrl.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.SourceUrl = expandStringFilter(tfList)
	}

	if !tfObj.Title.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.Title.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Title = expandStringFilter(tfList)
	}

	if !tfObj.Type.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.Type.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.Type = expandStringFilter(tfList)
	}

	if !tfObj.UpdatedAt.IsNull() {
		var tfList []dateFilterData
		diags.Append(tfObj.UpdatedAt.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		updatedAt, d := expandDateFilter(ctx, tfList)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		apiObject.UpdatedAt = updatedAt
	}

	if !tfObj.UserDefinedFields.IsNull() {
		var tfList []mapFilterData
		diags.Append(tfObj.UserDefinedFields.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.UserDefinedFields = expandMapFilter(tfList)
	}

	if !tfObj.VerificationState.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.VerificationState.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.VerificationState = expandStringFilter(tfList)
	}

	if !tfObj.WorkflowStatus.IsNull() {
		var tfList []stringFilterData
		diags.Append(tfObj.WorkflowStatus.ElementsAs(ctx, &tfList, false)...)
		if diags.HasError() {
			return nil, diags
		}

		apiObject.WorkflowStatus = expandStringFilter(tfList)
	}

	return &apiObject, diags
}

func expandStringFilter(tfList []stringFilterData) []awstypes.StringFilter {
	if len(tfList) == 0 {
		return nil
	}

	apiResult := []awstypes.StringFilter{}

	for _, filter := range tfList {
		apiObject := awstypes.StringFilter{
			Comparison: awstypes.StringFilterComparison(filter.Comparison.ValueString()),
			Value:      aws.String(filter.Value.ValueString()),
		}

		apiResult = append(apiResult, apiObject)
	}

	return apiResult
}

func expandNumberFilter(tfList []numberFilterData) []awstypes.NumberFilter {
	if len(tfList) == 0 {
		return nil
	}

	apiResult := []awstypes.NumberFilter{}

	for _, filter := range tfList {
		apiObject := awstypes.NumberFilter{}

		if !filter.Eq.IsNull() {
			apiObject.Eq = aws.Float64(filter.Eq.ValueFloat64())
		}

		if !filter.Gte.IsNull() {
			apiObject.Gte = aws.Float64(filter.Gte.ValueFloat64())
		}

		if !filter.Lte.IsNull() {
			apiObject.Lte = aws.Float64(filter.Lte.ValueFloat64())
		}

		apiResult = append(apiResult, apiObject)
	}

	return apiResult
}

func expandMapFilter(tfList []mapFilterData) []awstypes.MapFilter {
	if len(tfList) == 0 {
		return nil
	}

	apiResult := []awstypes.MapFilter{}

	for _, filter := range tfList {
		apiObject := awstypes.MapFilter{}

		if !filter.Comparison.IsNull() {
			apiObject.Comparison = awstypes.MapFilterComparison(filter.Comparison.ValueString())
		}

		if !filter.Key.IsNull() {
			apiObject.Key = aws.String(filter.Key.ValueString())
		}

		if !filter.Value.IsNull() {
			apiObject.Value = aws.String(filter.Value.ValueString())
		}

		apiResult = append(apiResult, apiObject)
	}

	return apiResult
}

func expandDateFilter(ctx context.Context, tfList []dateFilterData) ([]awstypes.DateFilter, diag.Diagnostics) {
	var diags diag.Diagnostics

	if len(tfList) == 0 {
		return nil, diags
	}

	apiResult := []awstypes.DateFilter{}

	for _, filter := range tfList {
		apiObject := awstypes.DateFilter{}

		if !filter.DateRange.IsNull() {
			var tfList []dateRangeData
			diags.Append(filter.DateRange.ElementsAs(ctx, &tfList, false)...)
			if diags.HasError() {
				return nil, diags
			}

			apiObject.DateRange = expandDateRange(tfList)
		}

		if !filter.End.IsNull() {
			apiObject.End = aws.String(filter.End.ValueString())
		}

		if !filter.Start.IsNull() {
			apiObject.Start = aws.String(filter.Start.ValueString())
		}

		apiResult = append(apiResult, apiObject)
	}

	return apiResult, diags
}

func expandDateRange(tfList []dateRangeData) *awstypes.DateRange {
	if len(tfList) == 0 {
		return nil
	}

	tfObj := tfList[0]

	apiObject := awstypes.DateRange{}

	if !tfObj.Unit.IsNull() {
		apiObject.Unit = awstypes.DateRangeUnit(tfObj.Unit.ValueString())
	}

	if !tfObj.Value.IsNull() {
		apiObject.Value = aws.Int32(int32(tfObj.Value.ValueInt64()))
	}

	return &apiObject
}

func flattenActions(ctx context.Context, apiObject []awstypes.AutomationRulesAction) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: actionsAttrTypes}

	if apiObject == nil {
		return types.SetNull(elemType), diags
	}

	result := []attr.Value{}

	for _, action := range apiObject {
		findingFieldsUpdate, d := flattenFindingFieldsUpdate(ctx, action.FindingFieldsUpdate)
		diags.Append(d...)

		obj := map[string]attr.Value{
			"finding_fields_update": findingFieldsUpdate,
			"type":                  flex.StringValueToFramework(ctx, action.Type),
		}

		objVal, d := types.ObjectValue(actionsAttrTypes, obj)
		diags.Append(d...)

		result = append(result, objVal)
	}

	setVal, d := types.SetValue(elemType, result)
	diags.Append(d...)

	return setVal, diags
}

func flattenFindingFieldsUpdate(ctx context.Context, apiObject *awstypes.AutomationRulesFindingFieldsUpdate) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: findingFieldsUpdateAttrTypes}

	if apiObject == nil {
		return types.ListNull(elemType), diags
	}

	note, d := flattenNote(ctx, apiObject.Note)
	diags.Append(d...)

	relatedFindings, d := flattenRelatedFindings(ctx, apiObject.RelatedFindings)
	diags.Append(d...)

	severity, d := flattenSeverity(ctx, apiObject.Severity)
	diags.Append(d...)

	workflow, d := flattenWorkflow(ctx, apiObject.Workflow)
	diags.Append(d...)

	obj := map[string]attr.Value{
		"confidence":          flex.Int32ToFramework(ctx, apiObject.Confidence),
		"criticality":         flex.Int32ToFramework(ctx, apiObject.Criticality),
		"note":                note,
		"related_findings":    relatedFindings,
		"severity":            severity,
		"types":               flex.FlattenFrameworkStringValueList(ctx, apiObject.Types),
		"user_defined_fields": flex.FlattenFrameworkStringValueMap(ctx, apiObject.UserDefinedFields),
		"verification_state":  flex.StringValueToFramework(ctx, apiObject.VerificationState),
		"workflow":            workflow,
	}

	objVal, d := types.ObjectValue(findingFieldsUpdateAttrTypes, obj)
	diags.Append(d...)

	listVal, d := types.ListValue(elemType, []attr.Value{objVal})
	diags.Append(d...)

	return listVal, diags
}

func flattenNote(ctx context.Context, apiObject *awstypes.NoteUpdate) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: noteAttrTypes}

	if apiObject == nil {
		return types.ListNull(elemType), diags
	}

	obj := map[string]attr.Value{
		"text":       flex.StringToFramework(ctx, apiObject.Text),
		"updated_by": flex.StringToFramework(ctx, apiObject.UpdatedBy),
	}

	objVal, d := types.ObjectValue(noteAttrTypes, obj)
	diags.Append(d...)

	listVal, d := types.ListValue(elemType, []attr.Value{objVal})
	diags.Append(d...)

	return listVal, diags
}

func flattenRelatedFindings(ctx context.Context, apiObject []awstypes.RelatedFinding) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: relatedFindingsAttrTypes}

	if len(apiObject) == 0 {
		return types.SetNull(elemType), diags
	}

	result := []attr.Value{}

	for _, relatedFinding := range apiObject {
		obj := map[string]attr.Value{
			"id":          flex.StringToFramework(ctx, relatedFinding.Id),
			"product_arn": flex.StringToFrameworkARN(ctx, relatedFinding.ProductArn),
		}

		objVal, d := types.ObjectValue(relatedFindingsAttrTypes, obj)
		diags.Append(d...)

		result = append(result, objVal)
	}

	setVal, d := types.SetValue(elemType, result)
	diags.Append(d...)

	return setVal, diags
}

func flattenSeverity(ctx context.Context, apiObject *awstypes.SeverityUpdate) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: severityAttrTypes}

	if apiObject == nil {
		return types.ListNull(elemType), diags
	}

	obj := map[string]attr.Value{
		"label": flex.StringValueToFramework(ctx, apiObject.Label),
	}

	if apiObject.Product != nil {
		obj["product"] = flex.Float64ToFramework(ctx, apiObject.Product)
	}

	objVal, d := types.ObjectValue(severityAttrTypes, obj)
	diags.Append(d...)

	listVal, d := types.ListValue(elemType, []attr.Value{objVal})
	diags.Append(d...)

	return listVal, diags
}

func flattenWorkflow(ctx context.Context, apiObject *awstypes.WorkflowUpdate) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: workflowAttrTypes}

	if apiObject == nil {
		return types.ListNull(elemType), diags
	}

	obj := map[string]attr.Value{
		"status": flex.StringValueToFramework(ctx, apiObject.Status),
	}

	objVal, d := types.ObjectValue(workflowAttrTypes, obj)
	diags.Append(d...)

	listVal, d := types.ListValue(elemType, []attr.Value{objVal})
	diags.Append(d...)

	return listVal, diags
}

func flattenCriteria(ctx context.Context, apiObject *awstypes.AutomationRulesFindingFilters) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: criteriaAttrTypes}

	if apiObject == nil {
		return types.ListNull(elemType), diags
	}

	awsAccountId, d := flattenStringFilter(ctx, apiObject.AwsAccountId)
	diags.Append(d...)

	awsAccountName, d := flattenStringFilter(ctx, apiObject.AwsAccountName)
	diags.Append(d...)

	companyName, d := flattenStringFilter(ctx, apiObject.CompanyName)
	diags.Append(d...)

	complianceAssociatedStandardsId, d := flattenStringFilter(ctx, apiObject.ComplianceAssociatedStandardsId)
	diags.Append(d...)

	complianceSecurityControlId, d := flattenStringFilter(ctx, apiObject.ComplianceSecurityControlId)
	diags.Append(d...)

	complianceStatus, d := flattenStringFilter(ctx, apiObject.ComplianceStatus)
	diags.Append(d...)

	confidence, d := flattenNumberFilter(ctx, apiObject.Confidence)
	diags.Append(d...)

	createdAt, d := flattenDateFilter(ctx, apiObject.CreatedAt)
	diags.Append(d...)

	criticality, d := flattenNumberFilter(ctx, apiObject.Criticality)
	diags.Append(d...)

	description, d := flattenStringFilter(ctx, apiObject.Description)
	diags.Append(d...)

	firstObservedAt, d := flattenDateFilter(ctx, apiObject.FirstObservedAt)
	diags.Append(d...)

	generatorId, d := flattenStringFilter(ctx, apiObject.GeneratorId)
	diags.Append(d...)

	id, d := flattenStringFilter(ctx, apiObject.Id)
	diags.Append(d...)

	lastObservedAt, d := flattenDateFilter(ctx, apiObject.LastObservedAt)
	diags.Append(d...)

	noteText, d := flattenStringFilter(ctx, apiObject.NoteText)
	diags.Append(d...)

	noteUpdatedAt, d := flattenDateFilter(ctx, apiObject.NoteUpdatedAt)
	diags.Append(d...)

	noteUpdatedBy, d := flattenStringFilter(ctx, apiObject.NoteUpdatedBy)
	diags.Append(d...)

	productArn, d := flattenStringFilter(ctx, apiObject.ProductArn)
	diags.Append(d...)

	productName, d := flattenStringFilter(ctx, apiObject.ProductName)
	diags.Append(d...)

	recordState, d := flattenStringFilter(ctx, apiObject.RecordState)
	diags.Append(d...)

	relatedFindingsId, d := flattenStringFilter(ctx, apiObject.RelatedFindingsId)
	diags.Append(d...)

	relatedFindingsProductArn, d := flattenStringFilter(ctx, apiObject.RelatedFindingsProductArn)
	diags.Append(d...)

	resourceApplicationArn, d := flattenStringFilter(ctx, apiObject.ResourceApplicationArn)
	diags.Append(d...)

	resourceApplicationName, d := flattenStringFilter(ctx, apiObject.ResourceApplicationName)
	diags.Append(d...)

	resourceDetailsOther, d := flattenMapFilter(ctx, apiObject.ResourceDetailsOther)
	diags.Append(d...)

	resourceId, d := flattenStringFilter(ctx, apiObject.ResourceId)
	diags.Append(d...)

	resourcePartition, d := flattenStringFilter(ctx, apiObject.ResourcePartition)
	diags.Append(d...)

	resourceRegion, d := flattenStringFilter(ctx, apiObject.ResourceRegion)
	diags.Append(d...)

	resourceTags, d := flattenMapFilter(ctx, apiObject.ResourceTags)
	diags.Append(d...)

	resourceType, d := flattenStringFilter(ctx, apiObject.ResourceType)
	diags.Append(d...)

	severityLabel, d := flattenStringFilter(ctx, apiObject.SeverityLabel)
	diags.Append(d...)

	sourceUrl, d := flattenStringFilter(ctx, apiObject.SourceUrl)
	diags.Append(d...)

	title, d := flattenStringFilter(ctx, apiObject.Title)
	diags.Append(d...)

	typeValue, d := flattenStringFilter(ctx, apiObject.Type)
	diags.Append(d...)

	updatedAt, d := flattenDateFilter(ctx, apiObject.UpdatedAt)
	diags.Append(d...)

	userDefinedFields, d := flattenMapFilter(ctx, apiObject.UserDefinedFields)
	diags.Append(d...)

	verificationState, d := flattenStringFilter(ctx, apiObject.VerificationState)
	diags.Append(d...)

	workflowStatus, d := flattenStringFilter(ctx, apiObject.WorkflowStatus)
	diags.Append(d...)

	obj := map[string]attr.Value{
		"aws_account_id":                     awsAccountId,
		"aws_account_name":                   awsAccountName,
		"company_name":                       companyName,
		"compliance_associated_standards_id": complianceAssociatedStandardsId,
		"compliance_security_control_id":     complianceSecurityControlId,
		"compliance_status":                  complianceStatus,
		"confidence":                         confidence,
		"created_at":                         createdAt,
		"criticality":                        criticality,
		"description":                        description,
		"first_observed_at":                  firstObservedAt,
		"generator_id":                       generatorId,
		"id":                                 id,
		"last_observed_at":                   lastObservedAt,
		"note_text":                          noteText,
		"note_updated_at":                    noteUpdatedAt,
		"note_updated_by":                    noteUpdatedBy,
		"product_arn":                        productArn,
		"product_name":                       productName,
		"record_state":                       recordState,
		"related_findings_id":                relatedFindingsId,
		"related_findings_product_arn":       relatedFindingsProductArn,
		"resource_application_arn":           resourceApplicationArn,
		"resource_application_name":          resourceApplicationName,
		"resource_details_other":             resourceDetailsOther,
		"resource_id":                        resourceId,
		"resource_partition":                 resourcePartition,
		"resource_region":                    resourceRegion,
		"resource_tags":                      resourceTags,
		"resource_type":                      resourceType,
		"severity_label":                     severityLabel,
		"source_url":                         sourceUrl,
		"title":                              title,
		"type":                               typeValue,
		"updated_at":                         updatedAt,
		"user_defined_fields":                userDefinedFields,
		"verification_state":                 verificationState,
		"workflow_status":                    workflowStatus,
	}

	objVal, d := types.ObjectValue(criteriaAttrTypes, obj)
	diags.Append(d...)

	listVal, d := types.ListValue(elemType, []attr.Value{objVal})
	diags.Append(d...)

	return listVal, diags
}

func flattenStringFilter(ctx context.Context, apiObject []awstypes.StringFilter) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: stringFilterAttrTypes}

	if apiObject == nil {
		return types.SetNull(elemType), diags
	}

	result := []attr.Value{}

	for _, filter := range apiObject {
		obj := map[string]attr.Value{
			"comparison": flex.StringValueToFramework(ctx, filter.Comparison),
			"value":      flex.StringToFramework(ctx, filter.Value),
		}
		objVal, d := types.ObjectValue(stringFilterAttrTypes, obj)
		diags.Append(d...)

		result = append(result, objVal)
	}

	setVal, d := types.SetValue(elemType, result)
	diags.Append(d...)

	return setVal, diags
}

func flattenNumberFilter(ctx context.Context, apiObject []awstypes.NumberFilter) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: numberFilterAttrTypes}

	if apiObject == nil {
		return types.SetNull(elemType), diags
	}

	result := []attr.Value{}

	for _, filter := range apiObject {
		obj := map[string]attr.Value{
			"eq":  flex.Float64ToFramework(ctx, filter.Eq),
			"gte": flex.Float64ToFramework(ctx, filter.Gte),
			"lte": flex.Float64ToFramework(ctx, filter.Lte),
		}
		objVal, d := types.ObjectValue(numberFilterAttrTypes, obj)
		diags.Append(d...)

		result = append(result, objVal)
	}

	setVal, d := types.SetValue(elemType, result)
	diags.Append(d...)

	return setVal, diags
}

func flattenMapFilter(ctx context.Context, apiObject []awstypes.MapFilter) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: mapFilterAttrTypes}

	if apiObject == nil {
		return types.SetNull(elemType), diags
	}

	result := []attr.Value{}

	for _, filter := range apiObject {
		obj := map[string]attr.Value{
			"comparison": flex.StringValueToFramework(ctx, filter.Comparison),
			"key":        flex.StringToFramework(ctx, filter.Key),
			"value":      flex.StringToFramework(ctx, filter.Value),
		}
		objVal, d := types.ObjectValue(mapFilterAttrTypes, obj)
		diags.Append(d...)

		result = append(result, objVal)
	}

	setVal, d := types.SetValue(elemType, result)
	diags.Append(d...)

	return setVal, diags
}

func flattenDateFilter(ctx context.Context, apiObject []awstypes.DateFilter) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: dateFilterAttrTypes}

	if apiObject == nil {
		return types.SetNull(elemType), diags
	}

	result := []attr.Value{}

	for _, filter := range apiObject {
		dateRange, d := flattenDateRange(ctx, filter.DateRange)
		diags.Append(d...)

		obj := map[string]attr.Value{
			"date_range": dateRange,
			"end":        flex.StringToFramework(ctx, filter.End),
			"start":      flex.StringToFramework(ctx, filter.Start),
		}
		objVal, d := types.ObjectValue(dateFilterAttrTypes, obj)
		diags.Append(d...)

		result = append(result, objVal)
	}

	setVal, d := types.SetValue(elemType, result)
	diags.Append(d...)

	return setVal, diags
}

func flattenDateRange(ctx context.Context, apiObject *awstypes.DateRange) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: dateRangeAttrTypes}

	if apiObject == nil {
		return types.ListNull(elemType), diags
	}

	obj := map[string]attr.Value{
		"unit":  flex.StringValueToFramework(ctx, apiObject.Unit),
		"value": flex.Int32ToFramework(ctx, apiObject.Value),
	}

	objVal, d := types.ObjectValue(dateRangeAttrTypes, obj)
	diags.Append(d...)

	listVal, d := types.ListValue(elemType, []attr.Value{objVal})
	diags.Append(d...)

	return listVal, diags
}

type automationRuleResourceModel struct {
	Actions     types.Set    `tfsdk:"actions"`
	ARN         types.String `tfsdk:"arn"`
	Criteria    types.List   `tfsdk:"criteria"`
	Description types.String `tfsdk:"description"`
	ID          types.String `tfsdk:"id"`
	IsTerminal  types.Bool   `tfsdk:"is_terminal"`
	RuleName    types.String `tfsdk:"rule_name"`
	RuleOrder   types.Int64  `tfsdk:"rule_order"`
	RuleStatus  types.String `tfsdk:"rule_status"`
	Tags        types.Map    `tfsdk:"tags"`
	TagsAll     types.Map    `tfsdk:"tags_all"`
}

type actionsData struct {
	FindingFieldsUpdate types.List   `tfsdk:"finding_fields_update"`
	Type                types.String `tfsdk:"type"`
}

type findingFieldsUpdateData struct {
	Confidence        types.Int64  `tfsdk:"confidence"`
	Criticality       types.Int64  `tfsdk:"criticality"`
	Note              types.List   `tfsdk:"note"`
	RelatedFindings   types.Set    `tfsdk:"related_findings"`
	Severity          types.List   `tfsdk:"severity"`
	Types             types.List   `tfsdk:"types"`
	UserDefinedFields types.Map    `tfsdk:"user_defined_fields"`
	VerificationState types.String `tfsdk:"verification_state"`
	Workflow          types.List   `tfsdk:"workflow"`
}

type noteData struct {
	Text      types.String `tfsdk:"text"`
	UpdatedBy types.String `tfsdk:"updated_by"`
}

type relatedFindingsData struct {
	Id         types.String `tfsdk:"id"`
	ProductARN fwtypes.ARN  `tfsdk:"product_arn"`
}

type severityData struct {
	Label   types.String  `tfsdk:"label"`
	Product types.Float64 `tfsdk:"product"`
}

type workflowData struct {
	Status types.String `tfsdk:"status"`
}

type criteriaData struct {
	AWSAccountId                    types.Set `tfsdk:"aws_account_id"`
	AWSAccountName                  types.Set `tfsdk:"aws_account_name"`
	CompanyName                     types.Set `tfsdk:"company_name"`
	ComplianceAssociatedStandardsId types.Set `tfsdk:"compliance_associated_standards_id"`
	ComplianceSecurityControlId     types.Set `tfsdk:"compliance_security_control_id"`
	ComplianceStatus                types.Set `tfsdk:"compliance_status"`
	Confidence                      types.Set `tfsdk:"confidence"`
	CreatedAt                       types.Set `tfsdk:"created_at"`
	Criticality                     types.Set `tfsdk:"criticality"`
	Description                     types.Set `tfsdk:"description"`
	FirstObservedAt                 types.Set `tfsdk:"first_observed_at"`
	GeneratorId                     types.Set `tfsdk:"generator_id"`
	Id                              types.Set `tfsdk:"id"`
	LastObservedAt                  types.Set `tfsdk:"last_observed_at"`
	NoteText                        types.Set `tfsdk:"note_text"`
	NoteUpdatedAt                   types.Set `tfsdk:"note_updated_at"`
	NoteUpdatedBy                   types.Set `tfsdk:"note_updated_by"`
	ProductARN                      types.Set `tfsdk:"product_arn"`
	ProductName                     types.Set `tfsdk:"product_name"`
	RecordState                     types.Set `tfsdk:"record_state"`
	RelatedFindingsId               types.Set `tfsdk:"related_findings_id"`
	RelatedFindingsProductArn       types.Set `tfsdk:"related_findings_product_arn"`
	ResourceApplicationArn          types.Set `tfsdk:"resource_application_arn"`
	ResourceApplicationName         types.Set `tfsdk:"resource_application_name"`
	ResourceDetailsOther            types.Set `tfsdk:"resource_details_other"`
	ResourceId                      types.Set `tfsdk:"resource_id"`
	ResourcePartition               types.Set `tfsdk:"resource_partition"`
	ResourceRegion                  types.Set `tfsdk:"resource_region"`
	ResourceTags                    types.Set `tfsdk:"resource_tags"`
	ResourceType                    types.Set `tfsdk:"resource_type"`
	SeverityLabel                   types.Set `tfsdk:"severity_label"`
	SourceUrl                       types.Set `tfsdk:"source_url"`
	Title                           types.Set `tfsdk:"title"`
	Type                            types.Set `tfsdk:"type"`
	UpdatedAt                       types.Set `tfsdk:"updated_at"`
	UserDefinedFields               types.Set `tfsdk:"user_defined_fields"`
	VerificationState               types.Set `tfsdk:"verification_state"`
	WorkflowStatus                  types.Set `tfsdk:"workflow_status"`
}

type dateFilterData struct {
	DateRange types.List   `tfsdk:"date_range"`
	End       types.String `tfsdk:"end"`
	Start     types.String `tfsdk:"start"`
}

type dateRangeData struct {
	Unit  types.String `tfsdk:"unit"`
	Value types.Int64  `tfsdk:"value"`
}

type stringFilterData struct {
	Comparison types.String `tfsdk:"comparison"`
	Value      types.String `tfsdk:"value"`
}

type numberFilterData struct {
	Eq  types.Float64 `tfsdk:"eq"`
	Gte types.Float64 `tfsdk:"gte"`
	Lte types.Float64 `tfsdk:"lte"`
}

type mapFilterData struct {
	Comparison types.String `tfsdk:"comparison"`
	Key        types.String `tfsdk:"key"`
	Value      types.String `tfsdk:"value"`
}

var actionsAttrTypes = map[string]attr.Type{
	"finding_fields_update": types.ListType{ElemType: types.ObjectType{AttrTypes: findingFieldsUpdateAttrTypes}},
	"type":                  types.StringType,
}

var findingFieldsUpdateAttrTypes = map[string]attr.Type{
	"confidence":          types.Int64Type,
	"criticality":         types.Int64Type,
	"note":                types.ListType{ElemType: types.ObjectType{AttrTypes: noteAttrTypes}},
	"related_findings":    types.SetType{ElemType: types.ObjectType{AttrTypes: relatedFindingsAttrTypes}},
	"severity":            types.ListType{ElemType: types.ObjectType{AttrTypes: severityAttrTypes}},
	"types":               types.ListType{ElemType: types.StringType},
	"user_defined_fields": types.MapType{ElemType: types.StringType},
	"verification_state":  types.StringType,
	"workflow":            types.ListType{ElemType: types.ObjectType{AttrTypes: workflowAttrTypes}},
}

var noteAttrTypes = map[string]attr.Type{
	"text":       types.StringType,
	"updated_by": types.StringType,
}

var relatedFindingsAttrTypes = map[string]attr.Type{
	"id":          types.StringType,
	"product_arn": fwtypes.ARNType,
}

var severityAttrTypes = map[string]attr.Type{
	"label":   types.StringType,
	"product": types.Float64Type,
}

var workflowAttrTypes = map[string]attr.Type{
	"status": types.StringType,
}

var criteriaAttrTypes = map[string]attr.Type{
	"aws_account_id":                     types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"aws_account_name":                   types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"company_name":                       types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"compliance_associated_standards_id": types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"compliance_security_control_id":     types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"compliance_status":                  types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"confidence":                         types.SetType{ElemType: types.ObjectType{AttrTypes: numberFilterAttrTypes}},
	"created_at":                         types.SetType{ElemType: types.ObjectType{AttrTypes: dateFilterAttrTypes}},
	"criticality":                        types.SetType{ElemType: types.ObjectType{AttrTypes: numberFilterAttrTypes}},
	"description":                        types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"first_observed_at":                  types.SetType{ElemType: types.ObjectType{AttrTypes: dateFilterAttrTypes}},
	"generator_id":                       types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"id":                                 types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"last_observed_at":                   types.SetType{ElemType: types.ObjectType{AttrTypes: dateFilterAttrTypes}},
	"note_text":                          types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"note_updated_at":                    types.SetType{ElemType: types.ObjectType{AttrTypes: dateFilterAttrTypes}},
	"note_updated_by":                    types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"product_arn":                        types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"product_name":                       types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"record_state":                       types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"related_findings_id":                types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"related_findings_product_arn":       types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"resource_application_arn":           types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"resource_application_name":          types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"resource_details_other":             types.SetType{ElemType: types.ObjectType{AttrTypes: mapFilterAttrTypes}},
	"resource_id":                        types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"resource_partition":                 types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"resource_region":                    types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"resource_tags":                      types.SetType{ElemType: types.ObjectType{AttrTypes: mapFilterAttrTypes}},
	"resource_type":                      types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"severity_label":                     types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"source_url":                         types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"title":                              types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"type":                               types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"updated_at":                         types.SetType{ElemType: types.ObjectType{AttrTypes: dateFilterAttrTypes}},
	"user_defined_fields":                types.SetType{ElemType: types.ObjectType{AttrTypes: mapFilterAttrTypes}},
	"verification_state":                 types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
	"workflow_status":                    types.SetType{ElemType: types.ObjectType{AttrTypes: stringFilterAttrTypes}},
}

var dateFilterAttrTypes = map[string]attr.Type{
	"date_range": types.ListType{ElemType: types.ObjectType{AttrTypes: dateRangeAttrTypes}},
	"end":        types.StringType,
	"start":      types.StringType,
}

var dateRangeAttrTypes = map[string]attr.Type{
	"unit":  types.StringType,
	"value": types.Int64Type,
}

var stringFilterAttrTypes = map[string]attr.Type{
	"comparison": types.StringType,
	"value":      types.StringType,
}

var numberFilterAttrTypes = map[string]attr.Type{
	"eq":  types.Float64Type,
	"gte": types.Float64Type,
	"lte": types.Float64Type,
}

var mapFilterAttrTypes = map[string]attr.Type{
	"comparison": types.StringType,
	"key":        types.StringType,
	"value":      types.StringType,
}
