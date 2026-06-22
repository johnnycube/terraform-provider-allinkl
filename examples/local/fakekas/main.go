// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Command fakekas is a standalone, in-memory fake of the all-inkl.com KAS SOAP
// API, so the provider can run a real plan/apply on this machine without a KAS
// account. KasAuth issues a session token (password defaults to "secret");
// KasApi validates the token and delegates to an in-memory state machine that
// implements the DNS, mail and subdomain actions.
//
// It speaks the same wire protocol as the github.com/johnnycube/kasapi/kasapitest
// server used by the acceptance tests; point the provider at it with
// KAS_API_ENDPOINT / KAS_AUTH_ENDPOINT. Run: go run ./fakekas -addr 127.0.0.1:8511
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	password = "secret"          // plaintext the fake expects (any login)
	token    = "session-token-1" // session token KasAuth issues
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8511", "address to listen on")
	flag.Parse()

	b := newBackend()
	// Seed a little server-side state so the read-only data sources return
	// something on a fresh apply.
	b.domains["example.com"] = "/"
	b.domains["example.org"] = "/example_org/"

	mux := http.NewServeMux()
	mux.HandleFunc("/KasAuth.php", b.serve)
	mux.HandleFunc("/KasApi.php", b.serve)

	log.Printf("fake KAS listening on http://%s (login: any, password: %q)", *addr, password)
	log.Printf("  KAS_AUTH_ENDPOINT=http://%s/KasAuth.php", *addr)
	log.Printf("  KAS_API_ENDPOINT=http://%s/KasApi.php", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

var paramsRe = regexp.MustCompile(`(?s)<Params[^>]*>(.*?)</Params>`)

func (b *backend) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	m := paramsRe.FindSubmatch(body)
	if m == nil {
		http.Error(w, "no Params", http.StatusBadRequest)
		return
	}

	var req map[string]any
	if err := json.Unmarshal([]byte(xmlUnescape(string(m[1]))), &req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")

	if strings.Contains(r.URL.Path, "KasAuth") {
		ok := false
		switch req["kas_auth_type"] {
		case "sha1":
			sum := sha1.Sum([]byte(password))
			ok = req["kas_auth_data"] == hex.EncodeToString(sum[:])
		case "plain":
			ok = req["kas_auth_data"] == password
		}
		if !ok {
			writeFault(w, "kas_login_incorrect")
			return
		}
		fmt.Fprint(w, envelope(`<return xsi:type="xsd:string">`+token+`</return>`))
		return
	}

	if req["kas_auth_data"] != token {
		writeFault(w, "session_expired")
		return
	}

	action, _ := req["kas_action"].(string)
	params, _ := req["KasRequestParams"].(map[string]any)

	info, fault := b.handle(action, params)
	if fault != "" {
		writeFault(w, fault)
		return
	}
	fmt.Fprint(w, envelope(`<return>
  <item><key>Request</key><value></value></item>
  <item><key>Response</key><value>
    <item><key>ReturnString</key><value>TRUE</value></item>
    <item><key>ReturnInfo</key><value>`+info+`</value></item>
  </value></item>
  <item><key>KasFloodDelay</key><value>0.01</value></item>
</return>`))
}

// backend is an in-memory KAS state machine. It mirrors the per-action
// behaviour of internal/provider/fake_backend_test.go so the provider drives
// real create/read/update/delete lifecycles against it.
type backend struct {
	mu sync.Mutex

	nextDNSID  int
	dnsRecords map[string]map[string]string // id -> fields

	nextMailID int
	accounts   map[string]*account // login -> account
	forwards   map[string][]string // source address -> targets

	subdomains map[string]string // fqdn -> path
	domains    map[string]string // name -> path
}

type account struct {
	address  string
	password string
	copies   []string
}

func newBackend() *backend {
	return &backend{
		nextDNSID:  100,
		dnsRecords: map[string]map[string]string{},
		nextMailID: 1000000,
		accounts:   map[string]*account{},
		forwards:   map[string][]string{},
		subdomains: map[string]string{},
		domains:    map[string]string{},
	}
}

func (b *backend) handle(action string, params map[string]any) (string, string) {
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
	// --- DNS ---------------------------------------------------------------
	case "get_dns_settings":
		out := ""
		for _, id := range sortedKeys(b.dnsRecords) {
			rec := b.dnsRecords[id]
			out += "<item>" +
				mapItem("record_id", id) +
				mapItem("record_name", rec["name"]) +
				mapItem("record_type", rec["type"]) +
				mapItem("record_data", rec["data"]) +
				mapItem("record_aux", rec["aux"]) +
				mapItem("record_changeable", "Y") +
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

	// --- mail accounts -----------------------------------------------------
	case "get_mailaccounts":
		out := ""
		for _, login := range sortedKeys(b.accounts) {
			acc := b.accounts[login]
			out += "<item>" +
				mapItem("mail_login", login) +
				mapItem("mail_adresses", acc.address) +
				mapItem("mail_responder", "N") +
				mapItem("mail_copy_adress", strings.Join(acc.copies, ",")) +
				"</item>"
		}
		return out, ""

	case "add_mailaccount":
		if str("mail_password") == "" {
			return "", "password_missing"
		}
		login := "m" + strconv.Itoa(b.nextMailID)
		b.nextMailID++
		b.accounts[login] = &account{
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

	// --- mail forwards -----------------------------------------------------
	case "get_mailforwards":
		out := ""
		for _, src := range sortedKeys(b.forwards) {
			out += "<item>" +
				mapItem("mail_forward_adress", src) +
				mapItem("mail_forward_targets", strings.Join(b.forwards[src], ",")) +
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

	// --- domains (read-only) -----------------------------------------------
	case "get_domains":
		out := ""
		for _, name := range sortedKeys(b.domains) {
			out += "<item>" +
				mapItem("domain_name", name) +
				mapItem("domain_path", b.domains[name]) +
				"</item>"
		}
		return out, ""

	// --- subdomains --------------------------------------------------------
	case "get_subdomains":
		out := ""
		for _, fqdn := range sortedKeys(b.subdomains) {
			out += "<item>" +
				mapItem("subdomain_name", fqdn) +
				mapItem("subdomain_path", b.subdomains[fqdn]) +
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

// --- SOAP helpers (the stable KAS wire format) -----------------------------

func envelope(inner string) string {
	return `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xmlns:xsd="http://www.w3.org/2001/XMLSchema">
 <SOAP-ENV:Body><ns1:Response xmlns:ns1="urn:test">` + inner + `</ns1:Response></SOAP-ENV:Body>
</SOAP-ENV:Envelope>`
}

func writeFault(w http.ResponseWriter, code string) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
 <SOAP-ENV:Body><SOAP-ENV:Fault>
  <faultcode>SOAP-ENV:Server</faultcode>
  <faultstring>`+code+`</faultstring>
 </SOAP-ENV:Fault></SOAP-ENV:Body>
</SOAP-ENV:Envelope>`)
}

func mapItem(key, value string) string {
	return "<item><key>" + key + "</key><value>" + value + "</value></item>"
}

func xmlUnescape(s string) string {
	r := strings.NewReplacer("&quot;", `"`, "&apos;", "'", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'", "&amp;", "&")
	return r.Replace(s)
}

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

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
