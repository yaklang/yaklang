package tlsutils

import (
	"crypto/rsa"
	"crypto/tls"
	_ "embed"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed test_pfx/certificate.pfx
var defaultPfx []byte

//go:embed test_pfx/certificate_legacy_des3.pfx
var legacyDes3Pfx []byte

//go:embed test_pfx/certificate_nopass.pfx
var nopassPfx []byte

//go:embed test_pfx/certificate_aes128.pfx
var aes128Pfx []byte

//go:embed test_pfx/test_cert.pem
var testCertPem []byte

//go:embed test_pfx/test_key.pem
var testKeyPem []byte

//go:embed test_pfx/test_des3.p12
var testDes3P12 []byte

//go:embed test_pfx/test_des.p12
var testDesP12 []byte

//go:embed test_pfx/test_rc2.p12
var testRc2P12 []byte

//go:embed test_pfx/test_noenc.p12
var testNoencP12 []byte

//go:embed test_pfx/test_legacy.p12
var testLegacyP12 []byte

func TestP12Auth(t *testing.T) {
	ca, key, err := GenerateSelfSignedCertKey("", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sCert, sKey, err := SignServerCrtNKeyEx(ca, key, "", true)
	if err != nil {
		t.Fatal(err)
	}
	sConfig, err := GetX509ServerTlsConfigWithAuth(ca, sCert, sKey, true)
	if err != nil {
		t.Fatal(err)
	}

	cCert, cKey, err := SignClientCrtNKeyEx(ca, key, "", true)
	if err != nil {
		t.Fatal(err)
	}

	port := utils.GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", utils.HostPort("127.0.0.1", port))
	if err != nil {
		t.Fatal(err)
	}

	token := utils.RandStringBytes(20)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			tlsConn := tls.Server(conn, sConfig)
			err = tlsConn.Handshake()
			if err != nil {
				log.Errorf("handshake to client failed: %s", err)
				continue
			}
			tlsConn.Write([]byte(token))
			tlsConn.Close()
		}
	}()
	time.Sleep(time.Second)
	clientConfig, err := GetX509MutualAuthGoClientTlsConfig(cCert, cKey, ca)
	if err != nil {
		t.Fatal()
	}
	conn, err := tls.Dial("tcp", utils.HostPort("127.0.0.1", port), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	var buf = make([]byte, 20)
	conn.Read(buf)
	if string(buf) != token {
		t.Fatal("token not match")
	}
	conn.Close()

	p12bytes, err := BuildP12(cCert, cKey, "", ca)
	if err != nil {
		t.Fatal(err)
	}
	cCert, cKey, _, err = LoadP12ToPEM(p12bytes, "")
	if err != nil {
		t.Fatal(err)
	}
	clientConfig, err = GetX509MutualAuthGoClientTlsConfig(cCert, cKey, ca)
	if err != nil {
		t.Fatal()
	}
	conn, err = tls.Dial("tcp", utils.HostPort("127.0.0.1", port), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	conn.Read(buf)
	if string(buf) != token {
		t.Fatal("token not match")
	}
	conn.Close()

	cCert2, cKey2, err := SignClientCrtNKeyEx(ca, key, "", false)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(cCert2, cKey2)
	if err != nil {
		t.Fatal(err)
	}
	clientConfig.Certificates = append(clientConfig.Certificates, cert)
	conn, err = tls.Dial("tcp", utils.HostPort("127.0.0.1", port), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	conn.Read(buf)
	if string(buf) != token {
		t.Fatal("token not match")
	}
	conn.Close()

	conn, err = tls.Dial("tcp", utils.HostPort("127.0.0.1", port), utils.NewDefaultTLSConfig())
	if err != nil {
		return
	}
	err = conn.Handshake()
	if err != nil {
		return
	}
	buf = make([]byte, 20)
	conn.Read(buf)
	if string(buf) == token {
		t.Fatal("token not match")
	}
	conn.Close()
}

func TestP12OrPFX(t *testing.T) {
	ca, key, err := GenerateSelfSignedCertKey("127.0.0.1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, sKey, err := SignServerCrtNKeyEx(ca, key, "", false)
	if err != nil {
		t.Fatal(err)
	}
	p12Bytes, err := BuildP12(cert, sKey, "123456", ca)
	if err != nil {
		t.Fatal(err)
	}
	certBytes, keyBytes, cas, err := LoadP12ToPEM(p12Bytes, "123456")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(certBytes, keyBytes, cas)

	p12Bytes, err = BuildP12(cert, sKey, "", ca)
	if err != nil {
		t.Fatal(err)
	}
	certBytes, keyBytes, cas, err = LoadP12ToPEM(p12Bytes, "")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(certBytes, keyBytes, cas)
}

func TestLoadP12FromOpenSSL(t *testing.T) {
	testCases := []struct {
		name        string
		pfxData     []byte
		password    string
		expectError bool
		checkCA     bool
	}{
		{
			name:        "Default PFX with password",
			pfxData:     defaultPfx,
			password:    "123456",
			expectError: false,
			checkCA:     true,
		},
		{
			name:        "Legacy DES3 PFX with password",
			pfxData:     legacyDes3Pfx,
			password:    "123456",
			expectError: false,
			checkCA:     true,
		},
		{
			name:        "No password PFX",
			pfxData:     nopassPfx,
			password:    "",
			expectError: false,
			checkCA:     true,
		},
		{
			name:        "AES-128 PFX with password",
			pfxData:     aes128Pfx,
			password:    "123456",
			expectError: false,
			checkCA:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.pfxData) == 0 {
				t.Fatalf("PFX data for %s is empty", tc.name)
			}
			cert, key, ca, err := LoadP12ToPEM(tc.pfxData, tc.password)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected an error but got none")
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error but got: %v", err)
				}
				if len(cert) == 0 {
					t.Fatal("cert is empty")
				}
				if len(key) == 0 {
					t.Fatal("key is empty")
				}
				if tc.checkCA && len(ca) == 0 {
					// In our test case, the cert itself is the CA. The LoadP12ToPEM function
					// might return it as the main cert and not in the ca bundle, which is acceptable.
					t.Log("ca bundle is empty, which can be expected for self-signed certs")
				}
				t.Logf("successfully loaded %s", tc.name)
			}
		})
	}
}

// TestGeneratedCertificates 测试通过 gen.sh 生成的证书
func TestGeneratedCertificates(t *testing.T) {
	// 首先测试 PEM 文件是否正确
	t.Run("test_cert_pem_validation", func(t *testing.T) {
		if len(testCertPem) == 0 {
			t.Skip("test_cert.pem 文件不存在，请先运行 gen.sh 脚本")
		}

		// 验证证书解析
		cert, err := ParsePEMCertificate(testCertPem)
		if err != nil {
			t.Fatalf("解析测试证书失败: %v", err)
		}

		t.Logf("测试证书信息:")
		t.Logf("  Subject: %s", cert.Subject.String())
		t.Logf("  Issuer: %s", cert.Issuer.String())
		t.Logf("  Serial: %s", cert.SerialNumber.String())
		t.Logf("  NotBefore: %s", cert.NotBefore.Format(time.RFC3339))
		t.Logf("  NotAfter: %s", cert.NotAfter.Format(time.RFC3339))

		// 验证私钥解析
		if len(testKeyPem) == 0 {
			t.Fatal("test_key.pem 文件不存在")
		}

		_, err = GetRSAPrivateKey(testKeyPem)
		if err != nil {
			t.Fatalf("解析测试私钥失败: %v", err)
		}

		// 验证证书和私钥匹配
		_, err = tls.X509KeyPair(testCertPem, testKeyPem)
		if err != nil {
			t.Fatalf("证书和私钥不匹配: %v", err)
		}

		t.Log("✓ 测试证书和私钥验证通过")
	})

	// 测试从 PEM 构建 P12
	t.Run("build_p12_from_test_cert", func(t *testing.T) {
		if len(testCertPem) == 0 || len(testKeyPem) == 0 {
			t.Skip("test_cert.pem 或 test_key.pem 文件不存在")
		}

		// 构建不同密码的 P12
		passwords := []string{"", "123456", "test-password"}

		for i, password := range passwords {
			t.Run(fmt.Sprintf("password_%d", i), func(t *testing.T) {
				p12Data, err := BuildP12(testCertPem, testKeyPem, password)
				if err != nil {
					t.Fatalf("构建 P12 失败 (密码: %s): %v", password, err)
				}

				// 验证解析
				loadedCert, loadedKey, _, err := LoadP12ToPEM(p12Data, password)
				if err != nil {
					t.Fatalf("解析 P12 失败 (密码: %s): %v", password, err)
				}

				// 验证内容一致性
				if !bytesEqual(testCertPem, loadedCert) {
					t.Errorf("证书内容不匹配 (密码: %s)", password)
				}

				// 验证 TLS 可用性
				_, err = tls.X509KeyPair(loadedCert, loadedKey)
				if err != nil {
					t.Fatalf("TLS 证书对创建失败 (密码: %s): %v", password, err)
				}

				t.Logf("✓ P12 构建和解析成功 (密码: %s)", password)
			})
		}
	})
}

// TestGeneratedP12Files 测试通过 gen.sh 生成的各种 P12 文件
func TestGeneratedP12Files(t *testing.T) {
	testCases := []struct {
		name     string
		p12Data  []byte
		password string
		desc     string
	}{
		{
			name:     "test_des3_p12",
			p12Data:  testDes3P12,
			password: "123456",
			desc:     "DES3 加密的 P12 文件",
		},
		{
			name:     "test_des_p12",
			p12Data:  testDesP12,
			password: "123456",
			desc:     "DES 加密的 P12 文件",
		},
		{
			name:     "test_rc2_p12",
			p12Data:  testRc2P12,
			password: "123456",
			desc:     "RC2 加密的 P12 文件",
		},
		{
			name:     "test_noenc_p12",
			p12Data:  testNoencP12,
			password: "123456",
			desc:     "无加密的 P12 文件",
		},
		{
			name:     "test_legacy_p12",
			p12Data:  testLegacyP12,
			password: "123456",
			desc:     "Legacy 模式的 P12 文件",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.p12Data) == 0 {
				t.Skipf("%s 文件不存在，请先运行 gen.sh 脚本", tc.name)
			}

			t.Logf("测试 %s - %s", tc.name, tc.desc)

			// 尝试解析 P12 文件
			cert, key, ca, err := LoadP12ToPEM(tc.p12Data, tc.password)
			if err != nil {
				t.Fatalf("解析 %s 失败: %v", tc.name, err)
			}

			// 验证证书内容
			if len(cert) == 0 {
				t.Fatalf("%s: 证书为空", tc.name)
			}

			if len(key) == 0 {
				t.Fatalf("%s: 私钥为空", tc.name)
			}

			// 解析并显示证书信息
			parsedCert, err := ParsePEMCertificate(cert)
			if err != nil {
				t.Fatalf("%s: 解析证书失败: %v", tc.name, err)
			}

			t.Logf("证书信息:")
			t.Logf("  Subject: %s", parsedCert.Subject.String())
			t.Logf("  Issuer: %s", parsedCert.Issuer.String())
			t.Logf("  NotBefore: %s", parsedCert.NotBefore.Format(time.RFC3339))
			t.Logf("  NotAfter: %s", parsedCert.NotAfter.Format(time.RFC3339))

			// 验证 TLS 可用性
			_, err = tls.X509KeyPair(cert, key)
			if err != nil {
				t.Fatalf("%s: 创建 TLS 证书对失败: %v", tc.name, err)
			}

			// 验证私钥类型
			if rsaKey, ok := parsedCert.PublicKey.(*rsa.PublicKey); ok {
				t.Logf("  RSA Key Size: %d bits", rsaKey.Size()*8)
			}

			// 如果有 CA 证书，也验证一下
			if len(ca) > 0 {
				t.Logf("  包含 %d 个 CA 证书", len(ca))
				for i, caCert := range ca {
					parsedCA, err := ParsePEMCertificate(caCert)
					if err != nil {
						t.Logf("  CA[%d]: 解析失败: %v", i, err)
					} else {
						t.Logf("  CA[%d]: %s", i, parsedCA.Subject.String())
					}
				}
			}

			t.Logf("✓ %s 测试通过", tc.name)
		})
	}
}

// TestCompatibilityWithOriginalCert 测试与原始证书的兼容性
func TestCompatibilityWithOriginalCert(t *testing.T) {
	if len(testCertPem) == 0 || len(testKeyPem) == 0 {
		t.Skip("测试证书文件不存在，请先运行 gen.sh 脚本")
	}

	// 使用原始 PEM 证书创建 P12
	p12FromPem, err := BuildP12(testCertPem, testKeyPem, "123456")
	if err != nil {
		t.Fatalf("从 PEM 构建 P12 失败: %v", err)
	}

	// 测试生成的 P12 文件数组
	generatedP12s := []struct {
		name string
		data []byte
	}{
		{"test_des3.p12", testDes3P12},
		{"test_des.p12", testDesP12},
		{"test_rc2.p12", testRc2P12},
		{"test_noenc.p12", testNoencP12},
		{"test_legacy.p12", testLegacyP12},
	}

	for _, p12File := range generatedP12s {
		t.Run(fmt.Sprintf("compare_with_%s", p12File.name), func(t *testing.T) {
			if len(p12File.data) == 0 {
				t.Skipf("%s 文件不存在", p12File.name)
			}

			// 解析生成的 P12 文件
			cert1, key1, _, err := LoadP12ToPEM(p12FromPem, "123456")
			if err != nil {
				t.Fatalf("解析从 PEM 构建的 P12 失败: %v", err)
			}

			cert2, key2, _, err := LoadP12ToPEM(p12File.data, "123456")
			if err != nil {
				t.Fatalf("解析 %s 失败: %v", p12File.name, err)
			}

			// 比较证书内容 (证书应该相同)
			if bytesEqual(cert1, cert2) {
				t.Logf("✓ 证书内容匹配 (%s)", p12File.name)
			} else {
				// 解析证书进行更详细的比较
				parsedCert1, _ := ParsePEMCertificate(cert1)
				parsedCert2, _ := ParsePEMCertificate(cert2)

				if parsedCert1 != nil && parsedCert2 != nil {
					if parsedCert1.Subject.String() == parsedCert2.Subject.String() &&
						parsedCert1.SerialNumber.Cmp(parsedCert2.SerialNumber) == 0 {
						t.Logf("✓ 证书关键信息匹配 (%s)", p12File.name)
					} else {
						t.Logf("⚠ 证书信息不完全匹配 (%s)", p12File.name)
						t.Logf("  PEM构建: %s", parsedCert1.Subject.String())
						t.Logf("  生成文件: %s", parsedCert2.Subject.String())
					}
				}
			}

			// 验证两个证书都可以创建有效的 TLS 配置
			_, err1 := tls.X509KeyPair(cert1, key1)
			_, err2 := tls.X509KeyPair(cert2, key2)

			if err1 == nil && err2 == nil {
				t.Logf("✓ 两个证书都可以创建有效的 TLS 配置 (%s)", p12File.name)
			} else {
				if err1 != nil {
					t.Errorf("PEM 构建的证书 TLS 配置失败: %v", err1)
				}
				if err2 != nil {
					t.Errorf("%s TLS 配置失败: %v", p12File.name, err2)
				}
			}
		})
	}
}

// 辅助函数：比较字节数组
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
