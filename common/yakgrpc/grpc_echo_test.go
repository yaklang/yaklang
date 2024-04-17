package yakgrpc

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/yaklang/yaklang/common/crep"
	"testing"
)

func Test_verify(t *testing.T) {
	type args struct {
		serConfig *tls.Config
		cliConfig *tls.Config
		domain    string
		useRoot   bool
	}
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
			caCert, caKey, _ := crep.GetDefaultCAAndPriv()
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

func TestServer_VerifySystemCertificate(t *testing.T) {
	err := crep.VerifySystemCertificate()
	if err != nil {
		t.Fatal(err)
	}
}
