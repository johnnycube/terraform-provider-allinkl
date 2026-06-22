// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/johnnycube/kasapi/kasapitest"
)

// fakeBackend is an in-memory KAS state machine implementing the DNS, mail
// and subdomain actions, so full create/read/update/delete lifecycles behave
// realistically in acceptance tests.
type fakeBackend struct {
	mu sync.Mutex

	nextDNSID  int
	dnsRecords map[string]map[string]string // id -> fields

	nextMailID int
	accounts   map[string]*fakeAccount // login -> account
	forwards   map[string][]string     // source address -> targets

	subdomains map[string]string // fqdn -> path
	domains    map[string]string // name -> path
}

type fakeAccount struct {
	address  string
	password string
	copies   []string
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		nextDNSID:  100,
		dnsRecords: map[string]map[string]string{},
		nextMailID: 1000000,
		accounts:   map[string]*fakeAccount{},
		forwards:   map[string][]string{},
		subdomains: map[string]string{},
		domains:    map[string]string{},
	}
}

func (b *fakeBackend) handle(action string, params map[string]any) (string, string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	str := func(k string) string { v, _ := params[k].(string); return v }
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

	switch action {
	// --- DNS -------------------------------------------------------------
	case "get_dns_settings":
		out := ""
		for _, id := range sortedKeys3(b.dnsRecords) {
			rec := b.dnsRecords[id]
			out += "<item>" +
				kasapitest.MapItem("record_id", id) +
				kasapitest.MapItem("record_name", rec["name"]) +
				kasapitest.MapItem("record_type", rec["type"]) +
				kasapitest.MapItem("record_data", rec["data"]) +
				kasapitest.MapItem("record_aux", rec["aux"]) +
				kasapitest.MapItem("record_changeable", "Y") +
				"</item>"
		}
		return out, ""

	case "add_dns_settings":
		id := strconv.Itoa(b.nextDNSID)
		b.nextDNSID++
		b.dnsRecords[id] = map[string]string{
			"name": str("record_name"),
			"type": str("record_type"),
			"data": str("record_data"),
			"aux":  num("record_aux"),
		}
		return id, ""

	case "update_dns_settings":
		rec, ok := b.dnsRecords[str("record_id")]
		if !ok {
			return "", "record_id_not_found"
		}
		rec["name"] = str("record_name")
		rec["data"] = str("record_data")
		rec["aux"] = num("record_aux")
		return "TRUE", ""

	case "delete_dns_settings":
		if _, ok := b.dnsRecords[str("record_id")]; !ok {
			return "", "record_id_not_found"
		}
		delete(b.dnsRecords, str("record_id"))
		return "TRUE", ""

	// --- mail accounts ----------------------------------------------------
	case "get_mailaccounts":
		out := ""
		for _, login := range sortedKeys3(b.accounts) {
			acc := b.accounts[login]
			out += "<item>" +
				kasapitest.MapItem("mail_login", login) +
				kasapitest.MapItem("mail_adresses", acc.address) +
				kasapitest.MapItem("mail_responder", "N") +
				kasapitest.MapItem("mail_copy_adress", strings.Join(acc.copies, ",")) +
				"</item>"
		}
		return out, ""

	case "add_mailaccount":
		if str("mail_password") == "" {
			return "", "password_missing"
		}
		login := "m" + strconv.Itoa(b.nextMailID)
		b.nextMailID++
		b.accounts[login] = &fakeAccount{
			address:  str("local_part") + "@" + str("domain_part"),
			password: str("mail_password"),
			copies:   collectIndexed(params, "copy_adress_", 0),
		}
		return login, ""

	case "update_mailaccount":
		acc, ok := b.accounts[str("mail_login")]
		if !ok {
			return "", "mail_login_not_found"
		}
		if pw := str("mail_new_password"); pw != "" {
			acc.password = pw
		} else {
			acc.copies = collectIndexed(params, "copy_adress_", 0)
		}
		return "TRUE", ""

	case "delete_mailaccount":
		if _, ok := b.accounts[str("mail_login")]; !ok {
			return "", "mail_login_not_found"
		}
		delete(b.accounts, str("mail_login"))
		return "TRUE", ""

	// --- mail forwards ------------------------------------------------------
	case "get_mailforwards":
		out := ""
		for _, src := range sortedKeys3(b.forwards) {
			out += "<item>" +
				kasapitest.MapItem("mail_forward_adress", src) +
				kasapitest.MapItem("mail_forward_targets", strings.Join(b.forwards[src], ",")) +
				"</item>"
		}
		return out, ""

	case "add_mailforward":
		src := str("local_part") + "@" + str("domain_part")
		if _, exists := b.forwards[src]; exists {
			return "", "mail_forward_exists"
		}
		b.forwards[src] = collectIndexed(params, "target_", 1)
		return "TRUE", ""

	case "update_mailforward":
		if _, ok := b.forwards[str("mail_forward")]; !ok {
			return "", "mail_forward_not_found"
		}
		b.forwards[str("mail_forward")] = collectIndexed(params, "target_", 1)
		return "TRUE", ""

	case "delete_mailforward":
		if _, ok := b.forwards[str("mail_forward")]; !ok {
			return "", "mail_forward_not_found"
		}
		delete(b.forwards, str("mail_forward"))
		return "TRUE", ""

	// --- domains (read-only) --------------------------------------------------
	case "get_domains":
		out := ""
		for _, name := range sortedKeys3(b.domains) {
			out += "<item>" +
				kasapitest.MapItem("domain_name", name) +
				kasapitest.MapItem("domain_path", b.domains[name]) +
				"</item>"
		}
		return out, ""

	// --- subdomains ---------------------------------------------------------
	case "get_subdomains":
		out := ""
		for _, fqdn := range sortedKeys3(b.subdomains) {
			out += "<item>" +
				kasapitest.MapItem("subdomain_name", fqdn) +
				kasapitest.MapItem("subdomain_path", b.subdomains[fqdn]) +
				"</item>"
		}
		return out, ""

	case "add_subdomain":
		fqdn := str("subdomain_name") + "." + str("domain_name")
		if _, exists := b.subdomains[fqdn]; exists {
			return "", "subdomain_exists"
		}
		b.subdomains[fqdn] = str("subdomain_path")
		return "TRUE", ""

	case "update_subdomain":
		if _, ok := b.subdomains[str("subdomain_name")]; !ok {
			return "", "subdomain_not_found"
		}
		b.subdomains[str("subdomain_name")] = str("subdomain_path")
		return "TRUE", ""

	case "delete_subdomain":
		if _, ok := b.subdomains[str("subdomain_name")]; !ok {
			return "", "subdomain_not_found"
		}
		delete(b.subdomains, str("subdomain_name"))
		return "TRUE", ""
	}
	return "", fmt.Sprintf("unknown_action_%s", action)
}

// --- assertions used by tests --------------------------------------------------

func (b *fakeBackend) dnsCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.dnsRecords)
}

func (b *fakeBackend) accountCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.accounts)
}

func (b *fakeBackend) forwardCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.forwards)
}

func (b *fakeBackend) subdomainCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.subdomains)
}

// passwordOf returns the password the API last received for a login.
func (b *fakeBackend) passwordOf(login string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if acc, ok := b.accounts[login]; ok {
		return acc.password
	}
	return ""
}

// firstAccountLogin returns the login of the only account (test helper).
func (b *fakeBackend) firstAccountLogin() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for login := range b.accounts {
		return login
	}
	return ""
}

// --- seeding (simulate pre-existing server-side state) ---------------------------

func (b *fakeBackend) seedDNSRecord(name, typ, data, aux string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := strconv.Itoa(b.nextDNSID)
	b.nextDNSID++
	b.dnsRecords[id] = map[string]string{"name": name, "type": typ, "data": data, "aux": aux}
}

func (b *fakeBackend) seedDomain(name, path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.domains[name] = path
}

func (b *fakeBackend) seedMailAccount(localPart, domain string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	login := "m" + strconv.Itoa(b.nextMailID)
	b.nextMailID++
	b.accounts[login] = &fakeAccount{address: localPart + "@" + domain}
}

// --- helpers --------------------------------------------------------------------

// collectIndexed gathers params named prefix0,prefix1,... (or 1-based) in order.
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

func sortedKeys3[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
