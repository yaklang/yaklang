`tls` 库提供 TLS/证书与密钥相关能力：生成根 CA、服务端/客户端证书、RSA/SM2 密钥对，以及探测目标的 TLS 配置，常用于搭建 HTTPS 服务、证书签发与 TLS 安全检查。

典型使用场景：

- 生成证书：`tls.GenerateRootCA` 生成根 CA，`tls.GenerateServerCert` / `tls.GenerateClientCert` 签发服务端/客户端证书（配 `tls.commonName` / `tls.alternativeDNS` / `tls.alternativeIP` / `tls.validity` 等选项）。
- 生成密钥：`tls.GenerateRSA2048KeyPair` / `tls.GenerateRSAKeyPair` / `tls.GenerateSM2KeyPair`，`tls.EncryptWithPkcs1v15` / `tls.DecryptWithPkcs1v15` 做 PKCS1v15 加解密。
- 探测：`tls.Inspect` / `tls.InspectForceHttp2` / `tls.InspectForceHttp1_1` 探测目标 TLS 配置与证书信息。

与相邻库的关系：`tls` 生成的证书常用于 `httpserver`/`tcp`（起 HTTPS/TLS 服务）、`mitm`（根证书），与 `codec`/`ja3` 在密码学与指纹方向互补。
