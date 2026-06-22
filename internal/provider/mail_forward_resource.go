// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var (
	_ resource.Resource                = (*mailForwardResource)(nil)
	_ resource.ResourceWithConfigure   = (*mailForwardResource)(nil)
	_ resource.ResourceWithImportState = (*mailForwardResource)(nil)
)

func NewMailForwardResource() resource.Resource {
	return &mailForwardResource{}
}

type mailForwardResource struct {
	client *kasapi.Client
}

type mailForwardModel struct {
	ID        types.String `tfsdk:"id"`
	LocalPart types.String `tfsdk:"local_part"`
	Domain    types.String `tfsdk:"domain"`
	Targets   types.List   `tfsdk:"targets"`
}

func (r *mailForwardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_forward"
}

func (r *mailForwardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a mail forward (redirect) at all-inkl.com (KAS). The source address must not collide with an existing mailbox.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The source address (`local_part@domain`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"local_part": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Part of the source address before the `@`. Changing it forces a new forward.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Domain part of the source address. Changing it forces a new forward.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"targets": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Destination addresses (1-10).",
				Validators: []validator.List{
					listvalidator.SizeBetween(1, 10),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(3)),
				},
			},
		},
	}
}

func (r *mailForwardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*kasapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected resource configure type",
			fmt.Sprintf("Expected *kasapi.Client, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *mailForwardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mailForwardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	targets := listToStrings(ctx, plan.Targets, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	fw := kasapi.MailForward{
		LocalPart: plan.LocalPart.ValueString(),
		Domain:    plan.Domain.ValueString(),
		Targets:   targets,
	}
	if err := r.client.Mail.CreateForward(ctx, fw); err != nil {
		resp.Diagnostics.AddError("Failed to create mail forward", err.Error())
		return
	}

	plan.ID = types.StringValue(fw.Source())
	tflog.Debug(ctx, "created mail forward", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailForwardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mailForwardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "reading mail forward", map[string]any{"id": state.ID.ValueString()})

	fw, err := r.client.Mail.GetForward(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read mail forward", err.Error())
		return
	}

	state.LocalPart = types.StringValue(fw.LocalPart)
	state.Domain = types.StringValue(fw.Domain)
	state.Targets = stringsToList(ctx, fw.Targets, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mailForwardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state mailForwardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	targets := listToStrings(ctx, plan.Targets, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Mail.UpdateForward(ctx, kasapi.MailForward{
		LocalPart: plan.LocalPart.ValueString(),
		Domain:    plan.Domain.ValueString(),
		Targets:   targets,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update mail forward", err.Error())
		return
	}

	plan.ID = state.ID
	tflog.Debug(ctx, "updated mail forward", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailForwardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mailForwardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Mail.DeleteForward(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete mail forward", err.Error())
		return
	}
	tflog.Debug(ctx, "deleted mail forward", map[string]any{"id": state.ID.ValueString()})
}

// ImportState supports `terraform import allinkl_mail_forward.sales sales@example.com`.
func (r *mailForwardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	lp, dom, ok := strings.Cut(req.ID, "@")
	if !ok || lp == "" || dom == "" {
		resp.Diagnostics.AddError("Invalid import id",
			"Expected the source address as import id, e.g. sales@example.com.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("local_part"), lp)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), dom)...)
}
