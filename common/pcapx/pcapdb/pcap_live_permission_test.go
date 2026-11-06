package pcapdb

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/pcap"
)

func TestLiveCapturePermissionAuthorization(t *testing.T) {
	denied := errors.New("lo0: You don't have permission to capture on that device ((cannot open BPF device) /dev/bpf0: Permission denied)")
	cancelled := errors.New("User cancelled operation")
	retryFailure := errors.New("device disappeared")
	driverFailure := errors.New("libpcap is unavailable")
	for _, test := range []struct {
		name                       string
		openErr, authErr, retryErr error
		cancelBefore, cancelDuring bool
		wantOpens, wantAuth        int
		wantErr                    error
	}{
		{name: "already permitted", wantOpens: 1},
		{name: "macOS BPF authorization", openErr: denied, wantOpens: 2, wantAuth: 1},
		{name: "typed permission error", openErr: os.ErrPermission, wantOpens: 2, wantAuth: 1},
		{name: "raw socket permission", openErr: errors.New("socket: Operation not permitted"), wantOpens: 2, wantAuth: 1},
		{name: "Npcap permission", openErr: errors.New("Error opening adapter: Access is denied. (5)"), wantOpens: 2, wantAuth: 1},
		{name: "missing device", openErr: retryFailure, wantOpens: 1, wantErr: retryFailure},
		{name: "driver unavailable", openErr: driverFailure, wantOpens: 1, wantErr: driverFailure},
		{name: "user rejects authorization", openErr: denied, authErr: cancelled, wantOpens: 1, wantAuth: 1, wantErr: cancelled},
		{name: "repair does not grant access", openErr: denied, retryErr: denied, wantOpens: 2, wantAuth: 1, wantErr: denied},
		{name: "retry reports actual error", openErr: denied, retryErr: retryFailure, wantOpens: 2, wantAuth: 1, wantErr: retryFailure},
		{name: "cancelled owner", cancelBefore: true, wantErr: context.Canceled},
		{name: "cancelled during authorization", openErr: denied, cancelDuring: true, wantOpens: 1, wantAuth: 1, wantErr: context.Canceled},
		{name: "cancelled authorization process", openErr: denied, authErr: cancelled, cancelDuring: true, wantOpens: 1, wantAuth: 1, wantErr: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancelBefore {
				cancel()
			}
			opens, auths := 0, 0
			// The sentinel is never used as a native handle: this test exercises
			// open/authorize/retry decisions without triggering a system dialog.
			expected := &pcap.Handle{}
			handle, err := openLiveWithPermission(ctx, func() (*pcap.Handle, error) {
				opens++
				if opens == 1 && test.openErr != nil {
					return nil, test.openErr
				}
				if opens > 1 && test.retryErr != nil {
					return nil, test.retryErr
				}
				return expected, nil
			}, func(authContext context.Context) error {
				auths++
				require.Equal(t, ctx, authContext)
				if test.cancelDuring {
					cancel()
				}
				return test.authErr
			})
			require.Equal(t, test.wantOpens, opens)
			require.Equal(t, test.wantAuth, auths)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				require.Nil(t, handle)
			} else {
				require.NoError(t, err)
				require.Same(t, expected, handle)
			}
		})
	}
}
