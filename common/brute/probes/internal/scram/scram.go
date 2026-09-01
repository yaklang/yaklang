// Package scram 提供 SCRAM (RFC 5802) 客户端实现的最小公共层，
// 供 PostgreSQL (SCRAM-SHA-256) 与 MongoDB (SCRAM-SHA-1/256) 探针共用。
// 只依赖标准库与 golang.org/x/text（SASLprep），不引入任何驱动。
package scram

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash"
	"strconv"
	"strings"

	"golang.org/x/text/secure/precis"
)

// PBKDF2 实现 RFC 2898（HMAC-SHA256/SHA1），避免引入 golang.org/x/crypto/pbkdf2。
func PBKDF2(hash func() hash.Hash, password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(hash, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	u := make([]byte, hashLen)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		prf.Write(buf[:4])
		dk = prf.Sum(dk)
		t := dk[len(dk)-hashLen:]
		copy(u, t)
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for x := range u {
				t[x] ^= u[x]
			}
		}
	}
	return dk[:keyLen]
}

// SASLprep 按 RFC 4013 规范化字符串（用于 SCRAM-SHA-256 的用户名与密码）。
// 规范化失败时按 RFC 要求返回错误。
func SASLprep(s string) (string, error) {
	out, err := precis.OpaqueString.String(s)
	if err != nil {
		return "", fmt.Errorf("saslprep: %w", err)
	}
	return out, nil
}

// Client 完成 RFC 5802 客户端侧计算。
type Client struct {
	hash func() hash.Hash
	// Hi 输出的密钥长度
	keyLen         int
	authMessage    string
	saltedPassword []byte
	clientKey      []byte
	storedKey      []byte
}

// NewClient 创建 SCRAM 客户端（sha1 → SCRAM-SHA-1，sha256 → SCRAM-SHA-256）。
func NewClient(sha256Mode bool) *Client {
	if sha256Mode {
		return &Client{hash: sha256.New, keyLen: 32}
	}
	return &Client{hash: sha1.New, keyLen: 20}
}

// SetCredentials 计算派生密钥。
// password 为该协议约定的"密码原文"（Mongo SCRAM-SHA-1 为 md5hex(user:mongo:pass)）。
func (c *Client) SetCredentials(password string, salt []byte, iterations int) error {
	if iterations < 1 || iterations > 10_000_000 {
		return fmt.Errorf("scram: iteration count out of range: %d", iterations)
	}
	c.saltedPassword = PBKDF2(c.hash, []byte(password), salt, iterations, c.keyLen)
	c.clientKey = c.hmacSum(c.saltedPassword, []byte("Client Key"))
	h := c.hash()
	h.Write(c.clientKey)
	c.storedKey = h.Sum(nil)
	return nil
}

// BuildClientFirst 构造 client-first-message（不含 gs2 头）。
// channelBinding 为 gs2 头之后 c= 展开值：无通道绑定为 "biws"（base64("n,,")）。
func BuildClientFirst(username, clientNonce string) string {
	return "n=" + username + ",r=" + clientNonce
}

// ParseServerFirst 解析 server-first-message，提取 r/s/i。
func ParseServerFirst(msg string) (combinedNonce, salt []byte, iterations int, err error) {
	var saltB64 string
	for _, part := range strings.Split(msg, ",") {
		if len(part) < 2 || part[1] != '=' {
			continue
		}
		switch part[0] {
		case 'r':
			combinedNonce = []byte(part[2:])
		case 's':
			saltB64 = part[2:]
		case 'i':
			iterations, err = strconv.Atoi(part[2:])
			if err != nil {
				return nil, nil, 0, fmt.Errorf("scram: bad iteration count")
			}
		case 'm':
			return nil, nil, 0, fmt.Errorf("scram: unsupported mandatory extension")
		}
	}
	if combinedNonce == nil || saltB64 == "" || iterations == 0 {
		return nil, nil, 0, fmt.Errorf("scram: malformed server-first")
	}
	salt, err = base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("scram: bad salt encoding")
	}
	return combinedNonce, salt, iterations, nil
}

// BuildClientFinal 构造 client-final-message（含 proof）并记录 authMessage。
// authMessage = clientFirstBare + "," + serverFirst + "," + clientFinalWithoutProof。
// channelBinding："biws"（gs2 "n,,"）或协议指定的 c= 值。
func (c *Client) BuildClientFinal(channelBinding, combinedNonce, clientFirstBare, serverFirst string) string {
	withoutProof := "c=" + channelBinding + ",r=" + combinedNonce
	c.authMessage = clientFirstBare + "," + serverFirst + "," + withoutProof
	clientSig := c.hmacSum(c.storedKey, []byte(c.authMessage)) // RFC5802: 对完整 authMessage 签名
	proof := make([]byte, len(c.clientKey))
	for i := range c.clientKey {
		proof[i] = c.clientKey[i] ^ clientSig[i]
	}
	return withoutProof + ",p=" + base64.StdEncoding.EncodeToString(proof)
}

// VerifyServerFinal 校验 server-final-message 的 v= 签名。
func (c *Client) VerifyServerFinal(msg string) error {
	if !strings.HasPrefix(msg, "v=") {
		return fmt.Errorf("scram: malformed server-final")
	}
	want, err := base64.StdEncoding.DecodeString(msg[2:])
	if err != nil {
		return fmt.Errorf("scram: bad server signature encoding")
	}
	serverKey := c.hmacSum(c.saltedPassword, []byte("Server Key"))
	serverSig := c.hmacSum(serverKey, []byte(c.authMessage))
	if !hmac.Equal(serverSig, want) {
		return fmt.Errorf("scram: server signature mismatch")
	}
	return nil
}

func (c *Client) hmacSum(key, data []byte) []byte {
	m := hmac.New(c.hash, key)
	m.Write(data)
	return m.Sum(nil)
}
