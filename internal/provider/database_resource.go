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
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                = (*databaseResource)(nil)
	_ resource.ResourceWithConfigure   = (*databaseResource)(nil)
	_ resource.ResourceWithImportState = (*databaseResource)(nil)
)

func NewDatabaseResource() resource.Resource {
	return &databaseResource{}
}

type databaseResource struct {
	client *kasapi.Client
}

type databaseModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Comment      types.String `tfsdk:"comment"`
	Password     types.String `tfsdk:"password"`
	AllowedHosts types.List   `tfsdk:"allowed_hosts"`
}

func (m *databaseModel) fill(ctx context.Context, d *kasapi.Database, diags *diag.Diagnostics) {
	m.ID = types.StringValue(d.Login)
	m.Name = types.StringValue(d.Name)
	m.Comment = types.StringValue(d.Comment)
	// KAS lists localhost on its own; only the external hosts are the user's.
	var hosts []string
	for _, h := range d.AllowedHosts {
		if h != "localhost" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) > 0 || !m.AllowedHosts.IsNull() {
		m.AllowedHosts = stringsToList(ctx, hosts, diags)
	}
}

func (r *databaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *databaseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a MySQL database of the all-inkl.com (KAS) account. KAS assigns name and login; " +
			"they are identical. Destroying the resource deletes the database with its data.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "KAS-assigned login, e.g. `d0123456`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Database name; the same as the login.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"comment": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Free text describing the database. KAS requires one.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "Password of the database user. Write-only: drift is not detectable; changing the " +
					"value updates it. As with all Terraform secrets, the value is stored in the state file.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(8)},
			},
			"allowed_hosts": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Hosts allowed to connect from outside: IP addresses or CIDR networks. Unset keeps the database reachable from the hosting environment only; `localhost`, which KAS lists on its own, is not part of the attribute.",
				Validators:          []validator.List{listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1))},
			},
		},
	}
}

func (r *databaseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *databaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hosts := listToStrings(ctx, plan.AllowedHosts, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	login, err := r.client.Databases.Create(ctx, kasapi.Database{Comment: plan.Comment.ValueString(), AllowedHosts: hosts}, plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create database", err.Error())
		return
	}
	tflog.Debug(ctx, "created database", map[string]any{"id": login})
	d, err := r.client.Databases.Get(ctx, login)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read database after creation", err.Error())
		return
	}
	plan.fill(ctx, d, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.Databases.Get(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read database", err.Error())
		return
	}
	state.fill(ctx, d, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *databaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state databaseModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	login := state.ID.ValueString()
	plan.ID, plan.Name = state.ID, state.Name

	if !plan.Comment.Equal(state.Comment) || !plan.AllowedHosts.Equal(state.AllowedHosts) {
		hosts := listToStrings(ctx, plan.AllowedHosts, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		err := r.client.Databases.Update(ctx, kasapi.Database{Login: login, Comment: plan.Comment.ValueString(), AllowedHosts: hosts})
		if err != nil {
			resp.Diagnostics.AddError("Failed to update database", err.Error())
			return
		}
	}
	if !plan.Password.Equal(state.Password) {
		if err := r.client.Databases.UpdatePassword(ctx, login, plan.Password.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update database password", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Databases.Delete(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete database", err.Error())
	}
}

// ImportState supports `terraform import allinkl_database.shop d0123456`. The
// next apply sets the password to the configured value, because the API
// cannot read the existing one.
func (r *databaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
