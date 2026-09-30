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

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var (
	_ resource.Resource                = (*ddnsUserResource)(nil)
	_ resource.ResourceWithConfigure   = (*ddnsUserResource)(nil)
	_ resource.ResourceWithImportState = (*ddnsUserResource)(nil)
)

func NewDDNSUserResource() resource.Resource {
	return &ddnsUserResource{}
}

type ddnsUserResource struct {
	client *kasapi.Client
}

type ddnsUserModel struct {
	ID          types.String `tfsdk:"id"`
	Zone        types.String `tfsdk:"zone"`
	Label       types.String `tfsdk:"label"`
	Host        types.String `tfsdk:"host"`
	Comment     types.String `tfsdk:"comment"`
	Password    types.String `tfsdk:"password"`
	TargetIP    types.String `tfsdk:"target_ip"`
	DualStack   types.Bool   `tfsdk:"dual_stack"`
	CurrentIP   types.String `tfsdk:"current_ip"`
	CurrentIPv6 types.String `tfsdk:"current_ipv6"`
}

// fill copies the user KAS reports into the model. target_ip is the initial
// address only; the DDNS client moves it afterwards, so it is not refreshed.
func (m *ddnsUserModel) fill(u *kasapi.DDNSUser) {
	m.ID = types.StringValue(u.Login)
	m.Zone, m.Label = types.StringValue(u.Zone), types.StringValue(u.Label)
	m.Host = types.StringValue(u.Host())
	m.Comment = types.StringValue(u.Comment)
	m.DualStack = types.BoolValue(u.DualStack)
	m.CurrentIP, m.CurrentIPv6 = types.StringValue(u.TargetIP), types.StringValue(u.TargetIPv6)
	if m.TargetIP.IsNull() {
		m.TargetIP = types.StringValue(u.TargetIP)
	}
}

func (r *ddnsUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ddns_user"
}

func (r *ddnsUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a dynamic DNS login of the all-inkl.com (KAS) account: one host name that a router " +
			"or script keeps pointed at its current address with the login and password.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "KAS-assigned login, e.g. `dyn0123456`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"zone": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone of the host, e.g. `example.com`. Changing it forces a new login.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(3)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"label": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Host label inside the zone, e.g. `home`. Changing it forces a new login.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"host": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The full host name, `label.zone`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"comment": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Free text describing the login. KAS requires one.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "Password the DDNS client authenticates with. Write-only: drift is not detectable; " +
					"changing the value updates it. As with all Terraform secrets, the value is stored in the state file.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(8)},
			},
			"target_ip": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Initial IPv4 address of the host. The DDNS client moves it afterwards, so the " +
					"attribute is not refreshed from KAS; `current_ip` holds the live value. Changing it forces a new login.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"dual_stack": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Let the host carry an A and an AAAA record.",
			},
			"current_ip": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IPv4 address the host points at now.",
			},
			"current_ipv6": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IPv6 address the host points at now, in dual-stack mode.",
			},
		},
	}
}

func (r *ddnsUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ddnsUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ddnsUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	login, err := r.client.DDNS.Create(ctx, kasapi.DDNSUser{
		Zone: plan.Zone.ValueString(), Label: plan.Label.ValueString(), Comment: plan.Comment.ValueString(),
		TargetIP: plan.TargetIP.ValueString(), DualStack: plan.DualStack.ValueBool(),
	}, plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create DDNS user", err.Error())
		return
	}
	tflog.Debug(ctx, "created DDNS user", map[string]any{"id": login})
	u, err := r.client.DDNS.Get(ctx, login)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read DDNS user after creation", err.Error())
		return
	}
	plan.fill(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ddnsUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ddnsUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	u, err := r.client.DDNS.Get(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read DDNS user", err.Error())
		return
	}
	state.fill(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ddnsUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ddnsUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	login := state.ID.ValueString()
	plan.ID, plan.Host = state.ID, state.Host
	plan.CurrentIP, plan.CurrentIPv6 = state.CurrentIP, state.CurrentIPv6

	if !plan.Comment.Equal(state.Comment) || !plan.DualStack.Equal(state.DualStack) {
		err := r.client.DDNS.Update(ctx, kasapi.DDNSUser{Login: login, Comment: plan.Comment.ValueString(), DualStack: plan.DualStack.ValueBool()})
		if err != nil {
			resp.Diagnostics.AddError("Failed to update DDNS user", err.Error())
			return
		}
	}
	if !plan.Password.Equal(state.Password) {
		if err := r.client.DDNS.UpdatePassword(ctx, login, plan.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update DDNS password", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ddnsUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ddnsUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DDNS.Delete(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete DDNS user", err.Error())
	}
}

// ImportState supports `terraform import allinkl_ddns_user.home dyn0123456`.
// The next apply sets the password to the configured value, because the API
// cannot read the existing one.
func (r *ddnsUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
