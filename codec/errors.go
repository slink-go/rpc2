// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package codec

import "errors"

var (
	InvalidDataErr   = errors.New("invalid input data")
	InvalidPrefixErr = errors.New("invalid data prefix")
	PrefixReadErr    = errors.New("prefix read error")
)
