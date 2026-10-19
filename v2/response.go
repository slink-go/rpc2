// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package rpc2

type Response[T any] struct {
	Error  error
	Status int
	Data   T
}
