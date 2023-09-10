package wsm

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func decodeBase64Values(i interface{}) (interface{}, error) {
	switch v := i.(type) {
	case string:
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 value: %w", err)
		}
		var j interface{}
		err = json.Unmarshal(decoded, &j)
		if err != nil {
			return string(decoded), nil
		}
		return decodeBase64Values(j)
	case []interface{}:
		for i, e := range v {
			decoded, err := decodeBase64Values(e)
			if err != nil {
				return nil, err
			}
			v[i] = decoded
		}
	case map[string]interface{}:
		for k, e := range v {
			decoded, err := decodeBase64Values(e)
			if err != nil {
				return nil, err
			}
			v[k] = decoded
		}
	}
	return i, nil
}
