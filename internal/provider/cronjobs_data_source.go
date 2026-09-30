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

var _ datasource.DataSourceWithConfigure = (*cronjobsDataSource)(nil)

func NewCronjobsDataSource() datasource.DataSource {
	return &cronjobsDataSource{}
}

type cronjobsDataSource struct {
	client *kasapi.Client
}

type cronjobsModel struct {
	Cronjobs []cronjobsEntryModel `tfsdk:"cronjobs"`
}

type cronjobsEntryModel struct {
	Id          types.String `tfsdk:"id"`
	Comment     types.String `tfsdk:"comment"`
	Protocol    types.String `tfsdk:"protocol"`
	URL         types.String `tfsdk:"url"`
	Minute      types.String `tfsdk:"minute"`
	Hour        types.String `tfsdk:"hour"`
	DayOfMonth  types.String `tfsdk:"day_of_month"`
	Month       types.String `tfsdk:"month"`
	DayOfWeek   types.String `tfsdk:"day_of_week"`
	HTTPUser    types.String `tfsdk:"http_user"`
	MailAddress types.String `tfsdk:"mail_address"`
	Active      types.Bool   `tfsdk:"active"`
}

func (d *cronjobsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cronjobs"
}

func (d *cronjobsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads all cronjobs of the KAS account.",
		Attributes: map[string]schema.Attribute{
			"cronjobs": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Every cronjob of the account.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "KAS-assigned id."},
						"comment":      schema.StringAttribute{Computed: true, MarkdownDescription: "Free text describing the job."},
						"protocol":     schema.StringAttribute{Computed: true, MarkdownDescription: "`http` or `https`."},
						"url":          schema.StringAttribute{Computed: true, MarkdownDescription: "Address the job requests, without the protocol."},
						"minute":       schema.StringAttribute{Computed: true, MarkdownDescription: "Minute field of the schedule."},
						"hour":         schema.StringAttribute{Computed: true, MarkdownDescription: "Hour field of the schedule."},
						"day_of_month": schema.StringAttribute{Computed: true, MarkdownDescription: "Day-of-month field of the schedule."},
						"month":        schema.StringAttribute{Computed: true, MarkdownDescription: "Month field of the schedule."},
						"day_of_week":  schema.StringAttribute{Computed: true, MarkdownDescription: "Day-of-week field of the schedule."},
						"http_user":    schema.StringAttribute{Computed: true, MarkdownDescription: "HTTP basic auth user."},
						"mail_address": schema.StringAttribute{Computed: true, MarkdownDescription: "Address that receives the output of each run."},
						"active":       schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the job runs."},
					},
				},
			},
		},
	}
}

func (d *cronjobsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *cronjobsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data cronjobsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, err := d.client.Cronjobs.List(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list cronjobs", err.Error())
		return
	}

	data.Cronjobs = make([]cronjobsEntryModel, 0, len(list))
	for _, it := range list {
		data.Cronjobs = append(data.Cronjobs, cronjobsEntryModel{
			Id:          types.StringValue(it.ID),
			Comment:     types.StringValue(it.Comment),
			Protocol:    types.StringValue(it.Protocol),
			URL:         types.StringValue(it.URL),
			Minute:      types.StringValue(it.Minute),
			Hour:        types.StringValue(it.Hour),
			DayOfMonth:  types.StringValue(it.DayOfMonth),
			Month:       types.StringValue(it.Month),
			DayOfWeek:   types.StringValue(it.DayOfWeek),
			HTTPUser:    types.StringValue(it.HTTPUser),
			MailAddress: types.StringValue(it.MailAddress),
			Active:      types.BoolValue(it.Active),
		})
	}
	tflog.Debug(ctx, "read cronjobs", map[string]any{"count": len(data.Cronjobs)})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
