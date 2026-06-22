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

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var (
	_ resource.Resource                = (*subdomainResource)(nil)
	_ resource.ResourceWithConfigure   = (*subdomainResource)(nil)
	_ resource.ResourceWithImportState = (*subdomainResource)(nil)
)

func NewSubdomainResource() resource.Resource {
	return &subdomainResource{}
}

type subdomainResource struct {
	client *kasapi.Client
}

type subdomainModel struct {
	ID     types.String `tfsdk:"id"`
	Name   types.String `tfsdk:"name"`
	Domain types.String `tfsdk:"domain"`
	Path   types.String `tfsdk:"path"`
}

func (r *subdomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subdomain"
}

func (r *subdomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a subdomain at all-inkl.com (KAS).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full host name (`name.domain`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Subdomain label, e.g. `blog`. Changing it forces a new subdomain.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Parent domain hosted in the KAS account. Changing it forces a new subdomain.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"path": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("/"),
				MarkdownDescription: "Document root path relative to the account root, e.g. `/blog/`.",
			},
		},
	}
}

func (r *subdomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *subdomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan subdomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Subdomains.Create(ctx,
		plan.Name.ValueString(), plan.Domain.ValueString(), plan.Path.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create subdomain", err.Error())
		return
	}

	plan.ID = types.StringValue(plan.Name.ValueString() + "." + plan.Domain.ValueString())
	tflog.Debug(ctx, "created subdomain", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *subdomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state subdomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "reading subdomain", map[string]any{"id": state.ID.ValueString()})

	sub, err := r.client.Subdomains.Get(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read subdomain", err.Error())
		return
	}

	if sub.Path != "" {
		state.Path = types.StringValue(sub.Path)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *subdomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state subdomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Path.Equal(state.Path) {
		if err := r.client.Subdomains.UpdatePath(ctx, state.ID.ValueString(), plan.Path.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update subdomain path", err.Error())
			return
		}
	}

	plan.ID = state.ID
	tflog.Debug(ctx, "updated subdomain", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *subdomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state subdomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Subdomains.Delete(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete subdomain", err.Error())
		return
	}
	tflog.Debug(ctx, "deleted subdomain", map[string]any{"id": state.ID.ValueString()})
}

// ImportState supports `terraform import allinkl_subdomain.blog blog.example.com`.
// The first label becomes `name`, the rest `domain`.
func (r *subdomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	name, domain, ok := strings.Cut(req.ID, ".")
	if !ok || name == "" || domain == "" {
		resp.Diagnostics.AddError("Invalid import id",
			"Expected the full host name as import id, e.g. blog.example.com.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), domain)...)
}
