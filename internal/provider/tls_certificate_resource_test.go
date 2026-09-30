// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testCert issues a self-signed certificate for host and returns certificate
// and key as PEM.
func testCert(t *testing.T, host string) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
}

// heredoc renders a PEM block as an HCL heredoc without the trailing newline,
// the form KAS hands back, so imports verify byte for byte.
func heredoc(pem string) string {
	return "chomp(<<-EOT\n" + strings.TrimSpace(pem) + "\nEOT\n)"
}

func TestAccTLSCertificate_lifecycle(t *testing.T) {
	backend := startFakeKAS(t)
	backend.SeedDomain("example.com", "/")
	cert, key := testCert(t, "example.com")
	cert2, key2 := testCert(t, "example.com")

	config := func(cert, key string, extra string) string {
		return testAccProviderConfig + fmt.Sprintf(`
resource "allinkl_tls_certificate" "main" {
  host        = "example.com"
  certificate = %s
  private_key = %s
%s}`, heredoc(cert), heredoc(key), extra)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(cert, key, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_tls_certificate.main", "id", "example.com"),
					resource.TestCheckResourceAttr("allinkl_tls_certificate.main", "active", "true"),
					resource.TestCheckResourceAttr("allinkl_tls_certificate.main", "force_https", "false"),
					resource.TestCheckResourceAttr("allinkl_tls_certificate.main", "hsts_max_age", "-1"),
					resource.TestCheckResourceAttr("allinkl_tls_certificate.main", "lets_encrypt", "false"),
					func(_ *terraform.State) error {
						crt, k, active := backend.HostTLS("example.com")
						if !samePEM(crt, cert) || !samePEM(k, key) || !active {
							return fmt.Errorf("backend holds a different certificate or key, active=%v", active)
						}
						return nil
					},
				),
			},
			// Settings change without re-sending the material.
			{
				Config: config(cert, key, "  force_https  = true\n  hsts_max_age = 31536000\n"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("allinkl_tls_certificate.main", "force_https", "true"),
					resource.TestCheckResourceAttr("allinkl_tls_certificate.main", "hsts_max_age", "31536000"),
				),
			},
			// Renewal: a new pair.
			{
				Config: config(cert2, key2, "  force_https  = true\n  hsts_max_age = 31536000\n"),
				Check: func(_ *terraform.State) error {
					crt, _, _ := backend.HostTLS("example.com")
					if !samePEM(crt, cert2) {
						return fmt.Errorf("renewed certificate not installed")
					}
					return nil
				},
			},
			{
				ResourceName:            "allinkl_tls_certificate.main",
				ImportState:             true,
				ImportStateId:           "example.com",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"private_key"},
			},
		},
		CheckDestroy: func(_ *terraform.State) error {
			if _, _, active := backend.HostTLS("example.com"); active {
				return fmt.Errorf("certificate still active after destroy")
			}
			return nil
		},
	})
}

func TestAccTLSCertificate_validation(t *testing.T) {
	startFakeKAS(t)
	cert, key := testCert(t, "example.com")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "allinkl_tls_certificate" "bad" {
  host        = "example.com"
  certificate = "not pem"
  private_key = "not pem"
}`,
				ExpectError: regexp.MustCompile(`must be PEM-encoded`),
			},
			{
				// The certificate does not name the host.
				Config: testAccProviderConfig + fmt.Sprintf(`
resource "allinkl_tls_certificate" "wrong_host" {
  host        = "other.example"
  certificate = %s
  private_key = %s
}`, heredoc(cert), heredoc(key)),
				ExpectError: regexp.MustCompile(`does not cover other.example`),
			},
		},
	})
}
