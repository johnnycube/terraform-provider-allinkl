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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var (
	_ resource.Resource                = (*ftpUserResource)(nil)
	_ resource.ResourceWithConfigure   = (*ftpUserResource)(nil)
	_ resource.ResourceWithImportState = (*ftpUserResource)(nil)
)

func NewFTPUserResource() resource.Resource {
	return &ftpUserResource{}
}

type ftpUserResource struct {
	client *kasapi.Client
}

type ftpUserModel struct {
	ID        types.String `tfsdk:"id"`
	Path      types.String `tfsdk:"path"`
	Comment   types.String `tfsdk:"comment"`
	Password  types.String `tfsdk:"password"`
	Read      types.Bool   `tfsdk:"read"`
	Write     types.Bool   `tfsdk:"write"`
	List      types.Bool   `tfsdk:"list"`
	VirusScan types.Bool   `tfsdk:"virus_scan"`
}

func (m ftpUserModel) user() kasapi.FTPUser {
	return kasapi.FTPUser{
		Login: m.ID.ValueString(), Path: m.Path.ValueString(), Comment: m.Comment.ValueString(),
		Read: m.Read.ValueBool(), Write: m.Write.ValueBool(), List: m.List.ValueBool(), VirusScan: m.VirusScan.ValueBool(),
	}
}

func (m *ftpUserModel) fill(u *kasapi.FTPUser) {
	m.ID = types.StringValue(u.Login)
	if u.Path != "" {
		m.Path = types.StringValue(u.Path)
	}
	m.Comment = types.StringValue(u.Comment)
	m.Read, m.Write, m.List = types.BoolValue(u.Read), types.BoolValue(u.Write), types.BoolValue(u.List)
	m.VirusScan = types.BoolValue(u.VirusScan)
}

func (r *ftpUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ftp_user"
}

func (r *ftpUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	permission := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an additional FTP login of the all-inkl.com (KAS) account. The account's own login is not manageable.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "KAS-assigned login, e.g. `f0123456`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"path": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("/"),
				MarkdownDescription: "Directory the login is confined to, relative to the account root.",
			},
			"comment": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Free text describing the login. KAS requires one.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "FTP password. Write-only: drift is not detectable; changing the value updates it. " +
					"As with all Terraform secrets, the value is stored in the state file.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(8)},
			},
			"read":       permission("May download files."),
			"write":      permission("May upload, change and delete files."),
			"list":       permission("May list directories."),
			"virus_scan": permission("Scan uploads with ClamAV."),
		},
	}
}

func (r *ftpUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ftpUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ftpUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	login, err := r.client.FTP.Create(ctx, plan.user(), plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create FTP user", err.Error())
		return
	}
	plan.ID = types.StringValue(login)
	tflog.Debug(ctx, "created FTP user", map[string]any{"id": login})
	u, err := r.client.FTP.Get(ctx, login)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read FTP user after creation", err.Error())
		return
	}
	plan.fill(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ftpUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ftpUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	u, err := r.client.FTP.Get(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read FTP user", err.Error())
		return
	}
	state.fill(u)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ftpUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ftpUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	if plan.user() != state.user() {
		if err := r.client.FTP.Update(ctx, plan.user()); err != nil {
			resp.Diagnostics.AddError("Failed to update FTP user", err.Error())
			return
		}
	}
	if !plan.Password.Equal(state.Password) {
		if err := r.client.FTP.UpdatePassword(ctx, state.ID.ValueString(), plan.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update FTP password", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ftpUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ftpUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.FTP.Delete(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete FTP user", err.Error())
	}
}

// ImportState supports `terraform import allinkl_ftp_user.logs f0123456`. The
// next apply sets the password to the configured value, because the API
// cannot read the existing one.
func (r *ftpUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
