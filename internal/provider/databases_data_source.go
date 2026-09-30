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

var _ datasource.DataSourceWithConfigure = (*databasesDataSource)(nil)

func NewDatabasesDataSource() datasource.DataSource {
	return &databasesDataSource{}
}

type databasesDataSource struct {
	client *kasapi.Client
}

type databasesModel struct {
	Databases []databasesEntryModel `tfsdk:"databases"`
}

type databasesEntryModel struct {
	Login        types.String `tfsdk:"login"`
	Name         types.String `tfsdk:"name"`
	Comment      types.String `tfsdk:"comment"`
	AllowedHosts types.List   `tfsdk:"allowed_hosts"`
	UsedSpace    types.Int64  `tfsdk:"used_space"`
}

func (d *databasesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_databases"
}

func (d *databasesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads all MySQL databases of the KAS account.",
		Attributes: map[string]schema.Attribute{
			"databases": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every database of the account.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"login":         schema.StringAttribute{Computed: true, MarkdownDescription: "Database user; the same as the name."},
						"name":          schema.StringAttribute{Computed: true, MarkdownDescription: "Database name."},
						"comment":       schema.StringAttribute{Computed: true, MarkdownDescription: "Free text describing the database."},
						"allowed_hosts": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Hosts allowed to connect from outside."},
						"used_space":    schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the database as KAS reports it."},
					},
				},
			},
		},
	}
}

func (d *databasesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *databasesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data databasesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := d.client.Databases.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list databases", err.Error())
		return
	}

	data.Databases = make([]databasesEntryModel, 0, len(list))
	for _, it := range list {
		data.Databases = append(data.Databases, databasesEntryModel{
			Login:        types.StringValue(it.Login),
			Name:         types.StringValue(it.Name),
			Comment:      types.StringValue(it.Comment),
			AllowedHosts: stringsToList(ctx, it.AllowedHosts, &resp.Diagnostics),
			UsedSpace:    types.Int64Value(int64(it.UsedSpace)),
		})
	}
	tflog.Debug(ctx, "read databases", map[string]any{"count": len(data.Databases)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
