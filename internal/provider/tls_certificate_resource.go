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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var (
	_ resource.Resource                = (*tlsCertificateResource)(nil)
	_ resource.ResourceWithConfigure   = (*tlsCertificateResource)(nil)
	_ resource.ResourceWithImportState = (*tlsCertificateResource)(nil)
)

func NewTLSCertificateResource() resource.Resource {
	return &tlsCertificateResource{}
}

type tlsCertificateResource struct {
	client *kasapi.Client
}

type tlsCertificateModel struct {
	ID          types.String `tfsdk:"id"`
	Host        types.String `tfsdk:"host"`
	Certificate types.String `tfsdk:"certificate"`
	PrivateKey  types.String `tfsdk:"private_key"`
	Bundle      types.String `tfsdk:"bundle"`
	CSR         types.String `tfsdk:"csr"`
	Active      types.Bool   `tfsdk:"active"`
	ForceHTTPS  types.Bool   `tfsdk:"force_https"`
	HSTSMaxAge  types.Int64  `tfsdk:"hsts_max_age"`
	Type        types.String `tfsdk:"type"`
	LetsEncrypt types.Bool   `tfsdk:"lets_encrypt"`
}

// fill copies the state KAS reports into the model. PEM material is kept as
// configured when it matches KAS modulo whitespace, so formatting does not
// show up as drift.
func (m *tlsCertificateModel) fill(t *kasapi.HostTLS) {
	if !samePEM(m.Certificate.ValueString(), t.Certificate) {
		m.Certificate = types.StringValue(t.Certificate)
	}
	if !samePEM(m.Bundle.ValueString(), t.Bundle) {
		if t.Bundle == "" {
			m.Bundle = types.StringNull()
		} else {
			m.Bundle = types.StringValue(t.Bundle)
		}
	}
	m.Active = types.BoolValue(t.Active)
	m.ForceHTTPS = types.BoolValue(t.ForceHTTPS)
	m.HSTSMaxAge = types.Int64Value(int64(t.HSTSMaxAge))
	m.Type = types.StringValue(t.Type)
	m.LetsEncrypt = types.BoolValue(t.LetsEncrypt())
}

// pemBlockRe matches a PEM block of the given type, with an optional
// algorithm prefix such as "EC PRIVATE KEY".
func pemBlockRe(block string) *regexp.Regexp {
	return regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z]+ )*` + block + `-----.*-----END (?:[A-Z]+ )*` + block + `-----`)
}

func samePEM(a, b string) bool {
	norm := func(s string) string { return strings.Join(strings.Fields(s), "") }
	return norm(a) == norm(b)
}

func (r *tlsCertificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tls_certificate"
}

func (r *tlsCertificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	pem := func(block string) validator.String {
		return stringvalidator.RegexMatches(pemBlockRe(block), "must be PEM-encoded ("+block+")")
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Installs a TLS certificate on a domain or subdomain hosted at all-inkl.com (KAS) and sets " +
			"the HTTPS redirect and HSTS. The certificate is one you bring, for example from an ACME client that " +
			"solved the DNS-01 challenge with `allinkl_dns_record`. The KAS API cannot request a Let's Encrypt " +
			"certificate; that switch exists in the KAS panel only. Destroying the resource deactivates the " +
			"certificate; KAS keeps it stored.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The host name.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"host": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Domain or subdomain the certificate is installed on. Changing it forces a new resource.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(3)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"certificate": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "PEM-encoded certificate. It must name the host.",
				Validators:          []validator.String{pem("CERTIFICATE")},
			},
			"private_key": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "PEM-encoded private key of the certificate. Write-only: KAS never returns it, so drift " +
					"is not detectable; changing the value re-installs the pair. As with all Terraform secrets, " +
					"the value is stored in the state file.",
				Validators: []validator.String{pem("PRIVATE KEY")},
			},
			"bundle": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "PEM-encoded intermediate certificates.",
				Validators:          []validator.String{pem("CERTIFICATE")},
			},
			"csr": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "PEM-encoded signing request. KAS stores it; it does not affect the served certificate.",
				Validators:          []validator.String{pem("CERTIFICATE REQUEST")},
			},
			"active": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the certificate is served.",
			},
			"force_https": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Redirect HTTP requests to HTTPS with a 301.",
			},
			"hsts_max_age": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(-1),
				MarkdownDescription: "Strict-Transport-Security max-age in seconds; `-1` turns HSTS off.",
				Validators:          []validator.Int64{int64validator.AtLeast(-1)},
			},
			"type": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Certificate type as KAS names it.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"lets_encrypt": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether KAS reports a Let's Encrypt certificate issued through the panel. `true` means the panel took over the host.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *tlsCertificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// update sends u and refreshes the model from KAS.
func (r *tlsCertificateResource) update(ctx context.Context, m *tlsCertificateModel, u kasapi.TLSUpdate, diags *diag.Diagnostics) {
	host := m.Host.ValueString()
	if err := r.client.TLS.Update(ctx, host, u); err != nil {
		diags.AddError("Failed to update TLS certificate", err.Error())
		return
	}
	t, err := r.client.TLS.Get(ctx, host)
	if err != nil {
		diags.AddError("Failed to read TLS state", err.Error())
		return
	}
	m.ID = types.StringValue(host)
	m.fill(t)
}

func (m tlsCertificateModel) fullUpdate() kasapi.TLSUpdate {
	active, force, hsts := m.Active.ValueBool(), m.ForceHTTPS.ValueBool(), int(m.HSTSMaxAge.ValueInt64())
	return kasapi.TLSUpdate{
		Certificate: m.Certificate.ValueString(),
		Key:         m.PrivateKey.ValueString(),
		Bundle:      m.Bundle.ValueString(),
		CSR:         m.CSR.ValueString(),
		Active:      &active,
		ForceHTTPS:  &force,
		HSTSMaxAge:  &hsts,
	}
}

func (r *tlsCertificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tlsCertificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.update(ctx, &plan, plan.fullUpdate(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, "installed TLS certificate", map[string]any{"host": plan.Host.ValueString()})
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *tlsCertificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tlsCertificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.TLS.Get(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read TLS state", err.Error())
		return
	}
	state.Host = state.ID
	state.fill(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *tlsCertificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state tlsCertificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Certificate and key travel together; settings go alone when the
	// material is unchanged.
	var u kasapi.TLSUpdate
	if !samePEM(plan.Certificate.ValueString(), state.Certificate.ValueString()) || !plan.PrivateKey.Equal(state.PrivateKey) {
		u.Certificate, u.Key = plan.Certificate.ValueString(), plan.PrivateKey.ValueString()
	}
	if !samePEM(plan.Bundle.ValueString(), state.Bundle.ValueString()) {
		u.Bundle = plan.Bundle.ValueString()
	}
	if !plan.CSR.Equal(state.CSR) {
		u.CSR = plan.CSR.ValueString()
	}
	if !plan.Active.Equal(state.Active) {
		v := plan.Active.ValueBool()
		u.Active = &v
	}
	if !plan.ForceHTTPS.Equal(state.ForceHTTPS) {
		v := plan.ForceHTTPS.ValueBool()
		u.ForceHTTPS = &v
	}
	if !plan.HSTSMaxAge.Equal(state.HSTSMaxAge) {
		v := int(plan.HSTSMaxAge.ValueInt64())
		u.HSTSMaxAge = &v
	}
	plan.ID = state.ID
	if u == (kasapi.TLSUpdate{}) {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}
	r.update(ctx, &plan, u, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deactivates the certificate. KAS has no call that removes one.
func (r *tlsCertificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state tlsCertificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	off := false
	err := r.client.TLS.Update(ctx, state.ID.ValueString(), kasapi.TLSUpdate{Active: &off})
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to deactivate TLS certificate", err.Error())
		return
	}
	tflog.Debug(ctx, "deactivated TLS certificate", map[string]any{"host": state.ID.ValueString()})
}

// ImportState supports `terraform import allinkl_tls_certificate.www www.example.com`.
// KAS returns the certificate but never the key, so the next apply
// re-installs the pair from the configuration.
func (r *tlsCertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host"), req.ID)...)
}
