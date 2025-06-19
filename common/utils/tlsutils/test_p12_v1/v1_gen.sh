#!/bin/bash
# 生成各种兼容性版本的 P12 证书 - OpenSSL 1.1.1t 版本

# 使用指定路径的 OpenSSL
OPENSSL_BIN="/usr/local/openssl-1.1.1t/bin/openssl"
export LD_LIBRARY_PATH="/usr/local/openssl-1.1.1t/lib:$LD_LIBRARY_PATH"

echo "使用 OpenSSL 1.1.1t 版本生成测试证书..."
$OPENSSL_BIN version

# 生成密钥和证书 - OpenSSL 1.1.1t 使用 PKCS1 格式
$OPENSSL_BIN genrsa -out test_key_v1.pem 2048
$OPENSSL_BIN req -new -x509 -key test_key_v1.pem -out test_cert_v1.pem -days 365 \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=Test/CN=test.example.com"

# 验证生成的私钥格式
echo "验证生成的私钥格式..."
head -1 test_key_v1.pem | grep -q "RSA PRIVATE KEY" && echo "✓ 私钥格式正确 (PKCS1)" || echo "⚠ 私钥格式可能不正确"

# 生成不同版本的 P12
echo "生成 AES-256 版本..."
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_aes256_v1.p12 \
    -certpbe AES-256-CBC -keypbe AES-256-CBC -macalg sha256 -password pass:123456

echo "生成 AES-128 版本..."
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_aes128_v1.p12 \
    -certpbe AES-128-CBC -keypbe AES-128-CBC -macalg sha1 -password pass:123456

# 生成 DES3 版本 (OpenSSL 1.1.1t 默认使用)
echo "生成 DES3 版本..."
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_des3_v1.p12 \
    -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -macalg sha1 -password pass:123456

echo "生成 RC2 版本..."
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_rc2_v1.p12 \
    -certpbe PBE-SHA1-RC2-40 -keypbe PBE-SHA1-RC2-40 -macalg sha1 -password pass:123456

echo "生成无密码版本..."
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_nopass_v1.p12 \
    -password pass:

echo "生成无加密版本..."
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_noenc_v1.p12 \
    -nodes -password pass:123456

# 生成兼容性版本
echo "生成兼容性版本..."
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_legacy_v1.p12 \
    -macalg sha1 -password pass:123456

# 尝试生成 DES 版本 (可能需要特殊参数)
echo "生成 DES 版本..."
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_des_v1.p12 \
    -certpbe PBE-SHA1-DES -keypbe PBE-SHA1-DES -macalg sha1 -password pass:123456

# 不同MAC算法测试
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_mac_md5_v1.p12 \
    -macalg md5 -password pass:123456

# 使用极短密码
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_shortpass_v1.p12 \
    -password pass:a

# 使用极长密码
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_longpass_v1.p12 \
    -password pass:$(printf 'a%.0s' {1..100})

# 使用UTF-8特殊字符密码
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_utf8pass_v1.p12 \
    -password pass:'测试密码@#$%中文'

# RC4加密版本
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_rc4_v1.p12 \
    -certpbe PBE-SHA1-RC4-128 -keypbe PBE-SHA1-RC4-128 -macalg sha1 -password pass:123456

# 非常弱的加密参数
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem -out test_weak_v1.p12 \
    -certpbe PBE-SHA1-RC2-40 -keypbe PBE-SHA1-RC2-40 -macalg md5 -password pass:123456

# 生成额外的证书
$OPENSSL_BIN genrsa -out extra_key.pem 2048
$OPENSSL_BIN req -new -x509 -key extra_key.pem -out extra_cert.pem -days 365 \
    -subj "/C=CN/ST=Shanghai/L=Shanghai/O=Extra/CN=extra.example.com"

# 生成带有两个证书的P12文件
$OPENSSL_BIN pkcs12 -export -in test_cert_v1.pem -inkey test_key_v1.pem \
    -certfile extra_cert.pem -out test_multicert_v1.p12 -password pass:123456

# 生成EC密钥的P12
$OPENSSL_BIN ecparam -name prime256v1 -genkey -out ec_key.pem
$OPENSSL_BIN req -new -x509 -key ec_key.pem -out ec_cert.pem -days 365 \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=Test/CN=ec.example.com"
$OPENSSL_BIN pkcs12 -export -in ec_cert.pem -inkey ec_key.pem \
    -out test_ec_v1.p12 -password pass:123456

# 生成DSA密钥的P12
$OPENSSL_BIN dsaparam -out dsaparam.pem 2048
$OPENSSL_BIN gendsa -out dsa_key.pem dsaparam.pem
$OPENSSL_BIN req -new -x509 -key dsa_key.pem -out dsa_cert.pem -days 365 \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=Test/CN=dsa.example.com"
$OPENSSL_BIN pkcs12 -export -in dsa_cert.pem -inkey dsa_key.pem \
    -out test_dsa_v1.p12 -password pass:123456

echo "完成！生成了以下文件："
ls -la test_*_v1.p12
