// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Command fakekas is a standalone, in-memory fake of the all-inkl.com KAS SOAP
// API, so the provider can run a real plan/apply on this machine without a KAS
// account. KasAuth issues a session token (password defaults to "secret");
// KasApi validates the token and delegates to the state machine in
// internal/fakekas, which the acceptance tests use as well.
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
	"strings"
	"time"

	"github.com/johnnycube/terraform-provider-allinkl/internal/fakekas"
)

const (
	password = "secret"          // plaintext the fake expects (any login)
	token    = "session-token-1" // session token KasAuth issues
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8511", "address to listen on")
	flag.Parse()

	b := &server{backend: fakekas.New()}
	// Seed a little server-side state so the read-only data sources return
	// something on a fresh apply.
	b.backend.SeedDomain("example.com", "/")
	b.backend.SeedDomain("example.org", "/example_org/")
	b.backend.SeedFTPUser("w0123456", "/", "account login", true)

	mux := http.NewServeMux()
	mux.HandleFunc("/KasAuth.php", b.serve)
	mux.HandleFunc("/KasApi.php", b.serve)

	log.Printf("fake KAS listening on http://%s (login: any, password: %q)", *addr, password)
	log.Printf("  KAS_AUTH_ENDPOINT=http://%s/KasAuth.php", *addr)
	log.Printf("  KAS_API_ENDPOINT=http://%s/KasApi.php", *addr)
	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

var paramsRe = regexp.MustCompile(`(?s)<Params[^>]*>(.*?)</Params>`)

// server speaks the KAS SOAP wire format over the shared backend.
type server struct {
	backend *fakekas.Backend
}

func (b *server) serve(w http.ResponseWriter, r *http.Request) {
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
		_, _ = fmt.Fprint(w, envelope(`<return xsi:type="xsd:string">`+token+`</return>`))
		return
	}

	if req["kas_auth_data"] != token {
		writeFault(w, "session_expired")
		return
	}

	action, _ := req["kas_action"].(string)
	params, _ := req["KasRequestParams"].(map[string]any)

	info, fault := b.backend.Handle(action, params)
	if fault != "" {
		writeFault(w, fault)
		return
	}
	_, _ = fmt.Fprint(w, envelope(`<return>
  <item><key>Request</key><value></value></item>
  <item><key>Response</key><value>
    <item><key>ReturnString</key><value>TRUE</value></item>
    <item><key>ReturnInfo</key><value>`+info+`</value></item>
  </value></item>
  <item><key>KasFloodDelay</key><value>0.01</value></item>
</return>`))
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
	_, _ = fmt.Fprint(w, `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
 <SOAP-ENV:Body><SOAP-ENV:Fault>
  <faultcode>SOAP-ENV:Server</faultcode>
  <faultstring>`+code+`</faultstring>
 </SOAP-ENV:Fault></SOAP-ENV:Body>
</SOAP-ENV:Envelope>`)
}

func xmlUnescape(s string) string {
	r := strings.NewReplacer("&quot;", `"`, "&apos;", "'", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'", "&amp;", "&")
	return r.Replace(s)
}
