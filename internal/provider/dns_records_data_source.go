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
	_ datasource.DataSource              = (*dnsRecordsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*dnsRecordsDataSource)(nil)
)

func NewDNSRecordsDataSource() datasource.DataSource {
	return &dnsRecordsDataSource{}
}

type dnsRecordsDataSource struct {
	client *kasapi.Client
}

type dnsRecordsModel struct {
	Zone    types.String          `tfsdk:"zone"`
	Records []dnsRecordEntryModel `tfsdk:"records"`
}

type dnsRecordEntryModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	Data       types.String `tfsdk:"data"`
	Aux        types.Int64  `tfsdk:"aux"`
	Changeable types.Bool   `tfsdk:"changeable"`
}

func (d *dnsRecordsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_records"
}

func (d *dnsRecordsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads all DNS records of a KAS-hosted zone.",
		Attributes: map[string]schema.Attribute{
			"zone": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone host, e.g. `example.com`.",
			},
			"records": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "All records in the zone, including system records.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Record id assigned by KAS."},
						"name":       schema.StringAttribute{Computed: true, MarkdownDescription: "Record name relative to the zone; empty for the zone apex."},
						"type":       schema.StringAttribute{Computed: true, MarkdownDescription: "Record type, e.g. `A`, `MX`, `TXT`."},
						"data":       schema.StringAttribute{Computed: true, MarkdownDescription: "Record payload (IP, hostname or text content)."},
						"aux":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Auxiliary value (MX/SRV priority)."},
						"changeable": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether KAS allows modifying this record."},
					},
				},
			},
		},
	}
}

func (d *dnsRecordsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *dnsRecordsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data dnsRecordsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	records, err := d.client.DNS.List(ctx, data.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to list DNS records", err.Error())
		return
	}

	data.Records = make([]dnsRecordEntryModel, 0, len(records))
	for _, r := range records {
		data.Records = append(data.Records, dnsRecordEntryModel{
			ID:         types.StringValue(r.ID),
			Name:       types.StringValue(r.Name),
			Type:       types.StringValue(r.Type),
			Data:       types.StringValue(r.Data),
			Aux:        types.Int64Value(int64(r.Aux)),
			Changeable: types.BoolValue(r.Changeable),
		})
	}
	tflog.Debug(ctx, "read DNS records", map[string]any{"zone": data.Zone.ValueString(), "count": len(data.Records)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
