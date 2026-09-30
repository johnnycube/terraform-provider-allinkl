// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/johnnycube/kasapi"
)

// Domains and subdomains share their host settings and TLS state. The
// attributes, the TLS object and the conversions live here for both.

const (
	descRedirect = "Redirect status: `0` (no redirect), `301`, `302` or `307`. With a redirect, `path` holds the target URL."
	descPHP      = "PHP version the host runs, e.g. `8.4`. Left unset, KAS chooses; the API documents 7.1 as its default."
	descActive   = "Whether the host is served."
	descTLS      = "TLS state of the host, as KAS reports it. Managed with `allinkl_tls_certificate`."
)

var tlsAttrTypes = map[string]attr.Type{
	"active":       types.BoolType,
	"type":         types.StringType,
	"lets_encrypt": types.BoolType,
	"force_https":  types.BoolType,
	"hsts_max_age": types.Int64Type,
}

// tlsObject converts the TLS state of a host into the `tls` attribute value.
func tlsObject(ctx context.Context, t kasapi.HostTLS, diags *diag.Diagnostics) types.Object {
	obj, d := types.ObjectValueFrom(ctx, tlsAttrTypes, struct {
		Active      bool   `tfsdk:"active"`
		Type        string `tfsdk:"type"`
		LetsEncrypt bool   `tfsdk:"lets_encrypt"`
		ForceHTTPS  bool   `tfsdk:"force_https"`
		HSTSMaxAge  int64  `tfsdk:"hsts_max_age"`
	}{t.Active, t.Type, t.LetsEncrypt(), t.ForceHTTPS, int64(t.HSTSMaxAge)})
	diags.Append(d...)
	return obj
}

// hostSettingsAttributes returns the writable host attributes of a resource.
// Domains take `active`; subdomains take it too, but KAS rejects it on
// create, so the subdomain resource applies it in a second call.
func hostSettingsAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"redirect_status": schema.Int64Attribute{
			Optional:            true,
			Computed:            true,
			Default:             int64default.StaticInt64(0),
			MarkdownDescription: descRedirect,
			Validators:          []validator.Int64{int64validator.OneOf(0, 301, 302, 307)},
		},
		"php_version": schema.StringAttribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: descPHP,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"active": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
			MarkdownDescription: descActive,
		},
		"tls": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: descTLS,
			Attributes: map[string]schema.Attribute{
				"active":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the certificate is served."},
				"type":         schema.StringAttribute{Computed: true, MarkdownDescription: "Certificate type as KAS names it; `LE90D` is a Let's Encrypt certificate issued through the panel."},
				"lets_encrypt": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the certificate was issued by Let's Encrypt through the KAS panel."},
				"force_https":  schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether HTTP requests are redirected to HTTPS."},
				"hsts_max_age": schema.Int64Attribute{Computed: true, MarkdownDescription: "HSTS max-age in seconds; `-1` when HSTS is off."},
			},
			PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
		},
	}
}

// hostTLSDataAttribute returns the read-only `tls` attribute of a data source.
func hostTLSDataAttribute() dsschema.Attribute {
	return dsschema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: "TLS state of the host, as KAS reports it.",
		Attributes: map[string]dsschema.Attribute{
			"active":       dsschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the certificate is served."},
			"type":         dsschema.StringAttribute{Computed: true, MarkdownDescription: "Certificate type as KAS names it; `LE90D` is a Let's Encrypt certificate issued through the panel."},
			"lets_encrypt": dsschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the certificate was issued by Let's Encrypt through the KAS panel."},
			"force_https":  dsschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether HTTP requests are redirected to HTTPS."},
			"hsts_max_age": dsschema.Int64Attribute{Computed: true, MarkdownDescription: "HSTS max-age in seconds; `-1` when HSTS is off."},
		},
	}
}

// hostSettingsModel holds the writable host attributes a resource plans.
type hostSettingsModel struct {
	Path           types.String
	RedirectStatus types.Int64
	PHPVersion     types.String
	Active         types.Bool
}

// changes returns the settings of plan that differ from state. A nil state
// yields every set attribute, for a create.
func (plan hostSettingsModel) changes(state *hostSettingsModel) kasapi.HostSettings {
	var hs kasapi.HostSettings
	differs := func(a, b attr.Value) bool { return state == nil || !a.Equal(b) }
	if !plan.Path.IsNull() && !plan.Path.IsUnknown() && differs(plan.Path, stateOr(state).Path) {
		hs.Path = plan.Path.ValueString()
	}
	if !plan.RedirectStatus.IsNull() && !plan.RedirectStatus.IsUnknown() && differs(plan.RedirectStatus, stateOr(state).RedirectStatus) {
		v := int(plan.RedirectStatus.ValueInt64())
		hs.RedirectStatus = &v
		// A redirect target travels in the path parameter, so a redirect
		// change resends the path.
		if hs.Path == "" && !plan.Path.IsNull() && !plan.Path.IsUnknown() {
			hs.Path = plan.Path.ValueString()
		}
	}
	if !plan.PHPVersion.IsNull() && !plan.PHPVersion.IsUnknown() && differs(plan.PHPVersion, stateOr(state).PHPVersion) {
		hs.PHPVersion = plan.PHPVersion.ValueString()
	}
	if !plan.Active.IsNull() && !plan.Active.IsUnknown() && differs(plan.Active, stateOr(state).Active) {
		v := plan.Active.ValueBool()
		hs.Active = &v
	}
	return hs
}

func hostSettingsEmpty(hs kasapi.HostSettings) bool {
	return hs.Path == "" && hs.RedirectStatus == nil && hs.PHPVersion == "" && hs.Active == nil
}

func stateOr(state *hostSettingsModel) hostSettingsModel {
	if state == nil {
		return hostSettingsModel{}
	}
	return *state
}
