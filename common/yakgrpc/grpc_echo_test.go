package yakgrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func Test_verify(t *testing.T) {
	type args struct {
		serConfig *tls.Config
		cliConfig *tls.Config
		domain    string
		useRoot   bool
	}
	pool := x509.NewCertPool()
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "根证书验证",
			args: args{
				serConfig: &tls.Config{},
				cliConfig: &tls.Config{
					ServerName: "www.example.com",
					MinVersion: tls.VersionSSL30, // nolint[:staticcheck]
					MaxVersion: tls.VersionTLS13,
				},
				useRoot: true,
				domain:  "www.example.com",
			},
			wantErr: false,
		},
		{
			name: "根证书验证 2",
			args: args{
				serConfig: &tls.Config{},
				cliConfig: &tls.Config{
					ServerName: "www.baidu.com",
					MinVersion: tls.VersionSSL30, // nolint[:staticcheck]
					MaxVersion: tls.VersionTLS13,
				},
				useRoot: true,
				domain:  "www.example.com",
			},
			wantErr: true,
		},
		{
			name: "未加入系统根证书池",
			args: args{
				serConfig: nil,
				cliConfig: &tls.Config{
					ServerName: "www.example.com",
					MinVersion: tls.VersionSSL30,
					MaxVersion: tls.VersionTLS13,
					// 本地安装了 yakit mitm 证书的情况下，不覆盖 root ca 会导致tls正常发送
					RootCAs: pool,
				},
				useRoot: false,
				domain:  "www.example.com",
			},
			// tls: failed to verify certificate: x509: certificate signed by unknown authority
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crep.InitMITMCert()
			caCert, caKey, _ := crep.GetDefaultMITMCAAndPriv()
			fakeCert, err := crep.FakeCertificateByHost(caCert, caKey, tt.args.domain)
			if err != nil {
				t.Fatal(err)
			}
			cConfig := tt.args.cliConfig
			sConfig := tt.args.serConfig
			if tt.args.useRoot {
				pool := x509.NewCertPool()
				pool.AddCert(caCert) // 信任根证书和信任子证书应当都正常
				//pool.AddCert(fakeCert.Leaf)
				cConfig.RootCAs = pool
				sConfig.Certificates = []tls.Certificate{fakeCert}
			}

			err = verify(sConfig, cConfig, tt.args.domain)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unexpected error status: got %v, want %v", err != nil, tt.wantErr)
			}
		})
	}
}

func setupCertificateVerifier(t *testing.T, cooldown time.Duration, verify func() (*ypb.VerifySystemCertificateResponse, error)) {
	t.Helper()
	originalCD, originalVerify := VerifySystemCertificateCD, verifyFunction
	originalResp, originalDone, originalErr := resp, verifyResultDone, verifyResultErr
	cd := utils.NewCoolDown(cooldown)
	VerifySystemCertificateCD, verifyFunction = cd, verify
	resp, verifyResultDone, verifyResultErr = nil, nil, nil
	t.Cleanup(func() {
		cd.Close()
		VerifySystemCertificateCD, verifyFunction = originalCD, originalVerify
		resp, verifyResultDone, verifyResultErr = originalResp, originalDone, originalErr
	})
}

func TestVerifySystemCertificateCooldown(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	setupCertificateVerifier(t, time.Minute, func() (*ypb.VerifySystemCertificateResponse, error) {
		calls.Add(1)
		close(started)
		<-release
		return &ypb.VerifySystemCertificateResponse{Valid: true}, nil
	})
	server := &Server{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	results := make(chan *ypb.VerifySystemCertificateResponse, 5)
	var wg sync.WaitGroup
	invoke := func() {
		defer wg.Done()
		result, err := server.VerifySystemCertificate(ctx, &ypb.Empty{})
		assert.NoError(t, err)
		results <- result
	}
	wg.Add(1)
	go invoke()
	<-started
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go invoke()
	}
	close(release)
	wg.Wait()
	close(results)
	for result := range results {
		require.True(t, result.GetValid())
	}
	require.Equal(t, int32(1), calls.Load(), "concurrent callers must share verification")
}

func TestVerifySystemCertificateCooldown2(t *testing.T) {
	var calls atomic.Int32
	setupCertificateVerifier(t, 10*time.Millisecond, func() (*ypb.VerifySystemCertificateResponse, error) {
		calls.Add(1)
		return nil, nil
	})
	server := &Server{}
	result, err := server.VerifySystemCertificate(context.Background(), &ypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, "Timeout", result.GetReason())
	require.Equal(t, int32(1), calls.Load())
	require.Eventually(t, func() bool {
		result, err = server.VerifySystemCertificate(context.Background(), &ypb.Empty{})
		return err == nil && result.GetReason() == "Timeout" && calls.Load() == 2
	}, time.Second, time.Millisecond, "nil results must finish promptly and allow retry after cooldown")
}

func TestInstallMITMCertificate(t *testing.T) {
	originalReady := checkMITMInstallReadyFunc
	t.Cleanup(func() { checkMITMInstallReadyFunc = originalReady })
	checkMITMInstallReadyFunc = func() (bool, string) { return true, "" }
	defer func(origInstall func() error, origVerify func() error) {
		installMITMCertFunc = origInstall
		verifyInstalledCertFunc = origVerify
	}(installMITMCertFunc, verifyInstalledCertFunc)

	server := &Server{}
	tests := []struct {
		name           string
		installErr     error
		verifyErr      error
		wantOK         bool
		expectedReason string
	}{
		{
			name:   "install success and verified",
			wantOK: true,
		},
		{
			name:           "install failed",
			installErr:     utils.Error("install failed"),
			wantOK:         false,
			expectedReason: "install failed",
		},
		{
			name:           "verify failed",
			verifyErr:      utils.Error("verify failed"),
			wantOK:         false,
			expectedReason: "verification failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installMITMCertFunc = func() error {
				return tt.installErr
			}
			verifyInstalledCertFunc = func() error {
				return tt.verifyErr
			}

			resp, err := server.InstallMITMCertificate(context.Background(), &ypb.Empty{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.GetOk() != tt.wantOK {
				t.Fatalf("expected ok=%v got=%v", tt.wantOK, resp.GetOk())
			}
			if !tt.wantOK && tt.expectedReason != "" && !strings.Contains(resp.GetReason(), tt.expectedReason) {
				t.Fatalf("expected reason to contain %q got %q", tt.expectedReason, resp.GetReason())
			}
		})
	}
}
