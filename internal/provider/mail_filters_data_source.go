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

var _ datasource.DataSourceWithConfigure = (*mailFiltersDataSource)(nil)

func NewMailFiltersDataSource() datasource.DataSource {
	return &mailFiltersDataSource{}
}

type mailFiltersDataSource struct {
	client *kasapi.Client
}

type mailFiltersModel struct {
	Filters []mailFiltersEntryModel `tfsdk:"filters"`
}

type mailFiltersEntryModel struct {
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Title       types.String `tfsdk:"title"`
	Recommended types.Bool   `tfsdk:"recommended"`
}

func (d *mailFiltersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_filters"
}

func (d *mailFiltersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the standard mail filters the KAS account may use. Their names go into the `filters` attribute of `allinkl_mail_account`.",
		Attributes: map[string]schema.Attribute{
			"filters": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every available filter.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Filter identifier, as used in `allinkl_mail_account.filters`."},
						"type":        schema.StringAttribute{Computed: true, MarkdownDescription: "Filter family; content filters take an action."},
						"title":       schema.StringAttribute{Computed: true, MarkdownDescription: "Display name."},
						"recommended": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether KAS recommends the filter."},
					},
				},
			},
		},
	}
}

func (d *mailFiltersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *mailFiltersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data mailFiltersModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := d.client.Mail.AvailableFilters(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list mail filters", err.Error())
		return
	}

	data.Filters = make([]mailFiltersEntryModel, 0, len(list))
	for _, it := range list {
		data.Filters = append(data.Filters, mailFiltersEntryModel{
			Name:        types.StringValue(it.Name),
			Type:        types.StringValue(it.Type),
			Title:       types.StringValue(it.Title),
			Recommended: types.BoolValue(it.Recommended),
		})
	}
	tflog.Debug(ctx, "read mail filters", map[string]any{"count": len(data.Filters)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
