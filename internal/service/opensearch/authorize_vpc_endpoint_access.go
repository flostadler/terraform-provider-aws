// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package opensearch

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	awstypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-provider-aws/internal/create"
	"github.com/hashicorp/terraform-provider-aws/internal/errs"
	"github.com/hashicorp/terraform-provider-aws/internal/framework"
	"github.com/hashicorp/terraform-provider-aws/internal/framework/flex"
	fwflex "github.com/hashicorp/terraform-provider-aws/internal/framework/flex"
	fwtypes "github.com/hashicorp/terraform-provider-aws/internal/framework/types"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
	"github.com/hashicorp/terraform-provider-aws/names"
)

// @FrameworkResource("aws_opensearch_authorize_vpc_endpoint_access", name="Authorize VPC Endpoint Access")
func newResourceAuthorizeVpcEndpointAccess(_ context.Context) (resource.ResourceWithConfigure, error) {
	r := &resourceAuthorizeVpcEndpointAccess{}

	return r, nil
}

const (
	ResNameAuthorizeVpcEndpointAccess = "Authorize Vpc Endpoint Access"
)

type resourceAuthorizeVpcEndpointAccess struct {
	framework.ResourceWithConfigure
	framework.WithImportByID
	framework.WithNoUpdate
}

func (r *resourceAuthorizeVpcEndpointAccess) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "aws_opensearch_authorize_vpc_endpoint_access"
}

func (r *resourceAuthorizeVpcEndpointAccess) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Required: true, PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			names.AttrDomainName: schema.StringAttribute{
				Required: true,
			},
			"authorized_principal": schema.ListAttribute{
				CustomType: fwtypes.NewListNestedObjectTypeOf[authorizedPrincipalData](ctx),
				Computed:   true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *resourceAuthorizeVpcEndpointAccess) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	conn := r.Meta().OpenSearchClient(ctx)

	var plan resourceAuthorizeVpcEndpointAccessData
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &opensearch.AuthorizeVpcEndpointAccessInput{
		Account:    plan.Account.ValueStringPointer(),
		DomainName: plan.DomainName.ValueStringPointer(),
	}

	resp.Diagnostics.Append(flex.Expand(ctx, plan, in)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := conn.AuthorizeVpcEndpointAccess(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.OpenSearch, create.ErrActionCreating, ResNameAuthorizeVpcEndpointAccess, plan.DomainName.String(), err),
			err.Error(),
		)
		return
	}

	if out == nil || out.AuthorizedPrincipal == nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.OpenSearch, create.ErrActionCreating, ResNameAuthorizeVpcEndpointAccess, plan.DomainName.String(), nil),
			errors.New("empty output").Error(),
		)
		return
	}

	resp.Diagnostics.Append(fwflex.Flatten(ctx, out, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *resourceAuthorizeVpcEndpointAccess) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	conn := r.Meta().OpenSearchClient(ctx)

	var state resourceAuthorizeVpcEndpointAccessData
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := findAuthorizeVpcEndpointAccessByName(ctx, conn, state.DomainName.ValueString())
	if tfresource.NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.OpenSearch, create.ErrActionSetting, ResNameAuthorizeVpcEndpointAccess, state.DomainName.String(), err),
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(fwflex.Flatten(ctx, out, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *resourceAuthorizeVpcEndpointAccess) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	conn := r.Meta().OpenSearchClient(ctx)

	var state resourceAuthorizeVpcEndpointAccessData
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := &opensearch.RevokeVpcEndpointAccessInput{
		Account:    aws.String(state.Account.ValueString()),
		DomainName: aws.String(state.DomainName.ValueString()),
	}

	_, err := conn.RevokeVpcEndpointAccess(ctx, in)
	if err != nil {
		if errs.IsA[*awstypes.ResourceNotFoundException](err) {
			return
		}
		resp.Diagnostics.AddError(
			create.ProblemStandardMessage(names.OpenSearch, create.ErrActionDeleting, ResNameAuthorizeVpcEndpointAccess, state.DomainName.String(), err),
			err.Error(),
		)
		return
	}
}

func (r *resourceAuthorizeVpcEndpointAccess) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root(names.AttrDomainName), req, resp)
}

func findAuthorizeVpcEndpointAccessByName(ctx context.Context, conn *opensearch.Client, domainName string) (*awstypes.AuthorizedPrincipal, error) {
	in := &opensearch.ListVpcEndpointAccessInput{
		DomainName: aws.String(domainName),
	}

	return findAuthorizeVpcEndpointAccess(ctx, conn, in)
}

func findAuthorizeVpcEndpointAccess(ctx context.Context, conn *opensearch.Client, input *opensearch.ListVpcEndpointAccessInput) (*awstypes.AuthorizedPrincipal, error) {
	output, err := findAuthorizeVpcEndpointAccesses(ctx, conn, input)

	if err != nil {
		return nil, err
	}

	return tfresource.AssertSingleValueResult(output)
}

func findAuthorizeVpcEndpointAccesses(ctx context.Context, conn *opensearch.Client, input *opensearch.ListVpcEndpointAccessInput) ([]awstypes.AuthorizedPrincipal, error) {
	var output []awstypes.AuthorizedPrincipal

	err := listVPCEndpointAccessPages(ctx, conn, input, func(page *opensearch.ListVpcEndpointAccessOutput, lastPage bool) bool {
		if page == nil {
			return !lastPage
		}

		output = append(output, page.AuthorizedPrincipalList...)

		return !lastPage
	})

	if err != nil {
		return nil, err
	}

	return output, nil
}

type resourceAuthorizeVpcEndpointAccessData struct {
	Account             types.String                                             `tfsdk:"account"`
	DomainName          types.String                                             `tfsdk:"domain_name"`
	AuthorizedPrincipal fwtypes.ListNestedObjectValueOf[authorizedPrincipalData] `tfsdk:"authorized_principal"`
}

type authorizedPrincipalData struct {
	Principal     types.String `tfsdk:"principal"`
	PrincipalType types.String `tfsdk:"principal_type"`
}
