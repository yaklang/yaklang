package codec

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"github.com/xdg-go/pbkdf2"
	"hash"
)

const DefaultPBKDF2Iterations = 10000
const DefaultBytesToKeyIterations = 1

const (
	opensslKeyLength = 32
	opensslIVLength  = 16
)

var (
	// BytesToKeyMD5 utilizes MD5 key-derivation (`-md md5`)
	BytesToKeyMD5 = NewBytesToKeyGenerator(md5.New, DefaultBytesToKeyIterations)
	// BytesToKeySHA1 utilizes SHA1 key-derivation (`-md sha1`)
	BytesToKeySHA1 = NewBytesToKeyGenerator(sha1.New, DefaultBytesToKeyIterations)
	// BytesToKeySHA256 utilizes SHA256 key-derivation (`-md sha256`)
	BytesToKeySHA256 = NewBytesToKeyGenerator(sha256.New, DefaultBytesToKeyIterations)
	// BytesToKeySHA384 utilizes SHA384 key-derivation (`-md sha384`)
	BytesToKeySHA384 = NewBytesToKeyGenerator(sha512.New384, DefaultBytesToKeyIterations)
	// BytesToKeySHA512 utilizes SHA512 key-derivation (`-md sha512`)
	BytesToKeySHA512 = NewBytesToKeyGenerator(sha512.New, DefaultBytesToKeyIterations)
	// PBKDF2MD5 utilizes PBKDF2 key derivation with MD5 hashing (`-pbkdf2 -md md5`)
	PBKDF2MD5 = NewPBKDF2Generator(md5.New, DefaultPBKDF2Iterations)
	// PBKDF2SHA1 utilizes PBKDF2 key derivation with SHA1 hashing (`-pbkdf2 -md sha1`)
	PBKDF2SHA1 = NewPBKDF2Generator(sha1.New, DefaultPBKDF2Iterations)
	// PBKDF2SHA256 utilizes PBKDF2 key derivation with SHA256 hashing (`-pbkdf2 -md sha256`)
	PBKDF2SHA256 = NewPBKDF2Generator(sha256.New, DefaultPBKDF2Iterations)
	// PBKDF2SHA384 utilizes PBKDF2 key derivation with SHA384 hashing (`-pbkdf2 -md sha384`)
	PBKDF2SHA384 = NewPBKDF2Generator(sha512.New384, DefaultPBKDF2Iterations)
	// PBKDF2SHA512 utilizes PBKDF2 key derivation with SHA512 hashing (`-pbkdf2 -md sha512`)
	PBKDF2SHA512 = NewPBKDF2Generator(sha512.New, DefaultPBKDF2Iterations)
)

type KeyDerivationFunc func(password, salt []byte) ([]byte, []byte, error)

func NewBytesToKeyGenerator(hashFunc func() hash.Hash, iterations int) KeyDerivationFunc {
	hasher := hashFunc()
	return func(password, salt []byte) ([]byte, []byte, error) {
		var m []byte
		block := []byte{}
		for len(m) < opensslKeyLength+opensslIVLength {
			hasher.Write(block)
			hasher.Write(password)
			hasher.Write(salt)
			block = hasher.Sum(nil)
			hasher.Reset()
			m = append(m, block...)
		}
		return m[:opensslKeyLength], m[opensslKeyLength : opensslKeyLength+opensslIVLength], nil
	}
}

func NewPBKDF2Generator(hashFunc func() hash.Hash, iterations int) KeyDerivationFunc {
	return func(password, salt []byte) ([]byte, []byte, error) {
		m := pbkdf2.Key(password, salt, iterations, opensslKeyLength+opensslIVLength, hashFunc)
		return m[:opensslKeyLength], m[opensslKeyLength : opensslKeyLength+opensslIVLength], nil
	}
}
