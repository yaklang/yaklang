package main

import (
	"context"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/grpc_auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newGRPCSecretAuth(secret string) grpc_auth.AuthFunc {
	return func(ctx context.Context) (context.Context, error) {
		method, _ := grpc.Method(ctx)
		userSecret, err := grpc_auth.AuthFromMD(ctx, "bearer")
		if err != nil {
			// A rejected request is not a server startup failure. Include the RPC
			// so callers can identify premature/unauthenticated connection probes.
			log.Warnf("gRPC authentication rejected: method=%s reason=missing or malformed Bearer authorization", method)
			return nil, err
		}
		if userSecret != secret {
			log.Warnf("gRPC authentication rejected: method=%s reason=invalid credentials", method)
			return nil, status.Error(codes.Unauthenticated, "secret verify failed")
		}
		return ctx, nil
	}
}
