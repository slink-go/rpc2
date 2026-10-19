// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package rpc2

import "encoding/gob"

type CallError struct {
	Message string
}

func NewCallError(message string) *CallError {
	return &CallError{Message: message}
}

func (e *CallError) Error() string {
	return e.Message
}

func init() {
	gob.Register(CallError{})
}
