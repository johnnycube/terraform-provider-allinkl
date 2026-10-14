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
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/johnnycube/kasapi"
)

var (
	_ resource.Resource                = (*mailAccountResource)(nil)
	_ resource.ResourceWithConfigure   = (*mailAccountResource)(nil)
	_ resource.ResourceWithImportState = (*mailAccountResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*mailAccountResource)(nil)
)

func NewMailAccountResource() resource.Resource {
	return &mailAccountResource{}
}

type mailAccountResource struct {
	client *kasapi.Client
}

type mailAccountModel struct {
	ID               types.String `tfsdk:"id"`
	LocalPart        types.String `tfsdk:"local_part"`
	Domain           types.String `tfsdk:"domain"`
	Password         types.String `tfsdk:"password"`
	CopyAddresses    types.List   `tfsdk:"copy_addresses"`
	SenderAliases    types.List   `tfsdk:"sender_aliases"`
	Address          types.String `tfsdk:"address"`
	State            types.String `tfsdk:"state"`
	AllowedClients   types.List   `tfsdk:"allowed_clients"`
	WebmailAutologin types.Bool   `tfsdk:"webmail_autologin"`
	Responder        types.Object `tfsdk:"responder"`
	Filters          types.List   `tfsdk:"filters"`
	SpamFilters      types.List   `tfsdk:"spam_filters"`
}

// responderModel is the `responder` block. Its presence turns the
// autoresponder on.
type responderModel struct {
	Text        types.String `tfsdk:"text"`
	ContentType types.String `tfsdk:"content_type"`
	DisplayName types.String `tfsdk:"display_name"`
	Start       types.String `tfsdk:"start"`
	End         types.String `tfsdk:"end"`
}

var responderAttrTypes = map[string]attr.Type{
	"text": types.StringType, "content_type": types.StringType, "display_name": types.StringType,
	"start": types.StringType, "end": types.StringType,
}

var mailStates = map[string]kasapi.MailState{
	"active":           kasapi.MailActive,
	"receive_disabled": kasapi.MailReceiveDisabled,
	"forbidden":        kasapi.MailForbidden,
}

func mailStateName(s kasapi.MailState) string {
	for name, v := range mailStates {
		if v == s {
			return name
		}
	}
	return string(s)
}

// responderFromObject converts the block into the kasapi responder; a null
// object is an inactive responder.
func responderFromObject(ctx context.Context, obj types.Object, diags *diag.Diagnostics) kasapi.Responder {
	if obj.IsNull() || obj.IsUnknown() {
		return kasapi.Responder{}
	}
	var m responderModel
	diags.Append(obj.As(ctx, &m, basetypes.ObjectAsOptions{})...)
	r := kasapi.Responder{Active: true, Text: m.Text.ValueString(), ContentType: m.ContentType.ValueString(), DisplayName: m.DisplayName.ValueString()}
	for _, f := range []struct {
		v   types.String
		dst *time.Time
		key string
	}{{m.Start, &r.Start, "start"}, {m.End, &r.End, "end"}} {
		if f.v.IsNull() || f.v.ValueString() == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, f.v.ValueString())
		if err != nil {
			diags.AddAttributeError(path.Root("responder").AtName(f.key), "Invalid timestamp", err.Error())
			continue
		}
		*f.dst = t
	}
	return r
}

// responderObject converts the responder KAS reports into the block; an
// inactive responder is a null object. Timestamps keep the configured
// spelling when they denote the same instant.
func responderObject(ctx context.Context, r kasapi.Responder, prev types.Object, diags *diag.Diagnostics) types.Object {
	if !r.Active {
		return types.ObjectNull(responderAttrTypes)
	}
	m := responderModel{
		Text: types.StringValue(r.Text), ContentType: types.StringValue(r.ContentType),
		DisplayName: types.StringValue(r.DisplayName), Start: types.StringNull(), End: types.StringNull(),
	}
	if r.ContentType == "" {
		m.ContentType = types.StringValue("text")
	}
	var old responderModel
	if !prev.IsNull() && !prev.IsUnknown() {
		diags.Append(prev.As(ctx, &old, basetypes.ObjectAsOptions{})...)
	}
	keep := func(configured types.String, t time.Time) types.String {
		if t.IsZero() {
			return types.StringNull()
		}
		if c, err := time.Parse(time.RFC3339, configured.ValueString()); err == nil && c.Equal(t) {
			return configured
		}
		return types.StringValue(t.UTC().Format(time.RFC3339))
	}
	m.Start, m.End = keep(old.Start, r.Start), keep(old.End, r.End)
	if old.DisplayName.IsNull() && r.DisplayName == "" {
		m.DisplayName = types.StringNull()
	}
	obj, d := types.ObjectValueFrom(ctx, responderAttrTypes, m)
	diags.Append(d...)
	return obj
}

// filtersFrom parses "name" or "name:action" entries.
func filtersFrom(entries []string) []kasapi.MailFilter {
	out := make([]kasapi.MailFilter, 0, len(entries))
	for _, e := range entries {
		name, action, _ := strings.Cut(e, ":")
		out = append(out, kasapi.MailFilter{Name: name, Action: action})
	}
	return out
}

func (r *mailAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mail_account"
}

func (r *mailAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a mailbox hosted at all-inkl.com (KAS).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "KAS-assigned mail login, e.g. `m1234567`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"local_part": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Part of the address before the `@`. Changing it forces a new mailbox.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Domain part of the address. Must be hosted in the KAS account. Changing it forces a new mailbox.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(3),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				MarkdownDescription: "Mailbox password. Write-only: the KAS API never returns passwords, so " +
					"drift in the password is not detectable; changing the value here updates it. " +
					"Prefer passing it via a variable with `sensitive = true` and note that, as with " +
					"all Terraform secrets, the value is stored in the state file — protect your state accordingly.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(8),
				},
			},
			"copy_addresses": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Addresses that receive a copy of every incoming mail.",
				Validators: []validator.List{
					listvalidator.SizeAtMost(10),
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(3)),
				},
			},
			"sender_aliases": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Addresses the mailbox may use in the FROM header when sending. " +
					"Aliases only affect sending; to receive mail under an alias, create an " +
					"`allinkl_mail_forward` pointing at this mailbox instead.",
				Validators: []validator.List{
					listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(3)),
				},
			},
			"address": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Full primary address (`local_part@domain`).",
			},
			"state": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("active"),
				MarkdownDescription: "`active` (default), `receive_disabled` (rejects new mail, stored mail stays " +
					"retrievable) or `forbidden` (rejects mail and blocks retrieval).",
				Validators: []validator.String{stringvalidator.OneOf("active", "receive_disabled", "forbidden")},
			},
			"allowed_clients": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Clients allowed to access the mailbox: IP addresses, CIDR networks or `webmail`. Unset means unrestricted.",
				Validators:          []validator.List{listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1))},
			},
			"webmail_autologin": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the KAS panel may open webmail without the mailbox password.",
			},
			"responder": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Autoresponder. Present turns it on; absent turns it off.",
				Attributes: map[string]schema.Attribute{
					"text": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Reply text.",
						Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
					},
					"content_type": schema.StringAttribute{
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString("text"),
						MarkdownDescription: "`text` (default) or `html`.",
						Validators:          []validator.String{stringvalidator.OneOf("text", "html")},
					},
					"display_name": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Sender name shown on the reply.",
					},
					"start": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Start of the responder window, RFC 3339. Set `start` and `end` together.",
					},
					"end": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "End of the responder window, RFC 3339.",
					},
				},
			},
			"filters": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				MarkdownDescription: "Standard filters, each `name` or `name:action` (`delete`, `mark`, `move=<folder>`, " +
					"`forward=<address>`; actions apply to content filters). `allinkl_mail_filters` lists the names. " +
					"Write-only: KAS reports active filters under other names, see `spam_filters`, so a change " +
					"at KAS is not detected.",
				Validators: []validator.List{listvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1))},
			},
			"spam_filters": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Filters active on the mailbox, as KAS names them.",
			},
		},
	}
}

func (r *mailAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan keeps spam_filters known when the filters do not change.
func (r *mailAccountResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var plan, state mailAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !plan.Filters.Equal(state.Filters) {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("spam_filters"), state.SpamFilters)...)
}

func (r *mailAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mailAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	copies := listToStrings(ctx, plan.CopyAddresses, &resp.Diagnostics)
	aliases := listToStrings(ctx, plan.SenderAliases, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	address := plan.LocalPart.ValueString() + "@" + plan.Domain.ValueString()

	if r.existsUnmanaged(ctx, address, &resp.Diagnostics) {
		return
	}

	clients := listToStrings(ctx, plan.AllowedClients, &resp.Diagnostics)
	responder := responderFromObject(ctx, plan.Responder, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	login, err := r.client.Mail.CreateAccount(ctx, kasapi.MailAccount{
		LocalPart:     plan.LocalPart.ValueString(),
		Domain:        plan.Domain.ValueString(),
		CopyAddresses: copies,
		SenderAliases: aliases,
		AllowNets:     clients,
		Responder:     responder,
	}, plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create mail account", err.Error())
		return
	}

	plan.ID = types.StringValue(login)
	plan.Address = types.StringValue(address)
	tflog.Debug(ctx, "created mail account", map[string]any{"id": login, "address": address})

	r.applyAfterCreate(ctx, login, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	r.refresh(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// existsUnmanaged reports a mailbox that exists at KAS but not in the state,
// pointing at import instead of failing with a raw fault. Best-effort: a
// failed lookup returns false and lets Create surface the real error.
func (r *mailAccountResource) existsUnmanaged(ctx context.Context, address string, diags *diag.Diagnostics) bool {
	existing, err := r.client.Mail.ListAccounts(ctx)
	if err != nil {
		return false
	}
	for _, acc := range existing {
		if acc.Address() == address {
			diags.AddError(
				"Mail account already exists",
				fmt.Sprintf("A mailbox for %s already exists at KAS (login %s). Import it "+
					"instead of creating it:\n\n  terraform import allinkl_mail_account.<name> %s",
					address, acc.Login, acc.Login),
			)
			return true
		}
	}
	return false
}

// applyAfterCreate sets what add_mailaccount cannot: KAS creates every mailbox
// active with webmail autologin on and no filters.
func (r *mailAccountResource) applyAfterCreate(ctx context.Context, login string, plan *mailAccountModel, diags *diag.Diagnostics) {
	if state := plan.State.ValueString(); state != "active" {
		if err := r.client.Mail.UpdateState(ctx, login, mailStates[state]); err != nil {
			diags.AddError("Failed to set mailbox state", err.Error())
			return
		}
	}
	if !plan.WebmailAutologin.ValueBool() {
		if err := r.client.Mail.UpdateWebmailAutologin(ctx, login, false); err != nil {
			diags.AddError("Failed to set webmail autologin", err.Error())
			return
		}
	}
	if filters := listToStrings(ctx, plan.Filters, diags); len(filters) > 0 {
		if err := r.client.Mail.SetFilters(ctx, login, filtersFrom(filters)); err != nil {
			diags.AddError("Failed to set mail filters", err.Error())
		}
	}
}

// refresh reads the mailbox from KAS into the model.
func (r *mailAccountResource) refresh(ctx context.Context, m *mailAccountModel, diags *diag.Diagnostics) {
	acc, err := r.client.Mail.GetAccount(ctx, m.ID.ValueString())
	if err != nil {
		diags.AddError("Failed to read mail account", err.Error())
		return
	}
	m.fill(ctx, acc, diags)
}

// fill copies the state KAS reports into the model; password and filters are
// write-only and stay as they are.
func (m *mailAccountModel) fill(ctx context.Context, acc *kasapi.MailAccount, diags *diag.Diagnostics) {
	m.LocalPart = types.StringValue(acc.LocalPart)
	m.Domain = types.StringValue(acc.Domain)
	m.Address = types.StringValue(acc.Address())
	if len(acc.CopyAddresses) > 0 || !m.CopyAddresses.IsNull() {
		m.CopyAddresses = stringsToList(ctx, acc.CopyAddresses, diags)
	}
	if len(acc.SenderAliases) > 0 || !m.SenderAliases.IsNull() {
		m.SenderAliases = stringsToList(ctx, acc.SenderAliases, diags)
	}
	if len(acc.AllowNets) > 0 || !m.AllowedClients.IsNull() {
		m.AllowedClients = stringsToList(ctx, acc.AllowNets, diags)
	}
	m.State = types.StringValue(mailStateName(acc.State))
	m.WebmailAutologin = types.BoolValue(acc.WebmailAutologin)
	m.Responder = responderObject(ctx, acc.Responder, m.Responder, diags)
	m.SpamFilters = stringsToList(ctx, acc.SpamFilters, diags)
	if m.SpamFilters.IsNull() {
		m.SpamFilters = types.ListValueMust(types.StringType, nil)
	}
}

func (r *mailAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mailAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	acc, err := r.client.Mail.GetAccount(ctx, state.ID.ValueString())
	if errors.Is(err, kasapi.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read mail account", err.Error())
		return
	}

	state.fill(ctx, acc, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mailAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state mailAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, u := range r.updates(ctx, state.ID.ValueString(), &plan, &state, &resp.Diagnostics) {
		if !u.changed {
			continue
		}
		if err := u.apply(); err != nil {
			resp.Diagnostics.AddError(u.summary, err.Error())
		}
		if resp.Diagnostics.HasError() {
			return
		}
	}

	plan.ID = state.ID
	r.refresh(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// mailUpdate is one KAS call of an update, made when its attribute changed.
type mailUpdate struct {
	changed bool
	summary string
	apply   func() error
}

// updates lists the KAS calls that move a mailbox from state to plan, in order.
func (r *mailAccountResource) updates(ctx context.Context, login string, plan, state *mailAccountModel, diags *diag.Diagnostics) []mailUpdate {
	mail := r.client.Mail
	// list converts a list attribute first; a conversion error lands in diags.
	list := func(l types.List, call func([]string) error) func() error {
		return func() error {
			v := listToStrings(ctx, l, diags)
			if diags.HasError() {
				return nil
			}
			return call(v)
		}
	}
	return []mailUpdate{
		{!plan.Password.Equal(state.Password), "Failed to update mailbox password", func() error {
			return mail.UpdatePassword(ctx, login, plan.Password.ValueString())
		}},
		{!plan.CopyAddresses.Equal(state.CopyAddresses), "Failed to update copy addresses", list(plan.CopyAddresses, func(v []string) error {
			return mail.UpdateCopyAddresses(ctx, login, v)
		})},
		{!plan.SenderAliases.Equal(state.SenderAliases), "Failed to update sender aliases", list(plan.SenderAliases, func(v []string) error {
			return mail.UpdateSenderAliases(ctx, login, v)
		})},
		{!plan.AllowedClients.Equal(state.AllowedClients), "Failed to update allowed clients", list(plan.AllowedClients, func(v []string) error {
			return mail.UpdateAllowNets(ctx, login, v)
		})},
		{!plan.State.Equal(state.State), "Failed to update mailbox state", func() error {
			return mail.UpdateState(ctx, login, mailStates[plan.State.ValueString()])
		}},
		{!plan.WebmailAutologin.Equal(state.WebmailAutologin), "Failed to update webmail autologin", func() error {
			return mail.UpdateWebmailAutologin(ctx, login, plan.WebmailAutologin.ValueBool())
		}},
		{!plan.Responder.Equal(state.Responder), "Failed to update responder", func() error {
			responder := responderFromObject(ctx, plan.Responder, diags)
			if diags.HasError() {
				return nil
			}
			return mail.UpdateResponder(ctx, login, responder)
		}},
		{!plan.Filters.Equal(state.Filters), "Failed to update mail filters", list(plan.Filters, func(v []string) error {
			if len(v) == 0 {
				return mail.DeleteFilters(ctx, login)
			}
			return mail.SetFilters(ctx, login, filtersFrom(v))
		})},
	}
}

func (r *mailAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mailAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Mail.DeleteAccount(ctx, state.ID.ValueString())
	if err != nil && !errors.Is(err, kasapi.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete mail account", err.Error())
	}
}

// ImportState supports `terraform import allinkl_mail_account.info m1234567`.
// After import the next apply sets the password to the configured value,
// because the API cannot read existing passwords.
func (r *mailAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
