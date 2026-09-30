// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package fakekas is an in-memory KAS state machine behind the SOAP wire
// format: the acceptance tests and the local example run the provider against
// it, so create/read/update/delete lifecycles behave realistically without a
// KAS account. Handle takes a KAS action with its parameters and answers the
// ReturnInfo XML or a fault code, the shape github.com/johnnycube/kasapi/kasapitest
// expects from a handler.
package fakekas

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Backend holds the account state. Every method is safe for concurrent use.
type Backend struct {
	mu sync.Mutex

	nextDNSID  int
	dnsRecords map[string]map[string]string // id -> fields

	nextMailID int
	accounts   map[string]*Account // login -> account
	forwards   map[string]*Forward // source address -> forward

	domains    map[string]*Host // name -> host
	subdomains map[string]*Host // fqdn -> host

	nextFTPID int
	ftpUsers  map[string]*FTPUser

	nextDBID  int
	databases map[string]*Database

	nextCronID int
	cronjobs   map[string]*Cronjob

	nextDDNSID int
	ddnsUsers  map[string]*DDNSUser
}

// Account is a mailbox.
type Account struct {
	Address     string
	Password    string
	Copies      []string
	Aliases     []string
	Responder   string // N, Y or "start|end"
	Text        string
	ContentType string
	DisplayName string
	State       string // Y, N or forbidden
	AllowNets   []string
	Filters     []string
	Autologin   string // Y or N
}

// Forward is a mail forward.
type Forward struct {
	Targets []string
}

// Host is a domain or subdomain with its settings and TLS state.
type Host struct {
	Path        string
	Redirect    string
	PHP         string
	Active      string // Y or N
	Certificate string
	Key         string
	Bundle      string
	CSR         string
	TLSActive   string // j or n, as KAS reports it
	TLSType     string
	ForceHTTPS  string
	HSTSMaxAge  string
}

// FTPUser is an additional FTP login.
type FTPUser struct {
	Password, Path, Comment      string
	Read, Write, List, VirusScan string // Y or N
	MainUser                     string
}

// Database is a MySQL database.
type Database struct {
	Password, Comment string
	AllowedHosts      []string
}

// Cronjob is a scheduled URL request.
type Cronjob struct {
	Fields map[string]string
}

// DDNSUser is a dynamic DNS login.
type DDNSUser struct {
	Password, Comment, Zone, Label, IP, IPv6, DualStack string
}

// New returns an empty backend.
func New() *Backend {
	return &Backend{
		nextDNSID:  100,
		dnsRecords: map[string]map[string]string{},
		nextMailID: 1000000,
		accounts:   map[string]*Account{},
		forwards:   map[string]*Forward{},
		domains:    map[string]*Host{},
		subdomains: map[string]*Host{},
		nextFTPID:  1,
		ftpUsers:   map[string]*FTPUser{},
		nextDBID:   1,
		databases:  map[string]*Database{},
		nextCronID: 300000,
		cronjobs:   map[string]*Cronjob{},
		nextDDNSID: 1,
		ddnsUsers:  map[string]*DDNSUser{},
	}
}

// MapItem renders one key/value pair of a KAS response entry.
func MapItem(key, value string) string {
	return "<item><key>" + key + "</key><value>" + value + "</value></item>"
}

func entry(kv ...string) string {
	var b strings.Builder
	b.WriteString("<item>")
	for i := 0; i+1 < len(kv); i += 2 {
		b.WriteString(MapItem(kv[i], kv[i+1]))
	}
	b.WriteString("</item>")
	return b.String()
}

func yn(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}

// Handle answers one KAS action. It is a kasapitest.Handler.
func (b *Backend) Handle(action string, params map[string]any) (string, string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	str := func(k string) string { v, _ := params[k].(string); return v }
	has := func(k string) bool { _, ok := params[k]; return ok }
	num := func(k string) string {
		switch v := params[k].(type) {
		case string:
			return v
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		default:
			return "0"
		}
	}
	// set copies a present parameter into a field and reports a change.
	set := func(dst *string, key string) bool {
		if !has(key) || *dst == str(key) {
			return false
		}
		*dst = str(key)
		return true
	}

	switch action {
	// --- DNS -------------------------------------------------------------
	case "get_dns_settings":
		out := ""
		for _, id := range sortedKeys(b.dnsRecords) {
			if has("record_id") && num("record_id") != id {
				continue
			}
			rec := b.dnsRecords[id]
			out += entry("record_id", id, "record_name", rec["name"], "record_type", rec["type"],
				"record_data", rec["data"], "record_aux", rec["aux"], "record_changeable", "Y", "record_deleteable", "Y")
		}
		return out, ""
	case "add_dns_settings":
		id := strconv.Itoa(b.nextDNSID)
		b.nextDNSID++
		b.dnsRecords[id] = map[string]string{
			"name": str("record_name"), "type": str("record_type"), "data": str("record_data"), "aux": num("record_aux"),
		}
		return id, ""
	case "update_dns_settings":
		rec, ok := b.dnsRecords[num("record_id")]
		if !ok {
			return "", "record_id_not_found"
		}
		rec["name"], rec["data"], rec["aux"] = str("record_name"), str("record_data"), num("record_aux")
		return "TRUE", ""
	case "delete_dns_settings":
		if _, ok := b.dnsRecords[num("record_id")]; !ok {
			return "", "record_id_not_found"
		}
		delete(b.dnsRecords, num("record_id"))
		return "TRUE", ""

	// --- mail accounts ----------------------------------------------------
	case "get_mailaccounts":
		out := ""
		for _, login := range sortedKeys(b.accounts) {
			if has("mail_login") && str("mail_login") != login {
				continue
			}
			acc := b.accounts[login]
			out += entry("mail_login", login, "mail_adresses", acc.Address, "mail_responder", acc.Responder,
				"mail_responder_text", acc.Text, "mail_responder_content_type", acc.ContentType,
				"mail_responder_displayname", acc.DisplayName,
				"mail_copy_adress", strings.Join(acc.Copies, ","), "mail_sender_alias", strings.Join(acc.Aliases, ","),
				"mail_is_active", acc.State, "mail_allow_nets", strings.Join(acc.AllowNets, ","),
				"mail_spamfilter", strings.Join(acc.Filters, ","), "webmail_autologin", acc.Autologin,
				"in_progress", "FALSE")
		}
		return out, ""
	case "add_mailaccount":
		if str("mail_password") == "" {
			return "", "password_missing"
		}
		login := "m" + strconv.Itoa(b.nextMailID)
		b.nextMailID++
		acc := &Account{
			Address: str("local_part") + "@" + str("domain_part"), Password: str("mail_password"),
			Copies: splitList(str("copy_adress")), Aliases: splitList(str("mail_sender_alias")),
			Responder: "N", ContentType: "text", State: "Y", Autologin: "Y",
			AllowNets: splitList(str("mail_allow_nets")),
		}
		if has("responder") {
			acc.Responder, acc.Text = str("responder"), str("responder_text")
			set(&acc.ContentType, "mail_responder_content_type")
			set(&acc.DisplayName, "mail_responder_displayname")
		}
		b.accounts[login] = acc
		return login, ""
	case "update_mailaccount":
		acc, ok := b.accounts[str("mail_login")]
		if !ok {
			return "", "mail_login_not_found"
		}
		changed := set(&acc.Password, "mail_new_password")
		if has("copy_adress") {
			acc.Copies, changed = splitList(str("copy_adress")), true
		}
		if has("mail_sender_alias") {
			acc.Aliases, changed = splitList(str("mail_sender_alias")), true
		}
		if has("mail_allow_nets") {
			acc.AllowNets, changed = splitList(str("mail_allow_nets")), true
		}
		if has("responder") {
			acc.Responder, changed = str("responder"), true
			if acc.Responder != "N" {
				set(&acc.Text, "responder_text")
				set(&acc.ContentType, "mail_responder_content_type")
				set(&acc.DisplayName, "mail_responder_displayname")
			}
		}
		changed = set(&acc.State, "is_active") || changed
		changed = set(&acc.Autologin, "webmail_autologin") || changed
		if !changed {
			return "", "nothing_to_do"
		}
		return "TRUE", ""
	case "delete_mailaccount":
		if _, ok := b.accounts[str("mail_login")]; !ok {
			return "", "mail_login_not_found"
		}
		delete(b.accounts, str("mail_login"))
		return "TRUE", ""
	case "get_mailstandardfilter":
		return entry("filter", "rspamd", "type", "rspamd", "title", "Rspam", "recommended", "Y") +
			entry("filter", "greyl", "type", "greylisting", "title", "Greylisting", "recommended", "N") +
			entry("filter", "spamc_move", "type", "content", "title", "Move spam", "recommended", "N"), ""
	case "add_mailstandardfilter":
		acc, ok := b.accounts[str("mail_login")]
		if !ok {
			return "", "login_not_found"
		}
		acc.Filters = nil
		for _, f := range strings.Split(str("filter"), ";") {
			if f != "" {
				acc.Filters = append(acc.Filters, f)
			}
		}
		return "TRUE", ""
	case "delete_mailstandardfilter":
		acc, ok := b.accounts[str("mail_login")]
		if !ok {
			return "", "login_not_found"
		}
		acc.Filters = nil
		return "TRUE", ""

	// --- mail forwards ------------------------------------------------------
	case "get_mailforwards":
		out := ""
		for _, src := range sortedKeys(b.forwards) {
			if has("mail_forward") && str("mail_forward") != src {
				continue
			}
			out += entry("mail_forward_adress", src, "mail_forward_targets", strings.Join(b.forwards[src].Targets, ","),
				"mail_forward_spamfilter", "", "in_progress", "FALSE")
		}
		return out, ""
	case "add_mailforward":
		src := str("local_part") + "@" + str("domain_part")
		if _, exists := b.forwards[src]; exists {
			return "", "mail_forward_exists"
		}
		b.forwards[src] = &Forward{Targets: collectIndexed(params, "target_", 0)}
		return "TRUE", ""
	case "update_mailforward":
		fw, ok := b.forwards[str("mail_forward")]
		if !ok {
			return "", "mail_forward_not_found"
		}
		fw.Targets = collectIndexed(params, "target_", 0)
		return "TRUE", ""
	case "delete_mailforward":
		if _, ok := b.forwards[str("mail_forward")]; !ok {
			return "", "mail_forward_not_found"
		}
		delete(b.forwards, str("mail_forward"))
		return "TRUE", ""

	// --- domains and subdomains ---------------------------------------------
	case "get_domains":
		out := ""
		for _, name := range sortedKeys(b.domains) {
			if has("domain_name") && !strings.EqualFold(str("domain_name"), name) {
				continue
			}
			out += hostEntry("domain", name, b.domains[name])
		}
		return out, ""
	case "update_domain":
		h, ok := b.domains[str("domain_name")]
		if !ok {
			return "", "domain_not_found_in_kas"
		}
		return updateHost(h, "domain_path", params, str, has)
	case "get_subdomains":
		out := ""
		for _, fqdn := range sortedKeys(b.subdomains) {
			if has("subdomain_name") && !strings.EqualFold(str("subdomain_name"), fqdn) {
				continue
			}
			out += hostEntry("subdomain", fqdn, b.subdomains[fqdn])
		}
		return out, ""
	case "add_subdomain":
		fqdn := str("subdomain_name") + "." + str("domain_name")
		if _, exists := b.subdomains[fqdn]; exists {
			return "", "subdomain_exist_as_subdomain"
		}
		h := newHost(str("subdomain_path"))
		set(&h.PHP, "php_version")
		if has("redirect_status") {
			h.Redirect = num("redirect_status")
		}
		b.subdomains[fqdn] = h
		return fqdn, ""
	case "update_subdomain":
		h, ok := b.subdomains[str("subdomain_name")]
		if !ok {
			return "", "subdomain_doenst_exist"
		}
		return updateHost(h, "subdomain_path", params, str, has)
	case "delete_subdomain":
		if _, ok := b.subdomains[str("subdomain_name")]; !ok {
			return "", "subdomain_doenst_exist"
		}
		delete(b.subdomains, str("subdomain_name"))
		return "TRUE", ""

	// --- TLS ----------------------------------------------------------------
	case "update_ssl":
		h := b.domains[str("hostname")]
		if h == nil {
			h = b.subdomains[str("hostname")]
		}
		if h == nil {
			return "", "hostname_not_found"
		}
		changed := false
		if has("ssl_certificate_sni_crt") {
			h.Certificate, h.Key = str("ssl_certificate_sni_crt"), str("ssl_certificate_sni_key")
			h.TLSActive, h.TLSType, changed = "j", "custom", true
		}
		changed = set(&h.Bundle, "ssl_certificate_sni_bundle") || changed
		changed = set(&h.CSR, "ssl_certificate_sni_csr") || changed
		if has("ssl_certificate_is_active") {
			active := map[string]string{"Y": "j", "N": "n"}[str("ssl_certificate_is_active")]
			if h.TLSActive != active {
				h.TLSActive, changed = active, true
			}
		}
		changed = set(&h.ForceHTTPS, "ssl_certificate_force_https") || changed
		if has("ssl_certificate_hsts_max_age") && h.HSTSMaxAge != num("ssl_certificate_hsts_max_age") {
			h.HSTSMaxAge, changed = num("ssl_certificate_hsts_max_age"), true
		}
		if !changed {
			return "", "nothing_to_do"
		}
		return "TRUE", ""

	// --- FTP users ----------------------------------------------------------
	case "get_ftpusers":
		out := ""
		for _, login := range sortedKeys(b.ftpUsers) {
			if has("ftp_login") && str("ftp_login") != login {
				continue
			}
			u := b.ftpUsers[login]
			out += entry("ftp_login", login, "ftp_password", u.Password, "ftp_path", u.Path, "ftp_comment", u.Comment,
				"ftp_is_main_user", u.MainUser, "ftp_permission_read", u.Read, "ftp_permission_write", u.Write,
				"ftp_permission_list", u.List, "ftp_virus_clamav", u.VirusScan, "in_progress", "FALSE")
		}
		return out, ""
	case "add_ftpuser":
		if str("ftp_password") == "" || str("ftp_comment") == "" {
			return "", "missing_parameter"
		}
		login := fmt.Sprintf("f%07d", b.nextFTPID)
		b.nextFTPID++
		u := &FTPUser{Password: str("ftp_password"), Path: "/", Comment: str("ftp_comment"),
			Read: "Y", Write: "Y", List: "Y", VirusScan: "Y", MainUser: "N"}
		set(&u.Path, "ftp_path")
		set(&u.Read, "ftp_permission_read")
		set(&u.Write, "ftp_permission_write")
		set(&u.List, "ftp_permission_list")
		set(&u.VirusScan, "ftp_virus_clamav")
		b.ftpUsers[login] = u
		return login, ""
	case "update_ftpuser":
		u, ok := b.ftpUsers[str("ftp_login")]
		if !ok {
			return "", "ftp_login_not_found"
		}
		changed := set(&u.Password, "ftp_new_password")
		for _, f := range []struct {
			dst *string
			key string
		}{{&u.Path, "ftp_path"}, {&u.Comment, "ftp_comment"}, {&u.Read, "ftp_permission_read"},
			{&u.Write, "ftp_permission_write"}, {&u.List, "ftp_permission_list"}, {&u.VirusScan, "ftp_virus_clamav"}} {
			changed = set(f.dst, f.key) || changed
		}
		if !changed {
			return "", "nothing_to_do"
		}
		return "TRUE", ""
	case "delete_ftpuser":
		u, ok := b.ftpUsers[str("ftp_login")]
		if !ok {
			return "", "ftp_login_not_found"
		}
		if u.MainUser == "Y" {
			return "", "ftp_login_belongs_to_account"
		}
		delete(b.ftpUsers, str("ftp_login"))
		return "TRUE", ""

	// --- databases ----------------------------------------------------------
	case "get_databases":
		out := ""
		for _, login := range sortedKeys(b.databases) {
			if has("database_login") && str("database_login") != login {
				continue
			}
			d := b.databases[login]
			out += entry("database_name", login, "database_login", login, "database_password", d.Password,
				"database_comment", d.Comment, "database_allowed_hosts", strings.Join(d.AllowedHosts, ", "),
				"used_database_space", "0", "in_progress", "FALSE")
		}
		return out, ""
	case "add_database":
		if str("database_password") == "" || str("database_comment") == "" {
			return "", "missing_parameter"
		}
		login := fmt.Sprintf("d%07d", b.nextDBID)
		b.nextDBID++
		b.databases[login] = &Database{Password: str("database_password"), Comment: str("database_comment"),
			AllowedHosts: splitList(str("database_allowed_hosts"))}
		return login, ""
	case "update_database":
		d, ok := b.databases[str("database_login")]
		if !ok {
			return "", "database_login_not_found"
		}
		changed := set(&d.Password, "database_new_password")
		changed = set(&d.Comment, "database_comment") || changed
		if has("database_allowed_hosts") {
			hosts := splitList(str("database_allowed_hosts"))
			if strings.Join(hosts, ",") != strings.Join(d.AllowedHosts, ",") {
				d.AllowedHosts, changed = hosts, true
			}
		}
		if !changed {
			return "", "nothing_to_do"
		}
		return "TRUE", ""
	case "delete_database":
		if _, ok := b.databases[str("database_login")]; !ok {
			return "", "database_login_not_found"
		}
		delete(b.databases, str("database_login"))
		return "TRUE", ""

	// --- cronjobs -----------------------------------------------------------
	case "get_cronjobs":
		out := ""
		for _, id := range sortedKeys(b.cronjobs) {
			if has("cronjob_id") && num("cronjob_id") != id {
				continue
			}
			f := b.cronjobs[id].Fields
			out += entry("cronjob_id", id, "cronjob_comment", f["cronjob_comment"], "protocol", f["protocol"],
				"http_url", f["http_url"], "http_user", f["http_user"], "http_password", f["http_password"],
				"minute", f["minute"], "hour", f["hour"], "day_of_month", f["day_of_month"], "month", f["month"],
				"day_of_week", f["day_of_week"], "mail_adress", f["mail_address"], "mail_condition", f["mail_condition"],
				"mail_subject", f["mail_subject"], "is_active", f["is_active"])
		}
		return out, ""
	case "add_cronjob":
		if str("http_url") == "" || str("cronjob_comment") == "" {
			return "", "missing_parameter"
		}
		id := strconv.Itoa(b.nextCronID)
		b.nextCronID++
		b.cronjobs[id] = &Cronjob{Fields: cronFields(params, map[string]string{"is_active": "Y", "protocol": "https"})}
		return id, ""
	case "update_cronjob":
		j, ok := b.cronjobs[num("cronjob_id")]
		if !ok {
			return "", "cronjob_not_found"
		}
		next := cronFields(params, j.Fields)
		if fmt.Sprint(next) == fmt.Sprint(j.Fields) {
			return "", "nothing_to_do"
		}
		j.Fields = next
		return "TRUE", ""
	case "delete_cronjob":
		if _, ok := b.cronjobs[num("cronjob_id")]; !ok {
			return "", "cronjob_id_not_found"
		}
		delete(b.cronjobs, num("cronjob_id"))
		return "TRUE", ""

	// --- dynamic DNS --------------------------------------------------------
	case "get_ddnsusers":
		if len(b.ddnsUsers) == 0 {
			return "", "empty_list"
		}
		out := ""
		for _, login := range sortedKeys(b.ddnsUsers) {
			if has("ddns_login") && str("ddns_login") != login {
				continue
			}
			u := b.ddnsUsers[login]
			out += entry("dyndns_login", login, "dyndns_comment", u.Comment, "dyndns_label", u.Label, "dyndns_zone", u.Zone,
				"dyndns_dual_stack", u.DualStack, "dyndns_password", u.Password, "dyndns_target_ip", u.IP,
				"dyndns_target_ipv4", u.IP, "dyndns_target_ipv6", u.IPv6)
		}
		if out == "" {
			return "", "dyndns_login_not_found"
		}
		return out, ""
	case "add_ddnsuser":
		if str("dyndns_password") == "" || str("dyndns_zone") == "" || str("dyndns_label") == "" {
			return "", "missing_parameter"
		}
		login := fmt.Sprintf("dyn%07d", b.nextDDNSID)
		b.nextDDNSID++
		u := &DDNSUser{Password: str("dyndns_password"), Comment: str("dyndns_comment"), Zone: str("dyndns_zone"),
			Label: str("dyndns_label"), IP: str("dyndns_target_ip"), DualStack: "N"}
		set(&u.DualStack, "dyndns_dual_stack")
		b.ddnsUsers[login] = u
		return login, ""
	case "update_ddnsuser":
		u, ok := b.ddnsUsers[str("dyndns_login")]
		if !ok {
			return "", "dyndns_login_not_found"
		}
		changed := set(&u.Password, "dyndns_password")
		changed = set(&u.Comment, "dyndns_comment") || changed
		changed = set(&u.DualStack, "dyndns_dual_stack") || changed
		if !changed {
			return "", "nothing_to_do"
		}
		return "TRUE", ""
	case "delete_ddnsuser":
		if _, ok := b.ddnsUsers[str("dyndns_login")]; !ok {
			return "", "dyndns_login_not_found"
		}
		delete(b.ddnsUsers, str("dyndns_login"))
		return "TRUE", ""
	}
	return "", fmt.Sprintf("unknown_action_%s", action)
}

func newHost(path string) *Host {
	if path == "" {
		path = "/"
	}
	return &Host{Path: path, Redirect: "0", PHP: "8.4", Active: "Y", TLSActive: "n", TLSType: "unknown", ForceHTTPS: "N", HSTSMaxAge: "-1"}
}

// hostEntry renders a domain or subdomain the way get_domains / get_subdomains do.
func hostEntry(kind, name string, h *Host) string {
	return entry(kind+"_name", name, kind+"_path", h.Path, kind+"_redirect_status", h.Redirect,
		"php_version", h.PHP, "php_deprecated", "N", "is_active", h.Active, "in_progress", "FALSE",
		"dkim_selector", "sel"+strconv.Itoa(len(name)),
		"ssl_certificate_sni_is_active", h.TLSActive, "ssl_certificate_sni_type", h.TLSType,
		"ssl_certificate_sni_force_https", h.ForceHTTPS, "ssl_certificate_sni_hsts_max_age", h.HSTSMaxAge,
		"ssl_certificate_sni_crt", h.Certificate, "ssl_certificate_sni_key", h.Key, "ssl_certificate_sni_bundle", h.Bundle)
}

// updateHost applies update_domain / update_subdomain parameters.
func updateHost(h *Host, pathKey string, params map[string]any, str func(string) string, has func(string) bool) (string, string) {
	changed := false
	apply := func(dst *string, key string) {
		if !has(key) {
			return
		}
		v := str(key)
		if f, ok := params[key].(float64); ok {
			v = strconv.FormatFloat(f, 'f', -1, 64)
		}
		if *dst != v {
			*dst, changed = v, true
		}
	}
	apply(&h.Path, pathKey)
	apply(&h.Redirect, "redirect_status")
	apply(&h.PHP, "php_version")
	apply(&h.Active, "is_active")
	if !changed {
		return "", "nothing_to_do"
	}
	return "TRUE", ""
}

// cronFields merges the cronjob parameters of a request over base.
func cronFields(params map[string]any, base map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for _, k := range []string{"protocol", "http_url", "cronjob_comment", "minute", "hour", "day_of_month", "month",
		"day_of_week", "http_user", "http_password", "mail_address", "mail_condition", "mail_subject", "is_active"} {
		if v, ok := params[k].(string); ok {
			out[k] = v
		}
	}
	return out
}

// --- state inspection and seeding, for tests and the example --------------------

// Count returns how many objects of kind exist: dns, accounts, forwards,
// subdomains, ftp, databases, cronjobs or ddns.
func (b *Backend) Count(kind string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch kind {
	case "dns":
		return len(b.dnsRecords)
	case "accounts":
		return len(b.accounts)
	case "forwards":
		return len(b.forwards)
	case "subdomains":
		return len(b.subdomains)
	case "ftp":
		return len(b.ftpUsers)
	case "databases":
		return len(b.databases)
	case "cronjobs":
		return len(b.cronjobs)
	case "ddns":
		return len(b.ddnsUsers)
	}
	panic("fakekas: unknown kind " + kind)
}

// PasswordOf returns the password the API last received for a mailbox, FTP,
// database or DDNS login, or "".
func (b *Backend) PasswordOf(login string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if acc, ok := b.accounts[login]; ok {
		return acc.Password
	}
	if u, ok := b.ftpUsers[login]; ok {
		return u.Password
	}
	if d, ok := b.databases[login]; ok {
		return d.Password
	}
	if u, ok := b.ddnsUsers[login]; ok {
		return u.Password
	}
	return ""
}

// FirstAccountLogin returns the login of the only mailbox.
func (b *Backend) FirstAccountLogin() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for login := range b.accounts {
		return login
	}
	return ""
}

// HostTLS returns the certificate and private key stored for a host and
// whether the certificate is active.
func (b *Backend) HostTLS(name string) (crt, key string, active bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	h := b.domains[name]
	if h == nil {
		h = b.subdomains[name]
	}
	if h == nil {
		return "", "", false
	}
	return h.Certificate, h.Key, h.TLSActive == "j"
}

// SeedDNSRecord adds a record to the zone.
func (b *Backend) SeedDNSRecord(name, typ, data, aux string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := strconv.Itoa(b.nextDNSID)
	b.nextDNSID++
	b.dnsRecords[id] = map[string]string{"name": name, "type": typ, "data": data, "aux": aux}
}

// SeedDomain adds a domain with the given document root.
func (b *Backend) SeedDomain(name, path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.domains[name] = newHost(path)
}

// SeedMailAccount adds a mailbox and returns its login.
func (b *Backend) SeedMailAccount(localPart, domain string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	login := "m" + strconv.Itoa(b.nextMailID)
	b.nextMailID++
	b.accounts[login] = &Account{Address: localPart + "@" + domain, Responder: "N", ContentType: "text", State: "Y", Autologin: "Y"}
	return login
}

// SeedFTPUser adds an FTP login; main marks the account's own login.
func (b *Backend) SeedFTPUser(login, path, comment string, main bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ftpUsers[login] = &FTPUser{Path: path, Comment: comment, Read: "Y", Write: "Y", List: "Y", VirusScan: "Y", MainUser: yn(main)}
}

// SeedDatabase adds a database.
func (b *Backend) SeedDatabase(login, comment string, hosts ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.databases[login] = &Database{Comment: comment, AllowedHosts: hosts}
}

// SeedCronjob adds a cronjob and returns its id.
func (b *Backend) SeedCronjob(url, comment string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := strconv.Itoa(b.nextCronID)
	b.nextCronID++
	b.cronjobs[id] = &Cronjob{Fields: map[string]string{"protocol": "https", "http_url": url, "cronjob_comment": comment,
		"minute": "*", "hour": "*", "day_of_month": "*", "month": "*", "day_of_week": "*", "is_active": "Y"}}
	return id
}

// SeedDDNSUser adds a dynamic DNS login and returns it.
func (b *Backend) SeedDDNSUser(zone, label, ip string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	login := fmt.Sprintf("dyn%07d", b.nextDDNSID)
	b.nextDDNSID++
	b.ddnsUsers[login] = &DDNSUser{Comment: label, Zone: zone, Label: label, IP: ip, DualStack: "N"}
	return login
}

// --- helpers --------------------------------------------------------------------

// splitList splits a comma-separated list; the empty string yields nil.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// collectIndexed gathers params named prefix0, prefix1, ... in order.
func collectIndexed(params map[string]any, prefix string, start int) []string {
	var out []string
	for i := start; ; i++ {
		v, ok := params[prefix+strconv.Itoa(i)]
		if !ok {
			break
		}
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
