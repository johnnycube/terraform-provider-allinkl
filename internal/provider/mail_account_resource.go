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
	_ resource.Resource                = (*mailAccountResource)(nil)
	_ resource.ResourceWithConfigure   = (*mailAccountResource)(nil)
	_ resource.ResourceWithImportState = (*mailAccountResource)(nil)
)

func NewMailAccountResource() resource.Resource {
	return &mailAccountResource{}
}

type mailAccountResource struct {
	client *kasapi.Client
}

type mailAccountModel struct {
	ID            types.String `tfsdk:"id"`
	LocalPart     types.String `tfsdk:"local_part"`
	Domain        types.String `tfsdk:"domain"`
	Password      types.String `tfsdk:"password"`
	CopyAddresses types.List   `tfsdk:"copy_addresses"`
	Address       types.String `tfsdk:"address"`
}

func (r *mailAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_account"
}

func (r *mailAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a mailbox hosted at all-inkl.com (KAS).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "KAS-assigned mail login, e.g. `m1234567`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"local_part": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Part of the address before the `@`. Changing it forces a new mailbox.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Domain part of the address. Must be hosted in the KAS account. Changing it forces a new mailbox.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "Mailbox password. Write-only: the KAS API never returns passwords, so " +
					"drift in the password is not detectable; changing the value here updates it. " +
					"Prefer passing it via a variable with `sensitive = true` and note that, as with " +
					"all Terraform secrets, the value is stored in the state file — protect your state accordingly.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(8),
				},
			},
			"copy_addresses": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Addresses that receive a copy of every incoming mail.",
				Validators: []validator.List{
					listvalidator.SizeAtMost(10),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(3)),
				},
			},
			"address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full primary address (`local_part@domain`).",
			},
		},
	}
}

func (r *mailAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *mailAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mailAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	copies := listToStrings(ctx, plan.CopyAddresses, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	address := plan.LocalPart.ValueString() + "@" + plan.Domain.ValueString()

	// Terraform only knows about objects in its state, so a mailbox that already
	// exists at KAS but is unmanaged would fail at create with a raw fault.
	// Detect that case and point the user at import instead. Best-effort: if the
	// lookup fails we fall through and let Create surface the real error.
	if existing, err := r.client.Mail.ListAccounts(ctx); err == nil {
		for _, acc := range existing {
			if acc.Address() == address {
				resp.Diagnostics.AddError(
					"Mail account already exists",
					fmt.Sprintf("A mailbox for %s already exists at KAS (login %s). Import it "+
						"instead of creating it:\n\n  terraform import allinkl_mail_account.<name> %s",
						address, acc.Login, acc.Login),
				)
				return
			}
		}
	}

	login, err := r.client.Mail.CreateAccount(ctx, kasapi.MailAccount{
		LocalPart:     plan.LocalPart.ValueString(),
		Domain:        plan.Domain.ValueString(),
		CopyAddresses: copies,
	}, plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create mail account", err.Error())
		return
	}

	plan.ID = types.StringValue(login)
	plan.Address = types.StringValue(address)
	tflog.Debug(ctx, "created mail account", map[string]any{"id": login, "address": address})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mailAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	acc, err := r.client.Mail.GetAccount(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read mail account", err.Error())
		return
	}

	state.LocalPart = types.StringValue(acc.LocalPart)
	state.Domain = types.StringValue(acc.Domain)
	state.Address = types.StringValue(acc.Address())
	if len(acc.CopyAddresses) > 0 || !state.CopyAddresses.IsNull() {
		state.CopyAddresses = stringsToList(ctx, acc.CopyAddresses, &resp.Diagnostics)
	}
	// password is write-only: keep whatever is in state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mailAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state mailAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	login := state.ID.ValueString()

	if !plan.Password.Equal(state.Password) {
		if err := r.client.Mail.UpdatePassword(ctx, login, plan.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update mailbox password", err.Error())
			return
		}
	}

	if !plan.CopyAddresses.Equal(state.CopyAddresses) {
		copies := listToStrings(ctx, plan.CopyAddresses, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.client.Mail.UpdateCopyAddresses(ctx, login, copies); err != nil {
			resp.Diagnostics.AddError("Failed to update copy addresses", err.Error())
			return
		}
	}

	plan.ID = state.ID
	plan.Address = types.StringValue(plan.LocalPart.ValueString() + "@" + plan.Domain.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mailAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mailAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Mail.DeleteAccount(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete mail account", err.Error())
	}
}

// ImportState supports `terraform import allinkl_mail_account.info m1234567`.
// After import the next apply sets the password to the configured value,
// because the API cannot read existing passwords.
func (r *mailAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
