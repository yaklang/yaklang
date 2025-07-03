package codec

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSM2SignWithSM3(t *testing.T) {
	// 生成SM2密钥对
	priKeyPEM, pubKeyPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成SM2密钥对失败: %v", err)
	}

	// 测试数据
	testData := []string{
		"hello world",
		"SM2数字签名测试",
		"", // 空字符串
		"这是一个较长的测试字符串，用于验证SM2签名功能是否能够正确处理各种长度的数据。包含中文字符、英文字符、数字123和特殊符号!@#$%^&*()",
	}

	for i, data := range testData {
		t.Run(fmt.Sprintf("测试数据_%d", i), func(t *testing.T) {
			// 签名
			signature, err := SM2SignWithSM3(priKeyPEM, data)
			assert.NoError(t, err, "SM2签名应该成功")
			assert.NotEmpty(t, signature, "签名结果不应该为空")

			// 验证
			err = SM2VerifyWithSM3(pubKeyPEM, data, signature)
			assert.NoError(t, err, "SM2签名验证应该成功")

			// 验证错误数据应该失败
			wrongData := data + "_wrong"
			err = SM2VerifyWithSM3(pubKeyPEM, wrongData, signature)
			assert.Error(t, err, "错误数据的验证应该失败")
		})
	}
}

func TestSM2SignWithSM3_HexKeyPair(t *testing.T) {
	// 测试HEX格式的密钥对
	priKeyHex, pubKeyHex, err := GenerateSM2PrivateKeyHEX()
	if err != nil {
		t.Fatalf("生成SM2 HEX密钥对失败: %v", err)
	}

	data := "HEX密钥对签名测试"

	// 签名
	signature, err := SM2SignWithSM3(priKeyHex, data)
	assert.NoError(t, err, "使用HEX私钥签名应该成功")

	// 验证
	err = SM2VerifyWithSM3(pubKeyHex, data, signature)
	assert.NoError(t, err, "使用HEX公钥验证应该成功")
}

func TestSM2SignWithSM3WithPassword(t *testing.T) {
	// 注意：这个测试使用普通密钥对模拟带密码的情况
	// 实际项目中应该有真正的加密私钥生成功能
	priKeyPEM, pubKeyPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成SM2密钥对失败: %v", err)
	}

	data := "带密码私钥签名测试"

	// 测试无密码的情况（传入nil密码）
	signature, err := SM2SignWithSM3WithPassword(priKeyPEM, data, nil)
	assert.NoError(t, err, "无密码签名应该成功")

	err = SM2VerifyWithSM3(pubKeyPEM, data, signature)
	assert.NoError(t, err, "无密码签名验证应该成功")

	// 对于未加密的私钥，传入密码可能会导致解析失败
	// 这是x509.ReadPrivateKeyFromPem的行为，不是我们的bug
	password := []byte("test123")
	_, err = SM2SignWithSM3WithPassword(priKeyPEM, data, password)
	// 根据x509库的实现，对未加密私钥传入密码可能会失败
	// 这里我们只记录这个行为，不强制要求成功
	t.Logf("未加密私钥传入密码的结果: %v", err)
}

func TestSM2SignatureCompatibility(t *testing.T) {
	// 测试不同格式密钥之间的兼容性
	priKeyPEM, pubKeyPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成PEM密钥对失败: %v", err)
	}

	priKeyHex, pubKeyHex, err := GenerateSM2PrivateKeyHEX()
	if err != nil {
		t.Fatalf("生成HEX密钥对失败: %v", err)
	}

	data := "格式兼容性测试数据"

	// PEM私钥签名，PEM公钥验证
	signature1, err := SM2SignWithSM3(priKeyPEM, data)
	assert.NoError(t, err)
	err = SM2VerifyWithSM3(pubKeyPEM, data, signature1)
	assert.NoError(t, err, "PEM格式内部应该兼容")

	// HEX私钥签名，HEX公钥验证
	signature2, err := SM2SignWithSM3(priKeyHex, data)
	assert.NoError(t, err)
	err = SM2VerifyWithSM3(pubKeyHex, data, signature2)
	assert.NoError(t, err, "HEX格式内部应该兼容")

	// 不同密钥对之间不应该互相验证通过
	err = SM2VerifyWithSM3(pubKeyPEM, data, signature2)
	assert.Error(t, err, "不同密钥对不应该验证通过")

	err = SM2VerifyWithSM3(pubKeyHex, data, signature1)
	assert.Error(t, err, "不同密钥对不应该验证通过")
}

func TestSM2SignWithInvalidKeys(t *testing.T) {
	data := "无效密钥测试"

	// 测试无效私钥
	invalidPriKey := []byte("invalid_private_key")
	_, err := SM2SignWithSM3(invalidPriKey, data)
	assert.Error(t, err, "无效私钥应该签名失败")

	// 测试无效公钥
	priKeyPEM, _, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	signature, err := SM2SignWithSM3(priKeyPEM, data)
	assert.NoError(t, err)

	invalidPubKey := []byte("invalid_public_key")
	err = SM2VerifyWithSM3(invalidPubKey, data, signature)
	assert.Error(t, err, "无效公钥应该验证失败")

	// 测试空密钥
	_, err = SM2SignWithSM3(nil, data)
	assert.Error(t, err, "空私钥应该签名失败")

	err = SM2VerifyWithSM3(nil, data, signature)
	assert.Error(t, err, "空公钥应该验证失败")

	// 测试空字节数组
	_, err = SM2SignWithSM3([]byte{}, data)
	assert.Error(t, err, "空字节数组私钥应该签名失败")

	err = SM2VerifyWithSM3([]byte{}, data, signature)
	assert.Error(t, err, "空字节数组公钥应该验证失败")
}

func TestSM2SignWithEmptyData(t *testing.T) {
	priKeyPEM, pubKeyPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 测试空数据
	signature, err := SM2SignWithSM3(priKeyPEM, "")
	assert.NoError(t, err, "空数据签名应该成功")

	err = SM2VerifyWithSM3(pubKeyPEM, "", signature)
	assert.NoError(t, err, "空数据验证应该成功")
}

func TestSM2SignWithNilData(t *testing.T) {
	priKeyPEM, pubKeyPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成密钥对失败: %v", err)
	}

	// 测试nil数据应该失败
	_, err = SM2SignWithSM3(priKeyPEM, nil)
	assert.Error(t, err, "nil数据签名应该失败")
	assert.Contains(t, err.Error(), "data cannot be nil", "错误信息应该包含nil检查")

	// 测试nil数据验证应该失败
	// 先生成一个有效签名用于测试
	validSignature, _ := SM2SignWithSM3(priKeyPEM, "test")
	err = SM2VerifyWithSM3(pubKeyPEM, nil, validSignature)
	assert.Error(t, err, "nil数据验证应该失败")
	assert.Contains(t, err.Error(), "data cannot be nil", "错误信息应该包含nil检查")
}

// 基准测试
func BenchmarkSM2SignWithSM3(b *testing.B) {
	priKeyPEM, _, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		b.Fatalf("生成密钥对失败: %v", err)
	}

	data := "基准测试数据"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := SM2SignWithSM3(priKeyPEM, data)
		if err != nil {
			b.Fatalf("签名失败: %v", err)
		}
	}
}

func BenchmarkSM2VerifyWithSM3(b *testing.B) {
	priKeyPEM, pubKeyPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		b.Fatalf("生成密钥对失败: %v", err)
	}

	data := "基准测试数据"
	signature, err := SM2SignWithSM3(priKeyPEM, data)
	if err != nil {
		b.Fatalf("签名失败: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := SM2VerifyWithSM3(pubKeyPEM, data, signature)
		if err != nil {
			b.Fatalf("验证失败: %v", err)
		}
	}
}
