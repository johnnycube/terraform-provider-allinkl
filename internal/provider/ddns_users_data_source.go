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

var _ datasource.DataSourceWithConfigure = (*ddnsUsersDataSource)(nil)

func NewDDNSUsersDataSource() datasource.DataSource {
	return &ddnsUsersDataSource{}
}

type ddnsUsersDataSource struct {
	client *kasapi.Client
}

type ddnsUsersModel struct {
	Users []ddnsUsersEntryModel `tfsdk:"users"`
}

type ddnsUsersEntryModel struct {
	Login       types.String `tfsdk:"login"`
	Host        types.String `tfsdk:"host"`
	Zone        types.String `tfsdk:"zone"`
	Label       types.String `tfsdk:"label"`
	Comment     types.String `tfsdk:"comment"`
	CurrentIP   types.String `tfsdk:"current_ip"`
	CurrentIPv6 types.String `tfsdk:"current_ipv6"`
	DualStack   types.Bool   `tfsdk:"dual_stack"`
}

func (d *ddnsUsersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ddns_users"
}

func (d *ddnsUsersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads all dynamic DNS logins of the KAS account.",
		Attributes: map[string]schema.Attribute{
			"users": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every dynamic DNS login of the account.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"login":        schema.StringAttribute{Computed: true, MarkdownDescription: "KAS login, e.g. `dyn0123456`."},
						"host":         schema.StringAttribute{Computed: true, MarkdownDescription: "The host name the login controls."},
						"zone":         schema.StringAttribute{Computed: true, MarkdownDescription: "Zone of the host."},
						"label":        schema.StringAttribute{Computed: true, MarkdownDescription: "Host label inside the zone."},
						"comment":      schema.StringAttribute{Computed: true, MarkdownDescription: "Free text describing the login."},
						"current_ip":   schema.StringAttribute{Computed: true, MarkdownDescription: "IPv4 address the host points at."},
						"current_ipv6": schema.StringAttribute{Computed: true, MarkdownDescription: "IPv6 address the host points at, in dual-stack mode."},
						"dual_stack":   schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the host carries an A and an AAAA record."},
					},
				},
			},
		},
	}
}

func (d *ddnsUsersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ddnsUsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ddnsUsersModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := d.client.DDNS.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list DDNS users", err.Error())
		return
	}

	data.Users = make([]ddnsUsersEntryModel, 0, len(list))
	for _, it := range list {
		data.Users = append(data.Users, ddnsUsersEntryModel{
			Login:       types.StringValue(it.Login),
			Host:        types.StringValue(it.Host()),
			Zone:        types.StringValue(it.Zone),
			Label:       types.StringValue(it.Label),
			Comment:     types.StringValue(it.Comment),
			CurrentIP:   types.StringValue(it.TargetIP),
			CurrentIPv6: types.StringValue(it.TargetIPv6),
			DualStack:   types.BoolValue(it.DualStack),
		})
	}
	tflog.Debug(ctx, "read DDNS users", map[string]any{"count": len(data.Users)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
