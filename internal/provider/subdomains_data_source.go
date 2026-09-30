// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var _ datasource.DataSourceWithConfigure = (*subdomainsDataSource)(nil)

func NewSubdomainsDataSource() datasource.DataSource {
	return &subdomainsDataSource{}
}

type subdomainsDataSource struct {
	client *kasapi.Client
}

type subdomainsModel struct {
	Subdomains []subdomainsEntryModel `tfsdk:"subdomains"`
}

type subdomainsEntryModel struct {
	Fqdn           types.String `tfsdk:"fqdn"`
	Path           types.String `tfsdk:"path"`
	RedirectStatus types.Int64  `tfsdk:"redirect_status"`
	PHPVersion     types.String `tfsdk:"php_version"`
	Active         types.Bool   `tfsdk:"active"`
	TLS            types.Object `tfsdk:"tls"`
}

func (d *subdomainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subdomains"
}

func (d *subdomainsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads all subdomains of the KAS account with their host settings.",
		Attributes: map[string]schema.Attribute{
			"subdomains": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every subdomain of the account.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"fqdn":            schema.StringAttribute{Computed: true, MarkdownDescription: "Full host name."},
						"path":            schema.StringAttribute{Computed: true, MarkdownDescription: "Document root path, or the redirect target."},
						"redirect_status": schema.Int64Attribute{Computed: true, MarkdownDescription: "Redirect status: 0, 301, 302 or 307."},
						"php_version":     schema.StringAttribute{Computed: true, MarkdownDescription: "PHP version the host runs."},
						"active":          schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the host is served."},
						"tls":             hostTLSDataAttribute(),
					},
				},
			},
		},
	}
}

func (d *subdomainsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*kasapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected data source configure type",
			fmt.Sprintf("Expected *kasapi.Client, got: %T", req.ProviderData))
		return
	}
	d.client = client
}

func (d *subdomainsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data subdomainsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := d.client.Subdomains.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list subdomains", err.Error())
		return
	}

	data.Subdomains = make([]subdomainsEntryModel, 0, len(list))
	for _, it := range list {
		data.Subdomains = append(data.Subdomains, subdomainsEntryModel{
			Fqdn:           types.StringValue(it.FQDN),
			Path:           types.StringValue(it.Path),
			RedirectStatus: types.Int64Value(int64(it.RedirectStatus)),
			PHPVersion:     types.StringValue(it.PHPVersion),
			Active:         types.BoolValue(it.Active),
			TLS:            tlsObject(ctx, it.TLS, &resp.Diagnostics),
		})
	}
	tflog.Debug(ctx, "read subdomains", map[string]any{"count": len(data.Subdomains)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
