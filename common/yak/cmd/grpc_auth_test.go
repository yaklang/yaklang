package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGRPCSecretAuth(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		allowed      bool
	}{
		{"missing", "", false},
		{"malformed", "bearer", false},
		{"wrong_scheme", "basic correct-secret", false},
		{"empty_token", "bearer ", false},
		{"wrong_token", "bearer wrong-secret", false},
		{"correct", "bearer correct-secret", true},
		{"case_insensitive_scheme", "Bearer correct-secret", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.header != "" {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", tc.header))
			}
			got, err := newGRPCSecretAuth("correct-secret")(ctx)
			if tc.allowed {
				require.NoError(t, err)
				require.Equal(t, ctx, got)
			} else {
				require.Nil(t, got)
				require.Equal(t, codes.Unauthenticated, status.Code(err))
			}
		})
	}
}
