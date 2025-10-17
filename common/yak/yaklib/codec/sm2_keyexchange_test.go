package codec

import (
	"bytes"
	"testing"
)

func TestSM2KeyExchange(t *testing.T) {
	// 1. 生成A方和B方的长期密钥对
	priKeyA, pubKeyA, err := GenerateSM2PrivateKeyHEX()
	if err != nil {
		t.Fatalf("生成A方密钥对失败: %v", err)
	}

	priKeyB, pubKeyB, err := GenerateSM2PrivateKeyHEX()
	if err != nil {
		t.Fatalf("生成B方密钥对失败: %v", err)
	}

	// 2. 生成临时密钥对
	tempPriKeyA, tempPubKeyA, err := SM2GenerateTemporaryKeyPair()
	if err != nil {
		t.Fatalf("生成A方临时密钥对失败: %v", err)
	}

	tempPriKeyB, tempPubKeyB, err := SM2GenerateTemporaryKeyPair()
	if err != nil {
		t.Fatalf("生成B方临时密钥对失败: %v", err)
	}

	// 3. 设置身份标识和密钥长度
	idA := []byte("Alice")
	idB := []byte("Bob")
	keyLength := 32

	// 4. A方执行密钥交换
	sharedKeyA, s1A, s2A, err := SM2KeyExchange(keyLength, idA, idB, priKeyA, pubKeyB, tempPriKeyA, tempPubKeyB, true)
	if err != nil {
		t.Fatalf("A方密钥交换失败: %v", err)
	}

	// 5. B方执行密钥交换
	sharedKeyB, s1B, s2B, err := SM2KeyExchange(keyLength, idA, idB, priKeyB, pubKeyA, tempPriKeyB, tempPubKeyA, false)
	if err != nil {
		t.Fatalf("B方密钥交换失败: %v", err)
	}

	// 6. 验证共享密钥是否一致
	if !bytes.Equal(sharedKeyA, sharedKeyB) {
		t.Errorf("共享密钥不一致:\nA方: %x\nB方: %x", sharedKeyA, sharedKeyB)
	}

	// 7. 验证验证值是否一致
	if !bytes.Equal(s1A, s1B) {
		t.Errorf("验证值S1不一致:\nA方: %x\nB方: %x", s1A, s1B)
	}

	if !bytes.Equal(s2A, s2B) {
		t.Errorf("验证值S2不一致:\nA方: %x\nB方: %x", s2A, s2B)
	}

	// 8. 验证密钥长度
	if len(sharedKeyA) != keyLength {
		t.Errorf("共享密钥长度不正确，期望: %d，实际: %d", keyLength, len(sharedKeyA))
	}

	// 9. 验证验证值长度（SM3哈希长度为32字节）
	if len(s1A) != 32 {
		t.Errorf("验证值S1长度不正确，期望: 32，实际: %d", len(s1A))
	}

	if len(s2A) != 32 {
		t.Errorf("验证值S2长度不正确，期望: 32，实际: %d", len(s2A))
	}

	// 10. 测试PEM格式密钥
	priKeyAPEM, pubKeyAPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成A方PEM密钥对失败: %v", err)
	}

	priKeyBPEM, pubKeyBPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成B方PEM密钥对失败: %v", err)
	}

	tempPriKeyAPEM, tempPubKeyAPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成A方临时PEM密钥对失败: %v", err)
	}

	tempPriKeyBPEM, tempPubKeyBPEM, err := GenerateSM2PrivateKeyPEM()
	if err != nil {
		t.Fatalf("生成B方临时PEM密钥对失败: %v", err)
	}

	// 使用PEM格式密钥进行密钥交换
	sharedKeyAPEM, _, _, err := SM2KeyExchange(keyLength, idA, idB, priKeyAPEM, pubKeyBPEM, tempPriKeyAPEM, tempPubKeyBPEM, true)
	if err != nil {
		t.Fatalf("A方PEM密钥交换失败: %v", err)
	}

	sharedKeyBPEM, _, _, err := SM2KeyExchange(keyLength, idA, idB, priKeyBPEM, pubKeyAPEM, tempPriKeyBPEM, tempPubKeyAPEM, false)
	if err != nil {
		t.Fatalf("B方PEM密钥交换失败: %v", err)
	}

	if !bytes.Equal(sharedKeyAPEM, sharedKeyBPEM) {
		t.Errorf("PEM格式密钥交换失败，共享密钥不一致")
	}

	// 11. 测试错误情况
	_, _, _, err = SM2KeyExchange(keyLength, idA, idB, []byte("invalid_key"), pubKeyB, tempPriKeyA, tempPubKeyB, true)
	if err == nil {
		t.Errorf("期望无效私钥会产生错误，但没有错误发生")
	}

	_, _, _, err = SM2KeyExchange(0, idA, idB, priKeyA, pubKeyB, tempPriKeyA, tempPubKeyB, true)
	if err == nil {
		t.Errorf("期望零长度密钥会产生错误，但没有错误发生")
	}

	t.Logf("✅ SM2密钥交换测试全部通过")
	t.Logf("共享密钥: %x", sharedKeyA)
	t.Logf("验证值S1: %x", s1A)
	t.Logf("验证值S2: %x", s2A)
}
