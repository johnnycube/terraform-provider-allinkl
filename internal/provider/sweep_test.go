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
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/johnnycube/kasapi"
)

// Sweepers delete leftover objects from interrupted acceptance runs against a
// real KAS account. Run them with, e.g.:
//
//	KAS_SWEEP_DOMAIN=test.example KAS_LOGIN=... KAS_PASSWORD=... \
//	  go test ./internal/provider/ -sweep=all
//
// They are a no-op unless KAS_SWEEP_DOMAIN is set, and only ever touch objects
// under that one disposable domain.

func init() {
	resource.AddTestSweepers("allinkl_mail_account", &resource.Sweeper{
		Name: "allinkl_mail_account",
		F:    sweepMailAccounts,
	})
	resource.AddTestSweepers("allinkl_mail_forward", &resource.Sweeper{
		Name: "allinkl_mail_forward",
		F:    sweepMailForwards,
	})
	resource.AddTestSweepers("allinkl_subdomain", &resource.Sweeper{
		Name: "allinkl_subdomain",
		F:    sweepSubdomains,
	})
}

func sweepClient() (*kasapi.Client, string, error) {
	domain := os.Getenv("KAS_SWEEP_DOMAIN")
	if domain == "" {
		return nil, "", fmt.Errorf("set KAS_SWEEP_DOMAIN to the disposable test domain to sweep")
	}
	authType := os.Getenv("KAS_AUTH_TYPE")
	if authType == "" {
		authType = string(kasapi.AuthSHA1)
	}
	c, err := kasapi.New(kasapi.Config{
		Login:        os.Getenv("KAS_LOGIN"),
		Password:     os.Getenv("KAS_PASSWORD"),
		AuthType:     kasapi.AuthType(authType),
		UserAgent:    "terraform-provider-allinkl/sweeper",
		APIEndpoint:  os.Getenv("KAS_API_ENDPOINT"),
		AuthEndpoint: os.Getenv("KAS_AUTH_ENDPOINT"),
	})
	return c, domain, err
}

func sweepMailAccounts(string) error {
	c, domain, err := sweepClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	accounts, err := c.Mail.ListAccounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		if a.Domain != domain {
			continue
		}
		if err := c.Mail.DeleteAccount(ctx, a.Login); err != nil {
			return fmt.Errorf("deleting mail account %s: %w", a.Login, err)
		}
	}
	return nil
}

func sweepMailForwards(string) error {
	c, domain, err := sweepClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	forwards, err := c.Mail.ListForwards(ctx)
	if err != nil {
		return err
	}
	for _, f := range forwards {
		if f.Domain != domain {
			continue
		}
		if err := c.Mail.DeleteForward(ctx, f.Source()); err != nil {
			return fmt.Errorf("deleting mail forward %s: %w", f.Source(), err)
		}
	}
	return nil
}

func sweepSubdomains(string) error {
	c, domain, err := sweepClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	subs, err := c.Subdomains.List(ctx)
	if err != nil {
		return err
	}
	suffix := "." + domain
	for _, s := range subs {
		if !strings.HasSuffix(s.FQDN, suffix) {
			continue
		}
		if err := c.Subdomains.Delete(ctx, s.FQDN); err != nil {
			return fmt.Errorf("deleting subdomain %s: %w", s.FQDN, err)
		}
	}
	return nil
}
