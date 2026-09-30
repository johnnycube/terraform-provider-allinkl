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
	_ resource.Resource                = (*domainSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*domainSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*domainSettingsResource)(nil)
)

func NewDomainSettingsResource() resource.Resource {
	return &domainSettingsResource{}
}

type domainSettingsResource struct {
	client *kasapi.Client
}

type domainSettingsModel struct {
	ID             types.String `tfsdk:"id"`
	Domain         types.String `tfsdk:"domain"`
	Path           types.String `tfsdk:"path"`
	RedirectStatus types.Int64  `tfsdk:"redirect_status"`
	PHPVersion     types.String `tfsdk:"php_version"`
	Active         types.Bool   `tfsdk:"active"`
	DKIMSelector   types.String `tfsdk:"dkim_selector"`
	TLS            types.Object `tfsdk:"tls"`
}

func (m domainSettingsModel) settings() hostSettingsModel {
	return hostSettingsModel{Path: m.Path, RedirectStatus: m.RedirectStatus, PHPVersion: m.PHPVersion, Active: m.Active}
}

func (m *domainSettingsModel) fill(ctx context.Context, d *kasapi.Domain, diags *diag.Diagnostics) {
	m.ID = types.StringValue(d.Name)
	m.Domain = types.StringValue(d.Name)
	if d.Path != "" {
		m.Path = types.StringValue(d.Path)
	}
	m.RedirectStatus = types.Int64Value(int64(d.RedirectStatus))
	m.PHPVersion = types.StringValue(d.PHPVersion)
	m.Active = types.BoolValue(d.Active)
	m.DKIMSelector = types.StringValue(d.DKIMSelector)
	m.TLS = tlsObject(ctx, d.TLS, diags)
}

func (r *domainSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_settings"
}

func (r *domainSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := hostSettingsAttributes()
	for k, v := range map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The domain name.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"domain": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "A domain hosted in the KAS account. Changing it forces a new resource.",
			Validators:          []validator.String{stringvalidator.LengthAtLeast(3)},
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"path": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: "Document root path relative to the account root, or the redirect target URL when `redirect_status` is set.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"dkim_selector": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Selector of the DKIM key KAS signs mail with.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
	} {
		attrs[k] = v
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the host settings of a domain that exists in the KAS account: document root or " +
			"redirect, PHP version and activation. It does not register, transfer or delete the domain; " +
			"destroying the resource only forgets the domain and leaves its settings as they are.",
		Attributes: attrs,
	}
}

func (r *domainSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// apply writes the changed settings and refreshes the model from KAS.
func (r *domainSettingsResource) apply(ctx context.Context, m *domainSettingsModel, hs kasapi.HostSettings, diags *diag.Diagnostics) {
	name := m.Domain.ValueString()
	if !hostSettingsEmpty(hs) {
		if err := r.client.Domains.Update(ctx, name, hs); err != nil {
			diags.AddError("Failed to update domain settings", err.Error())
			return
		}
	}
	d, err := r.client.Domains.Get(ctx, name)
	if err != nil {
		diags.AddError("Failed to read domain", err.Error())
		return
	}
	m.fill(ctx, d, diags)
}

func (r *domainSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Domain.ValueString()
	current, err := r.client.Domains.Get(ctx, name)
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddAttributeError(path.Root("domain"), "Domain not found",
			fmt.Sprintf("%s is not hosted in this KAS account. The resource manages settings of existing domains only.", name))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read domain", err.Error())
		return
	}

	// Only the settings that differ from the domain are written; KAS
	// answers an update without change with a fault.
	state := domainSettingsModel{}
	state.fill(ctx, current, &resp.Diagnostics)
	cur := state.settings()
	r.apply(ctx, &plan, plan.settings().changes(&cur), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "adopted domain settings", map[string]any{"domain": name})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *domainSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.Domains.Get(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read domain", err.Error())
		return
	}
	state.fill(ctx, d, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *domainSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state domainSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cur := state.settings()
	r.apply(ctx, &plan, plan.settings().changes(&cur), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete forgets the domain. Its settings stay: the domain is not the
// provider's to remove.
func (r *domainSettingsResource) Delete(ctx context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	tflog.Debug(ctx, "domain settings removed from state; the domain is untouched")
}

// ImportState supports `terraform import allinkl_domain_settings.main example.com`.
func (r *domainSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), req.ID)...)
}
