// Copyright (c) 2026 Johannes Küber
// SPDX-License-Identifier: MPL-2.0
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// listToStrings converts a types.List of strings into a Go slice; null and
// unknown lists yield nil.
func listToStrings(ctx context.Context, l types.List, diags *diag.Diagnostics) []string {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var out []string
	diags.Append(l.ElementsAs(ctx, &out, false)...)
	return out
}

// stringsToList converts a Go slice into a types.List of strings; an empty or
// nil slice yields a null list so it round-trips with optional attributes.
func stringsToList(ctx context.Context, in []string, diags *diag.Diagnostics) types.List {
	if len(in) == 0 {
		return types.ListNull(types.StringType)
	}
	l, d := types.ListValueFrom(ctx, types.StringType, in)
	diags.Append(d...)
	return l
}
