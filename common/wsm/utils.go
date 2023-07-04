package wsm

import (
	"crypto/md5"
	"encoding/hex"
)

// SecretKey 将字符串转换为符合冰蝎、哥斯拉加密要求的 md5[0:16] 后的结果
func secretKey(pwd string) []byte {
	return []byte(pass2MD5(pwd))
}

// 获取前十六位 md5 值
func pass2MD5(input string) string {
	md5hash := md5.New()
	md5hash.Write([]byte(input))
	return hex.EncodeToString(md5hash.Sum(nil))[0:16]
}
