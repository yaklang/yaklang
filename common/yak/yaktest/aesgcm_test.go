package yaktest

import (
	"testing"
)

func TestAesGCM_TEST(t *testing.T) {
	const code = `
//                             6ZmI6I2j5Y+R5aSn5ZOlAA==
keyBytes = codec.DecodeBase64("6ZmI6I2j5Y+R5aSn5ZOlAA==")[0]

javaDec := "Gvf/8Em+YA7bMk9uLxgc5GZnLYKmqz8FvP4c8Qq6EktwJ/O0UlpAHi4ELL5/sWwJKGsdR1+d0GjREumXZ01UjZtkouKfu750a8sW/4ZwNWh5Wa0dt55iB0MFhzwOUFbNh0FFyjmOqrQcrCKrFO8W9AboZBh87roMnAf/sHlVp0XlRQvU6yEJY6sRf5kwkg6f"
javaData = codec.DecodeBase64(javaDec)[0]
dump(javaData)
javaDecData, _ = codec.AESGCMDecrypt(keyBytes, javaData, nil)

encData := codec.AESGCMEncrypt(keyBytes, javaDecData, nil)[0]
dump(encData)
dump(
    codec.EncodeBase64(encData),
)

data = codec.DecodeHex("656ddcaa78122838d22de2a33a81a0e6aced0005737200326f72672e6170616368652e736869726f2e7375626a6563742e53696d706c655072696e636970616c436f6c6c656374696f6ea87f5825c6a3084a0300014c000f7265616c6d5072696e636970616c7374000f4c6a6176612f7574696c2f4d61703b78707077010078")[0]

encoded = codec.AESGCMEncrypt(keyBytes, data, nil)[0]
//encoded = append(keyBytes, encoded...)
aa = codec.EncodeBase64(encoded)
dump(aa)
originData = codec.EncodeToHex(codec.AESGCMDecrypt(codec.DecodeBase64("6ZmI6I2j5Y+R5aSn5ZOlAA==")[0]/*type: bytes*/, codec.DecodeBase64(aa)[0]/*type: any*/, nil/*type: bytes*/)[0])
dump(originData)`
	cases := []YakTestCase{
		{
			Name: "AES GCM 配置",
			Src:  code,
		},
	}

	Run("AES GCM 配置", t, cases...)
}
