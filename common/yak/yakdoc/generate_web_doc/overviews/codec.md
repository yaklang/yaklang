`codec` 库是 yaklang 的编解码与密码学工具箱，覆盖编码转换、哈希、对称/非对称加密、国密算法、字符集转换等近 180 个函数，是数据处理与密码学相关脚本的核心依赖。

典型使用场景：

- 编码转换：`codec.EncodeBase64` / `codec.DecodeBase64`、`codec.EncodeToHex` / `codec.DecodeHex`、`codec.EncodeUrl` / `codec.DecodeUrl`、`codec.EncodeHtml` / `codec.DecodeHtml`、`codec.UnicodeEncode` / `codec.UnicodeDecode`，以及 `codec.AutoDecode` 智能识别多层编码。
- 哈希与 HMAC：`codec.Md5` / `codec.Sha1` / `codec.Sha256` / `codec.Sm3`、`codec.HmacSha256` / `codec.HmacSM3`、`codec.MMH3Hash128` 等。
- 对称加密：AES（`codec.AESCBCEncrypt` / `codec.AESGCMEncrypt` 等多模式多填充）、DES/3DES、RC4，以及国密 `codec.Sm4*`。
- 非对称与签名：RSA（`codec.RSAEncryptWithOAEP` / `codec.RSASignWithPKCS1v15Digest` 等）、国密 `codec.Sm2*`（加解密、签名、密钥交换）。
- 字符集与填充：`codec.GBKToUTF8` / `codec.UTF8ToGBK`、`codec.PKCS7Padding` / `codec.ZeroPadding` 等。

与相邻库的关系：`codec` 是纯计算库，无副作用，被 `poc`、`jwt`、`tls`、`yso` 等大量上层库依赖。注意部分哈希/加密函数返回 `[]byte`，需要 `codec.EncodeToHex`/`codec.EncodeBase64` 转成可读字符串；对称算法对密钥/IV 长度有严格要求。
