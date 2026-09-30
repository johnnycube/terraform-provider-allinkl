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

var _ datasource.DataSourceWithConfigure = (*ftpUsersDataSource)(nil)

func NewFTPUsersDataSource() datasource.DataSource {
	return &ftpUsersDataSource{}
}

type ftpUsersDataSource struct {
	client *kasapi.Client
}

type ftpUsersModel struct {
	Users []ftpUsersEntryModel `tfsdk:"users"`
}

type ftpUsersEntryModel struct {
	Login     types.String `tfsdk:"login"`
	Path      types.String `tfsdk:"path"`
	Comment   types.String `tfsdk:"comment"`
	Read      types.Bool   `tfsdk:"read"`
	Write     types.Bool   `tfsdk:"write"`
	List      types.Bool   `tfsdk:"list"`
	VirusScan types.Bool   `tfsdk:"virus_scan"`
	MainUser  types.Bool   `tfsdk:"main_user"`
}

func (d *ftpUsersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ftp_users"
}

func (d *ftpUsersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads all FTP logins of the KAS account, the account's own login included.",
		Attributes: map[string]schema.Attribute{
			"users": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every FTP login of the account.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"login":      schema.StringAttribute{Computed: true, MarkdownDescription: "KAS login, e.g. `f0123456`."},
						"path":       schema.StringAttribute{Computed: true, MarkdownDescription: "Directory the login is confined to."},
						"comment":    schema.StringAttribute{Computed: true, MarkdownDescription: "Free text describing the login."},
						"read":       schema.BoolAttribute{Computed: true, MarkdownDescription: "May download files."},
						"write":      schema.BoolAttribute{Computed: true, MarkdownDescription: "May upload, change and delete files."},
						"list":       schema.BoolAttribute{Computed: true, MarkdownDescription: "May list directories."},
						"virus_scan": schema.BoolAttribute{Computed: true, MarkdownDescription: "Uploads are scanned with ClamAV."},
						"main_user":  schema.BoolAttribute{Computed: true, MarkdownDescription: "The account's own login, which cannot be deleted."},
					},
				},
			},
		},
	}
}

func (d *ftpUsersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ftpUsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ftpUsersModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := d.client.FTP.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list FTP users", err.Error())
		return
	}

	data.Users = make([]ftpUsersEntryModel, 0, len(list))
	for _, it := range list {
		data.Users = append(data.Users, ftpUsersEntryModel{
			Login:     types.StringValue(it.Login),
			Path:      types.StringValue(it.Path),
			Comment:   types.StringValue(it.Comment),
			Read:      types.BoolValue(it.Read),
			Write:     types.BoolValue(it.Write),
			List:      types.BoolValue(it.List),
			VirusScan: types.BoolValue(it.VirusScan),
			MainUser:  types.BoolValue(it.MainUser),
		})
	}
	tflog.Debug(ctx, "read FTP users", map[string]any{"count": len(data.Users)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
