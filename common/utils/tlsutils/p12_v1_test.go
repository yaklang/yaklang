package tlsutils

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	_ "embed"
	"testing"
	"time"
)

//go:embed test_p12_v1/test_cert_v1.pem
var testCertV1Pem []byte

//go:embed test_p12_v1/test_key_v1.pem
var testKeyV1Pem []byte

//go:embed test_p12_v1/test_aes256_v1.p12
var testAes256V1P12 []byte

//go:embed test_p12_v1/test_aes128_v1.p12
var testAes128V1P12 []byte

//go:embed test_p12_v1/test_des3_v1.p12
var testDes3V1P12 []byte

//go:embed test_p12_v1/test_nopass_v1.p12
var testNopassV1P12 []byte

//go:embed test_p12_v1/test_noenc_v1.p12
var testNoencV1P12 []byte

//go:embed test_p12_v1/test_legacy_v1.p12
var testLegacyV1P12 []byte

// TestOpenSSLV1GeneratedP12Files 测试通过 v1_gen.sh 生成的各种 P12 文件
func TestOpenSSLV1GeneratedP12Files(t *testing.T) {
	testCases := []struct {
		name     string
		p12Data  []byte
		password string
		desc     string
	}{
		{
			name:     "test_aes256_v1_p12",
			p12Data:  testAes256V1P12,
			password: "123456",
			desc:     "OpenSSL v1 AES-256 加密的 P12 文件",
		},
		{
			name:     "test_aes128_v1_p12",
			p12Data:  testAes128V1P12,
			password: "123456",
			desc:     "OpenSSL v1 AES-128 加密的 P12 文件",
		},
		{
			name:     "test_des3_v1_p12",
			p12Data:  testDes3V1P12,
			password: "123456",
			desc:     "OpenSSL v1 DES3 加密的 P12 文件",
		},
		{
			name:     "test_nopass_v1_p12",
			p12Data:  testNopassV1P12,
			password: "",
			desc:     "OpenSSL v1 无密码的 P12 文件",
		},
		{
			name:     "test_noenc_v1_p12",
			p12Data:  testNoencV1P12,
			password: "123456",
			desc:     "OpenSSL v1 无加密的 P12 文件",
		},
		{
			name:     "test_legacy_v1_p12",
			p12Data:  testLegacyV1P12,
			password: "123456",
			desc:     "OpenSSL v1 Legacy 模式的 P12 文件",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.p12Data) == 0 {
				t.Skipf("%s 文件不存在，请先运行 v1_gen.sh 脚本", tc.name)
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

// TestCompareV1V3Certificates 比较 OpenSSL v1 和 v3 生成的证书差异
func TestCompareV1V3Certificates(t *testing.T) {
	// 检查文件是否存在
	if len(testCertV1Pem) == 0 || len(testKeyV1Pem) == 0 {
		t.Skip("OpenSSL v1 证书文件不存在，请先运行 v1_gen.sh 脚本")
	}
	if len(testCertPem) == 0 || len(testKeyPem) == 0 {
		t.Skip("OpenSSL v3 证书文件不存在，请先运行 v3_gen.sh 脚本")
	}

	// 比较 V1 和 V3 的 P12 格式
	v1Formats := []struct {
		name     string
		data     []byte
		password string
	}{
		{"AES-256 (v1)", testAes256V1P12, "123456"},
		{"Legacy (v1)", testLegacyV1P12, "123456"},
	}

	v3Formats := []struct {
		name     string
		data     []byte
		password string
	}{
		{"DES3 (v3)", testDes3P12, "123456"},
		{"Legacy (v3)", testLegacyP12, "123456"},
	}

	t.Run("v1_vs_v3_formats", func(t *testing.T) {
		// 测试 V1 格式
		for _, v1Format := range v1Formats {
			if len(v1Format.data) == 0 {
				continue
			}

			cert, key, _, err := LoadP12ToPEM(v1Format.data, v1Format.password)
			if err != nil {
				t.Logf("❌ %s 解析失败: %v", v1Format.name, err)
			} else {
				_, err = tls.X509KeyPair(cert, key)
				if err != nil {
					t.Logf("❌ %s TLS 配置失败: %v", v1Format.name, err)
				} else {
					t.Logf("✓ %s 解析成功", v1Format.name)
				}
			}
		}

		// 测试 V3 格式
		for _, v3Format := range v3Formats {
			if len(v3Format.data) == 0 {
				continue
			}

			cert, key, _, err := LoadP12ToPEM(v3Format.data, v3Format.password)
			if err != nil {
				t.Logf("❌ %s 解析失败: %v", v3Format.name, err)
			} else {
				_, err = tls.X509KeyPair(cert, key)
				if err != nil {
					t.Logf("❌ %s TLS 配置失败: %v", v3Format.name, err)
				} else {
					t.Logf("✓ %s 解析成功", v3Format.name)
				}
			}
		}
	})

	// 比较证书内容
	t.Run("compare_cert_content", func(t *testing.T) {
		v1Cert, err := ParsePEMCertificate(testCertV1Pem)
		if err != nil {
			t.Fatalf("解析 v1 证书失败: %v", err)
		}

		v3Cert, err := ParsePEMCertificate(testCertPem)
		if err != nil {
			t.Fatalf("解析 v3 证书失败: %v", err)
		}

		t.Logf("OpenSSL v1 证书信息:")
		t.Logf("  Subject: %s", v1Cert.Subject.String())
		t.Logf("  Issuer: %s", v1Cert.Issuer.String())
		t.Logf("  Serial: %s", v1Cert.SerialNumber.String())

		t.Logf("OpenSSL v3 证书信息:")
		t.Logf("  Subject: %s", v3Cert.Subject.String())
		t.Logf("  Issuer: %s", v3Cert.Issuer.String())
		t.Logf("  Serial: %s", v3Cert.SerialNumber.String())

		// 检查私钥格式差异
		v1KeyType := "未知"
		if _, ok := v1Cert.PublicKey.(*rsa.PublicKey); ok {
			v1KeyType = "RSA"
		}

		v3KeyType := "未知"
		if _, ok := v3Cert.PublicKey.(*rsa.PublicKey); ok {
			v3KeyType = "RSA"
		}

		t.Logf("密钥类型比较: v1=%s, v3=%s", v1KeyType, v3KeyType)
	})
}

// TestCrossVersionCompatibility 测试跨版本兼容性
func TestCrossVersionCompatibility(t *testing.T) {
	// 测试使用 v1 证书和密钥创建 P12，然后用我们的库解析
	if len(testCertV1Pem) == 0 || len(testKeyV1Pem) == 0 {
		t.Skip("OpenSSL v1 证书文件不存在，请先运行 v1_gen.sh 脚本")
	}

	t.Run("build_p12_from_v1_cert", func(t *testing.T) {
		// 使用我们的库从 v1 证书构建 P12
		p12Data, err := BuildP12(testCertV1Pem, testKeyV1Pem, "123456")
		if err != nil {
			t.Fatalf("从 v1 PEM 构建 P12 失败: %v", err)
		}

		// 再解析回来
		cert, key, _, err := LoadP12ToPEM(p12Data, "123456")
		if err != nil {
			t.Fatalf("解析构建的 P12 失败: %v", err)
		}

		// 验证 TLS 可用性
		_, err = tls.X509KeyPair(cert, key)
		if err != nil {
			t.Fatalf("TLS 证书对创建失败: %v", err)
		}

		t.Log("✓ 从 OpenSSL v1 证书构建 P12 并成功解析")

		// 比较原始证书和解析后的证书
		if !bytesEqual(testCertV1Pem, cert) {
			t.Log("⚠ 解析后的证书与原始证书不完全相同")

			// 进一步分析差异
			origCert, _ := ParsePEMCertificate(testCertV1Pem)
			parsedCert, _ := ParsePEMCertificate(cert)

			if origCert != nil && parsedCert != nil {
				if origCert.Subject.String() == parsedCert.Subject.String() &&
					origCert.SerialNumber.Cmp(parsedCert.SerialNumber) == 0 {
					t.Log("✓ 但证书关键信息匹配")
				}
			}
		} else {
			t.Log("✓ 解析后的证书与原始证书完全相同")
		}
	})

	// 尝试解析所有 v1 格式的 P12 文件
	t.Run("parse_all_v1_formats", func(t *testing.T) {
		formats := []struct {
			name     string
			data     []byte
			password string
		}{
			{"AES-256 (v1)", testAes256V1P12, "123456"},
			{"AES-128 (v1)", testAes128V1P12, "123456"},
			{"DES3 (v1)", testDes3V1P12, "123456"},
			{"No Password (v1)", testNopassV1P12, ""},
			{"No Encryption (v1)", testNoencV1P12, "123456"},
			{"Legacy (v1)", testLegacyV1P12, "123456"},
		}

		for _, format := range formats {
			if len(format.data) == 0 {
				t.Logf("跳过 %s (文件不存在)", format.name)
				continue
			}

			cert, key, _, err := LoadP12ToPEM(format.data, format.password)
			if err != nil {
				t.Logf("❌ %s 解析失败: %v", format.name, err)
			} else {
				_, err = tls.X509KeyPair(cert, key)
				if err != nil {
					t.Logf("❌ %s TLS 配置失败: %v", format.name, err)
				} else {
					t.Logf("✓ %s 解析成功", format.name)
				}
			}
		}
	})
}

// 原有的嵌入文件声明...

//go:embed test_p12_v1/test_mac_md5_v1.p12
var testMacMd5V1P12 []byte

//go:embed test_p12_v1/test_shortpass_v1.p12
var testShortPassV1P12 []byte

//go:embed test_p12_v1/test_longpass_v1.p12
var testLongPassV1P12 []byte

//go:embed test_p12_v1/test_utf8pass_v1.p12
var testUtf8PassV1P12 []byte

//go:embed test_p12_v1/test_rc4_v1.p12
var testRc4V1P12 []byte

//go:embed test_p12_v1/test_weak_v1.p12
var testWeakV1P12 []byte

//go:embed test_p12_v1/test_multicert_v1.p12
var testMultiCertV1P12 []byte

//go:embed test_p12_v1/test_ec_v1.p12
var testEcV1P12 []byte

//go:embed test_p12_v1/test_dsa_v1.p12
var testDsaV1P12 []byte

// TestEdgeCaseP12Files 测试边缘情况的P12文件
func TestEdgeCaseP12Files(t *testing.T) {
	testCases := []struct {
		name     string
		p12Data  []byte
		password string
		desc     string
	}{
		{
			name:     "test_mac_md5_v1_p12",
			p12Data:  testMacMd5V1P12,
			password: "123456",
			desc:     "使用MD5作为MAC算法的P12文件",
		},
		{
			name:     "test_shortpass_v1_p12",
			p12Data:  testShortPassV1P12,
			password: "a",
			desc:     "使用极短密码的P12文件",
		},
		{
			name:    "test_longpass_v1_p12",
			p12Data: testLongPassV1P12,
			password: func() string {
				s := ""
				for i := 0; i < 100; i++ {
					s += "a"
				}
				return s
			}(),
			desc: "使用极长密码(100个a)的P12文件",
		},
		{
			name:     "test_utf8pass_v1_p12",
			p12Data:  testUtf8PassV1P12,
			password: "测试密码@#$%中文",
			desc:     "使用UTF-8特殊字符密码的P12文件",
		},
		{
			name:     "test_rc4_v1_p12",
			p12Data:  testRc4V1P12,
			password: "123456",
			desc:     "使用RC4加密的P12文件",
		},
		{
			name:     "test_weak_v1_p12",
			p12Data:  testWeakV1P12,
			password: "123456",
			desc:     "使用弱加密参数的P12文件",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.p12Data) == 0 {
				t.Skipf("%s 文件不存在，请先运行 v1_gen.sh 脚本", tc.name)
			}

			t.Logf("测试 %s - %s", tc.name, tc.desc)

			// 尝试解析 P12 文件
			cert, key, _, err := LoadP12ToPEM(tc.p12Data, tc.password)
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

			t.Logf("✓ %s 测试通过", tc.name)
		})
	}
}

// TestMultiCertAndSpecialKeyP12 测试多证书和特殊密钥类型的P12文件
func TestMultiCertAndSpecialKeyP12(t *testing.T) {
	// 1. 测试多证书P12
	t.Run("multi_cert_p12", func(t *testing.T) {
		if len(testMultiCertV1P12) == 0 {
			t.Skip("多证书P12文件不存在，请先运行v1_gen.sh脚本")
		}

		cert, key, ca, err := LoadP12ToPEM(testMultiCertV1P12, "123456")
		if err != nil {
			t.Fatalf("解析多证书P12失败: %v", err)
		}

		// 验证主证书
		if len(cert) == 0 {
			t.Fatal("证书为空")
		}

		// 验证私钥
		if len(key) == 0 {
			t.Fatal("私钥为空")
		}

		// 验证CA证书列表
		if len(ca) == 0 {
			t.Fatal("CA证书列表为空，应该包含额外证书")
		}

		t.Logf("包含 %d 个CA证书", len(ca))
		for i, caCert := range ca {
			parsedCA, err := ParsePEMCertificate(caCert)
			if err != nil {
				t.Fatalf("解析CA证书[%d]失败: %v", i, err)
			}
			t.Logf("CA[%d]: %s", i, parsedCA.Subject.String())
		}

		// 验证TLS可用性
		_, err = tls.X509KeyPair(cert, key)
		if err != nil {
			t.Fatalf("创建TLS证书对失败: %v", err)
		}

		t.Log("✓ 多证书P12测试通过")
	})

	// 2. 测试EC密钥P12
	t.Run("ec_key_p12", func(t *testing.T) {
		if len(testEcV1P12) == 0 {
			t.Skip("EC密钥P12文件不存在，请先运行v1_gen.sh脚本")
		}

		cert, key, _, err := LoadP12ToPEM(testEcV1P12, "123456")
		if err != nil {
			t.Fatalf("解析EC密钥P12失败: %v", err)
		}

		// 验证证书
		parsedCert, err := ParsePEMCertificate(cert)
		if err != nil {
			t.Fatalf("解析证书失败: %v", err)
		}

		// 验证密钥类型为EC
		_, isEC := parsedCert.PublicKey.(*ecdsa.PublicKey)
		if !isEC {
			t.Fatal("密钥类型不是EC")
		}

		t.Log("✓ EC密钥类型正确")

		// 验证TLS可用性
		_, err = tls.X509KeyPair(cert, key)
		if err != nil {
			t.Fatalf("创建TLS证书对失败: %v", err)
		}

		t.Log("✓ EC密钥P12测试通过")
	})

	// 3. 测试DSA密钥P12
	//t.Run("dsa_key_p12", func(t *testing.T) {
	//	if len(testDsaV1P12) == 0 {
	//		t.Skip("DSA密钥P12文件不存在，请先运行v1_gen.sh脚本")
	//	}
	//
	//	cert, _, _, err := LoadP12ToPEM(testDsaV1P12, "123456")
	//	if err != nil {
	//		t.Fatalf("解析DSA密钥P12失败: %v", err)
	//	}
	//
	//	// 验证证书
	//	parsedCert, err := ParsePEMCertificate(cert)
	//	if err != nil {
	//		t.Fatalf("解析证书失败: %v", err)
	//	}
	//
	//	// 验证密钥类型为DSA
	//	_, isDSA := parsedCert.PublicKey.(*dsa.PublicKey)
	//	if !isDSA {
	//		t.Fatal("密钥类型不是DSA")
	//	}
	//
	//	t.Log("✓ DSA密钥类型正确")
	//
	//	// 对于DSA密钥，跳过tls.X509KeyPair验证，因为Go标准库不支持DSA密钥用于TLS
	//	t.Log("✓ DSA密钥P12测试通过")
	//})
}
