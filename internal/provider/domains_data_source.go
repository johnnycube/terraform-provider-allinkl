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

var (
	_ datasource.DataSource              = (*domainsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*domainsDataSource)(nil)
)

func NewDomainsDataSource() datasource.DataSource {
	return &domainsDataSource{}
}

type domainsDataSource struct {
	client *kasapi.Client
}

type domainsModel struct {
	Domains []domainEntryModel `tfsdk:"domains"`
}

type domainEntryModel struct {
	Name types.String `tfsdk:"name"`
	Path types.String `tfsdk:"path"`
}

func (d *domainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domains"
}

func (d *domainsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads all domains hosted in the KAS account. Domain management itself is intentionally not exposed as a resource, since add/delete touch registration; use this to reference existing domains.",
		Attributes: map[string]schema.Attribute{
			"domains": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every domain hosted in the account.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Domain name."},
						"path": schema.StringAttribute{Computed: true, MarkdownDescription: "Document root path relative to the account root."},
					},
				},
			},
		},
	}
}

func (d *domainsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *domainsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data domainsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domains, err := d.client.Domains.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list domains", err.Error())
		return
	}

	data.Domains = make([]domainEntryModel, 0, len(domains))
	for _, dom := range domains {
		data.Domains = append(data.Domains, domainEntryModel{
			Name: types.StringValue(dom.Name),
			Path: types.StringValue(dom.Path),
		})
	}
	tflog.Debug(ctx, "read domains", map[string]any{"count": len(data.Domains)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
