package codec

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"fmt"
	"github.com/yaklang/yaklang/common/gmsm/sm4"
	"strings"
)

/* CMAC uses mac with no iv to compute the MAC.

    +-----+     +-----+     +-----+     +-----+     +-----+     +---+----+
    | M_1 |     | M_2 |     | M_n |     | M_1 |     | M_2 |     |M_n|10^i|
    +-----+     +-----+     +-----+     +-----+     +-----+     +---+----+
       |           |           |   +--+    |           |           |   +--+
       |     +--->(+)    +--->(+)<-|K1|    |     +--->(+)    +--->(+)<-|K2|
       |     |     |     |     |   +--+    |     |     |     |     |   +--+
    +-----+  |  +-----+  |  +-----+     +-----+  |  +-----+  |  +-----+
    |AES_K|  |  |AES_K|  |  |AES_K|     |AES_K|  |  |AES_K|  |  |AES_K|
    +-----+  |  +-----+  |  +-----+     +-----+  |  +-----+  |  +-----+
       |     |     |     |     |           |     |     |     |     |
       +-----+     +-----+     |           +-----+     +-----+     |
                               |                                   |
                            +-----+                              +-----+
                            |  T  |                              |  T  |
                            +-----+                              +-----+

	Illustration of the two cases of CMAC computation using the cipher AES.

The case on the left is when the number of bytes of the message is a multiple
of the block size. The case of the right is when padding bits must be
appended to the last block to get a full block. The padding is the bit 1
followed by as many bit 0 as required.

K1 and K2 have the size of a block and are computed as follow:

   const_zero = [0, ..., 0, 0]
   const_Rb   = [0, ..., 0, 0x87]

   Step 1.  L := AES-128(K, const_Zero);
   Step 2.  if MostSignificantBit(L) is equal to 0
            then    K1 := L << 1;
            else    K1 := (L << 1) XOR const_Rb;
   Step 3.  if MostSignificantBit(K1) is equal to 0
            then    K2 := K1 << 1;
            else    K2 := (K1 << 1) XOR const_Rb;
*/

type CmacBuilder struct {
	blockSize, n   int
	mac, k1, k2, x []byte
	cipher         cipher.Block
}

// NewCipherFunc instantiates a block cipher
type NewCipherFunc func(key []byte) (cipher.Block, error)

// New returns a new CMAC hash using the given cipher instantiation function and key.
func New(newCipher NewCipherFunc, key []byte) (*CmacBuilder, error) {
	c, err := newCipher(key)
	if err != nil {
		return nil, err
	}
	var bs = c.BlockSize()
	var cm = new(CmacBuilder)
	cm.blockSize = bs
	b := make([]byte, 4*bs)
	cm.mac, cm.k1, cm.k2, cm.x = b[:bs], b[bs:2*bs], b[2*bs:3*bs], b[3*bs:4*bs]
	cm.cipher = c
	c.Encrypt(cm.k1, cm.k1)
	tmp := cm.k1[0]
	shiftLeftOneBit(cm.k1, cm.k1)
	cm.k1[bs-1] ^= 0x87 & byte(int8(tmp)>>7) // xor with 0x87 when most significant bit of tmp is 1
	tmp = cm.k1[0]
	shiftLeftOneBit(cm.k2, cm.k1)
	cm.k2[bs-1] ^= 0x87 & byte(int8(tmp)>>7) // xor with 0x87 when most significant bit of tmp is 1
	return cm, nil
}

func (c *CmacBuilder) Size() int { return c.blockSize }

func (c *CmacBuilder) BlockSize() int { return c.blockSize }

func shiftLeftOneBit(dst, src []byte) {
	var overflow byte
	for i := len(src) - 1; i >= 0; i-- {
		var tmp = src[i]
		dst[i] = (tmp << 1) | overflow
		overflow = tmp >> 7
	}
}

// Write accumulates the bytes in m in the cmac computation.
func (c *CmacBuilder) Write(m []byte) (n int, err error) {
	n = len(m)
	if l := c.blockSize - c.n; len(m) > l {
		xor(c.x[c.n:], m[:l])
		m = m[l:]
		c.cipher.Encrypt(c.x, c.x)
		c.n = 0
	}
	for len(m) > c.blockSize {
		xor(c.x, m[:c.blockSize])
		m = m[c.blockSize:]
		c.cipher.Encrypt(c.x, c.x)
	}
	if len(m) > 0 {
		xor(c.x[c.n:], m)
		c.n += len(m)
	}
	return
}

// Sum returns the CMAC appended to m. m may be nil. Write may be called after Sum.
func (c *CmacBuilder) Sum(m []byte) []byte {
	if c.n == c.blockSize {
		copy(c.mac, c.k1)
	} else {
		copy(c.mac, c.k2)
		c.mac[c.n] ^= 0x80
	}
	xor(c.mac, c.x)
	c.cipher.Encrypt(c.mac, c.mac)
	return append(m, c.mac...)
}

// Reset the the CMAC
func (c *CmacBuilder) Reset() {
	for i := range c.x {
		c.x[i] = 0
	}
	c.n = 0
}

// xor stores a xor b in a. The length of b must be smaller or equal to a.
func xor(a, b []byte) {
	for i, v := range b {
		a[i] ^= v
	}
}

// Equal compares two MACs for equality without leaking timing information.
func Equal(mac1, mac2 []byte) bool {
	if len(mac1) != len(mac2) {
		return false
	}
	// copied from libsodium
	var b byte
	for i := range mac1 {
		b |= mac1[i] ^ mac2[i]
	}
	return ((uint16(b)-1)>>8)&1 == 1
}

func Cmac(alg string, key []byte, message []byte) ([]byte, error) {
	var cipherFunc NewCipherFunc
	alg = strings.ToUpper(alg)
	switch alg {
	case "AES":
		cipherFunc = aes.NewCipher
	case "SM4":
		cipherFunc = sm4.NewCipher
	case "DES":
		cipherFunc = des.NewCipher
	case "3DES":
		cipherFunc = des.NewTripleDESCipher
	default:
		return nil, fmt.Errorf("unsupported cipher algorithm %s", alg)
	}

	builder, err := New(cipherFunc, key)
	if err != nil {
		return nil, err
	}

	_, err = builder.Write(message)
	if err != nil {
		return nil, err
	}
	return builder.Sum(nil), nil
}
