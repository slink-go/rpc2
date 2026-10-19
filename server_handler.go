// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2025-2026 Mikhail Kantur

package rpc2

import (
	"context"
	"go.slink.ws/rpc"
)

type ServerHandler interface {
	Handle(ctx context.Context, codec rpc.ServerCodec)
}
