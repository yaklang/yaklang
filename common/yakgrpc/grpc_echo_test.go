package yakgrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/crep"
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

var callCount int
var mu sync.Mutex

func mockVerifySystemCertificate() (*ypb.VerifySystemCertificateResponse, error) {
	mu.Lock()
	callCount++
	time.Sleep(1000 * time.Millisecond)
	mu.Unlock()
	return &ypb.VerifySystemCertificateResponse{Valid: true}, nil
}

func mockVerifySystemCertificateNil() (*ypb.VerifySystemCertificateResponse, error) {
	mu.Lock()
	callCount++
	time.Sleep(1000 * time.Millisecond)
	mu.Unlock()
	return nil, nil
}

func TestVerifySystemCertificateCooldown(t *testing.T) {
	callCount = 0
	resp = nil

	// mock
	verifyFunction = mockVerifySystemCertificate

	server := &Server{}
	sw := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		sw.Add(1)
		go func() {
			defer sw.Done()
			_, _ = server.VerifySystemCertificate(context.Background(), &ypb.Empty{})
		}()
		time.Sleep(1 * time.Second)
	}

	// 等待足够的时间以确保所有协程都已完成
	sw.Wait()

	time.Sleep(2 * time.Second)
	verifyFunction = mockVerifySystemCertificateNil
	// spinlock 结束，cooldown 时间内，不会增加count
	_, _ = server.VerifySystemCertificate(context.Background(), &ypb.Empty{})

	// 检查 mockVerifySystemCertificate 是否只被调用了一次 CI卡顿可能不稳定导致数量大于1
	mu.Lock()
	if callCount > 2 {
		t.Errorf("verifySystemCertificate was called %d times; want 1", callCount)
	}
	mu.Unlock()
}

func TestVerifySystemCertificateCooldown2(t *testing.T) {
	callCount = 0
	resp = nil

	// mock
	verifyFunction = mockVerifySystemCertificateNil

	server := &Server{}
	sw := sync.WaitGroup{}
	for i := 0; i < 5; i++ {
		sw.Add(1)
		go func() {
			defer sw.Done()
			_, _ = server.VerifySystemCertificate(context.Background(), &ypb.Empty{})
		}()
		time.Sleep(1 * time.Second)
	}

	// 等待足够的时间以确保所有协程都已完成
	sw.Wait()

	time.Sleep(2 * time.Second)
	// spinlock 超时，会增加count
	_, _ = server.VerifySystemCertificate(context.Background(), &ypb.Empty{})

	mu.Lock()
	if callCount > 3 || callCount < 2 {
		t.Errorf("verifySystemCertificate was called %d times; want 1", callCount)
	}
	mu.Unlock()
}
