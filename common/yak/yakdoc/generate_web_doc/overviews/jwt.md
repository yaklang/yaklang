`jwt` 库提供 JWT（JSON Web Token）的生成、解析与攻击辅助能力，用于接口鉴权测试，尤其是常见的 JWT 误用漏洞（如算法混淆、空算法、弱密钥）验证。

典型使用场景：

- 生成：`jwt.JWTGenerate` / `jwt.JWTGenerateEx`（自定义 header）按算法与密钥签发 Token，`jwt.JWSGenerate` / `jwt.JWSGenerateEx` 生成 JWS。
- 解析与攻击：`jwt.Parse` 解析 Token 取出 claims，`jwt.RemoveAlg` 构造 `alg=none` 攻击 Token，`jwt.AllAlgs` 列出支持的算法。

与相邻库的关系：`jwt` 与 `codec`（底层签名/编码）、`fuzz`/`poc`（携带篡改后的 Token 发包）配合，用于鉴权绕过类测试。
