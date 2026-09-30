// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/johnnycube/terraform-provider-allinkl/internal/fakekas"
)

// Shared checks for the acceptance tests.

func checkPassword(backend *fakekas.Backend, resourceName, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not in state", resourceName)
		}
		if got := backend.PasswordOf(rs.Primary.ID); got != want {
			return fmt.Errorf("API holds password %q for %s, want %q", got, rs.Primary.ID, want)
		}
		return nil
	}
}

func checkCount(backend *fakekas.Backend, kind string, want int) func(*terraform.State) error {
	return func(*terraform.State) error {
		if n := backend.Count(kind); n != want {
			return fmt.Errorf("expected %d %s, got %d", want, kind, n)
		}
		return nil
	}
}
