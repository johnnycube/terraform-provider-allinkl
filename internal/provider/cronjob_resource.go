// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var (
	_ resource.Resource                = (*cronjobResource)(nil)
	_ resource.ResourceWithConfigure   = (*cronjobResource)(nil)
	_ resource.ResourceWithImportState = (*cronjobResource)(nil)
)

func NewCronjobResource() resource.Resource {
	return &cronjobResource{}
}

type cronjobResource struct {
	client *kasapi.Client
}

type cronjobModel struct {
	ID            types.String `tfsdk:"id"`
	Comment       types.String `tfsdk:"comment"`
	Protocol      types.String `tfsdk:"protocol"`
	URL           types.String `tfsdk:"url"`
	Minute        types.String `tfsdk:"minute"`
	Hour          types.String `tfsdk:"hour"`
	DayOfMonth    types.String `tfsdk:"day_of_month"`
	Month         types.String `tfsdk:"month"`
	DayOfWeek     types.String `tfsdk:"day_of_week"`
	HTTPUser      types.String `tfsdk:"http_user"`
	HTTPPassword  types.String `tfsdk:"http_password"`
	MailAddress   types.String `tfsdk:"mail_address"`
	MailCondition types.String `tfsdk:"mail_condition"`
	MailSubject   types.String `tfsdk:"mail_subject"`
	Active        types.Bool   `tfsdk:"active"`
}

func (m cronjobModel) job() kasapi.Cronjob {
	return kasapi.Cronjob{
		ID: m.ID.ValueString(), Comment: m.Comment.ValueString(), Protocol: m.Protocol.ValueString(), URL: m.URL.ValueString(),
		Minute: m.Minute.ValueString(), Hour: m.Hour.ValueString(), DayOfMonth: m.DayOfMonth.ValueString(),
		Month: m.Month.ValueString(), DayOfWeek: m.DayOfWeek.ValueString(),
		HTTPUser: m.HTTPUser.ValueString(), HTTPPassword: m.HTTPPassword.ValueString(),
		MailAddress: m.MailAddress.ValueString(), MailCondition: m.MailCondition.ValueString(),
		MailSubject: m.MailSubject.ValueString(), Active: m.Active.ValueBool(),
	}
}

// fill copies the job KAS reports into the model. The HTTP password is
// write-only and stays as it is.
func (m *cronjobModel) fill(j *kasapi.Cronjob) {
	m.ID = types.StringValue(j.ID)
	m.Comment = types.StringValue(j.Comment)
	m.Protocol = types.StringValue(j.Protocol)
	m.URL = types.StringValue(j.URL)
	m.Minute, m.Hour = types.StringValue(j.Minute), types.StringValue(j.Hour)
	m.DayOfMonth, m.Month, m.DayOfWeek = types.StringValue(j.DayOfMonth), types.StringValue(j.Month), types.StringValue(j.DayOfWeek)
	m.Active = types.BoolValue(j.Active)
	optional := func(dst *types.String, v string) {
		if v == "" && dst.IsNull() {
			return
		}
		*dst = types.StringValue(v)
	}
	optional(&m.HTTPUser, j.HTTPUser)
	optional(&m.MailAddress, j.MailAddress)
	// KAS fills these two itself; they are computed so the server value
	// does not read as drift.
	m.MailCondition = types.StringValue(j.MailCondition)
	m.MailSubject = types.StringValue(j.MailSubject)
	if m.MailSubject.ValueString() == "" {
		m.MailSubject = types.StringValue("default")
	}
}

func (r *cronjobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cronjob"
}

func (r *cronjobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	field := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Optional: true, Computed: true, Default: stringdefault.StaticString("*"), MarkdownDescription: desc,
			Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^(\*|\*/\d+|\d+(-\d+)?(,\d+(-\d+)?)*)$`), "must be cron syntax: a number, `*`, or a step like `*/15`")},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a cronjob of the all-inkl.com (KAS) account. A KAS cronjob requests a URL on a schedule; it does not run a command.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "KAS-assigned id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"comment": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Free text describing the job. KAS requires one.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"url": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Address to request without the protocol, e.g. `example.com/cron.php`.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1),
					stringvalidator.RegexMatches(regexp.MustCompile(`^[^:/][^:]*$`), "must not carry a protocol; set `protocol` instead")},
			},
			"protocol": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("https"),
				MarkdownDescription: "`https` (default) or `http`.",
				Validators:          []validator.String{stringvalidator.OneOf("http", "https")},
			},
			"minute":       field("Minute: `0-59`, `*` or a step like `*/15`."),
			"hour":         field("Hour: `0-23`, `*` or a step."),
			"day_of_month": field("Day of month: `1-31` or `*`."),
			"month":        field("Month: `1-12` or `*`."),
			"day_of_week":  field("Day of week: `0-7` or `*`; Sunday is `0` or `7`."),
			"http_user": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "HTTP basic auth user. Set `http_password` with it.",
			},
			"http_password": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "HTTP basic auth password. Write-only: drift is not detectable. As with all Terraform secrets, the value is stored in the state file.",
			},
			"mail_address": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Address that receives the output of each run.",
			},
			"mail_condition": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "No mail is sent when the output contains this word.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mail_subject": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("default"),
				MarkdownDescription: "`default`, or `comment` to use the comment as subject.",
				Validators:          []validator.String{stringvalidator.OneOf("default", "comment")},
			},
			"active": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the job runs.",
			},
		},
	}
}

func (r *cronjobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*kasapi.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected resource configure type",
			fmt.Sprintf("Expected *kasapi.Client, got: %T", req.ProviderData))
		return
	}
	r.client = client
}

func (r *cronjobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cronjobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.client.Cronjobs.Create(ctx, plan.job())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create cronjob", err.Error())
		return
	}
	tflog.Debug(ctx, "created cronjob", map[string]any{"id": id})
	j, err := r.client.Cronjobs.Get(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read cronjob after creation", err.Error())
		return
	}
	plan.fill(j)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *cronjobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cronjobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	j, err := r.client.Cronjobs.Get(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read cronjob", err.Error())
		return
	}
	state.fill(j)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *cronjobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state cronjobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	if err := r.client.Cronjobs.Update(ctx, plan.job()); err != nil {
		resp.Diagnostics.AddError("Failed to update cronjob", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *cronjobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cronjobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Cronjobs.Delete(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete cronjob", err.Error())
	}
}

// ImportState supports `terraform import allinkl_cronjob.nightly 325208`.
func (r *cronjobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
