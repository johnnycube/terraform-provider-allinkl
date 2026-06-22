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
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var (
	_ resource.Resource                = (*dnsRecordResource)(nil)
	_ resource.ResourceWithConfigure   = (*dnsRecordResource)(nil)
	_ resource.ResourceWithImportState = (*dnsRecordResource)(nil)
)

func NewDNSRecordResource() resource.Resource {
	return &dnsRecordResource{}
}

type dnsRecordResource struct {
	client *kasapi.Client
}

type dnsRecordModel struct {
	ID   types.String `tfsdk:"id"`
	Zone types.String `tfsdk:"zone"`
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
	Data types.String `tfsdk:"data"`
	Aux  types.Int64  `tfsdk:"aux"`
}

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a single DNS record in a zone hosted at all-inkl.com (KAS).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Record id assigned by KAS.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"zone": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone host, e.g. `example.com`. Changing it forces a new record.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Record name relative to the zone (e.g. `www`). Empty string for the zone apex.",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record type: A, AAAA, CNAME, MX, TXT, SRV, NS, CAA, ... The KAS API cannot change the type in place, so changing it forces a new record.",
				Validators: []validator.String{
					// Permissive on purpose: KAS adds types over time. This only
					// rejects clearly malformed input (lowercase is normalized
					// by the API layer).
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,15}$`),
						"must be a DNS record type such as A, AAAA, CNAME, MX, TXT, SRV, CAA",
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"data": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record payload, e.g. an IP address, hostname or TXT content.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"aux": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				MarkdownDescription: "Auxiliary value: MX priority or SRV priority. 0 for other types.",
				Validators: []validator.Int64{
					int64validator.Between(0, 65535),
				},
			},
		},
	}
}

func (r *dnsRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := r.client.DNS.Create(ctx, kasapi.DNSRecord{
		Zone: plan.Zone.ValueString(),
		Name: plan.Name.ValueString(),
		Type: plan.Type.ValueString(),
		Data: plan.Data.ValueString(),
		Aux:  int(plan.Aux.ValueInt64()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to create DNS record", err.Error())
		return
	}

	plan.ID = types.StringValue(id)
	tflog.Debug(ctx, "created DNS record", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "reading DNS record", map[string]any{"id": state.ID.ValueString()})

	rec, err := r.client.DNS.Get(ctx, state.Zone.ValueString(), state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		// Record was deleted out of band: drop it from state.
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read DNS record", err.Error())
		return
	}

	state.Name = types.StringValue(rec.Name)
	state.Type = types.StringValue(rec.Type)
	state.Data = types.StringValue(rec.Data)
	state.Aux = types.Int64Value(int64(rec.Aux))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DNS.Update(ctx, kasapi.DNSRecord{
		ID:   state.ID.ValueString(),
		Zone: plan.Zone.ValueString(),
		Name: plan.Name.ValueString(),
		Data: plan.Data.ValueString(),
		Aux:  int(plan.Aux.ValueInt64()),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to update DNS record", err.Error())
		return
	}

	plan.ID = state.ID
	tflog.Debug(ctx, "updated DNS record", map[string]any{"id": plan.ID.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DNS.Delete(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete DNS record", err.Error())
		return
	}
	tflog.Debug(ctx, "deleted DNS record", map[string]any{"id": state.ID.ValueString()})
}

// ImportState supports `terraform import allinkl_dns_record.www example.com/12345`.
func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import id",
			"Expected import id in the form <zone>/<record_id>, e.g. example.com/12345.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
