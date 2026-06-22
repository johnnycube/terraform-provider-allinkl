// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

// Ensure the implementation satisfies the framework interfaces.
var _ provider.Provider = (*allinklProvider)(nil)

type allinklProvider struct {
	version string
}

type allinklProviderModel struct {
	Login           types.String `tfsdk:"login"`
	Password        types.String `tfsdk:"password"`
	AuthType        types.String `tfsdk:"auth_type"`
	SessionLifetime types.Int64  `tfsdk:"session_lifetime"`
}

// New returns the provider factory used by main.go and acceptance tests.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &allinklProvider{version: version}
	}
}

func (p *allinklProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "allinkl"
	resp.Version = p.version
}

func (p *allinklProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage all-inkl.com (KAS) resources via the KAS SOAP API. " +
			"Unofficial, community-maintained provider; not affiliated with, endorsed by, " +
			"or supported by all-inkl.com (Neue Medien Münnich GmbH).",
		Attributes: map[string]schema.Attribute{
			"login": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "KAS login (e.g. `w0123456`). Can also be set via the `KAS_LOGIN` environment variable.",
			},
			"password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "KAS account or API password. Can also be set via the `KAS_PASSWORD` environment variable.",
			},
			"auth_type": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "How credentials are sent when creating the API session: `sha1` (default) or `plain`. Can also be set via `KAS_AUTH_TYPE`.",
				Validators: []validator.String{
					stringvalidator.OneOf(string(kasapi.AuthSHA1), string(kasapi.AuthPlain)),
				},
			},
			"session_lifetime": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "API session lifetime in seconds (max 3600, default 1800).",
				Validators: []validator.Int64{
					int64validator.Between(0, 3600),
				},
			},
		},
	}
}

func (p *allinklProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data allinklProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	login := firstNonEmpty(data.Login.ValueString(), os.Getenv("KAS_LOGIN"))
	password := firstNonEmpty(data.Password.ValueString(), os.Getenv("KAS_PASSWORD"))
	authType := firstNonEmpty(data.AuthType.ValueString(), os.Getenv("KAS_AUTH_TYPE"), string(kasapi.AuthSHA1))

	if login == "" {
		resp.Diagnostics.AddAttributeError(path.Root("login"),
			"Missing KAS login",
			"Set the `login` provider attribute or the KAS_LOGIN environment variable.")
	}
	if password == "" {
		resp.Diagnostics.AddAttributeError(path.Root("password"),
			"Missing KAS password",
			"Set the `password` provider attribute or the KAS_PASSWORD environment variable.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	sessionLifetime := int(data.SessionLifetime.ValueInt64())
	if sessionLifetime == 0 {
		if v := os.Getenv("KAS_SESSION_LIFETIME"); v != "" {
			sessionLifetime, _ = strconv.Atoi(v)
		}
	}

	client, err := kasapi.New(kasapi.Config{
		Login:           login,
		Password:        password,
		AuthType:        kasapi.AuthType(authType),
		SessionLifetime: sessionLifetime,
		UserAgent:       "terraform-provider-allinkl/" + p.version,
		// Endpoint overrides are intentionally env-only (not schema
		// attributes): they exist for acceptance tests against the fake KAS
		// server, and keeping them out of the schema prevents configs from
		// silently redirecting credentials to a third-party host.
		APIEndpoint:  os.Getenv("KAS_API_ENDPOINT"),
		AuthEndpoint: os.Getenv("KAS_AUTH_ENDPOINT"),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create KAS API client", err.Error())
		return
	}

	// One shared client for all resources and data sources.
	resp.ResourceData = client
	resp.DataSourceData = client

	tflog.Debug(ctx, "configured KAS API client", map[string]any{
		"login":     login,
		"auth_type": authType,
	})
}

func (p *allinklProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDNSRecordResource,
		NewMailAccountResource,
		NewMailForwardResource,
		NewSubdomainResource,
		// Future use cases register here, each backed by its own kasapi
		// service, e.g.:
		// NewFTPUserResource,
		// NewDatabaseResource,
	}
}

func (p *allinklProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDNSRecordsDataSource,
		NewDomainsDataSource,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
