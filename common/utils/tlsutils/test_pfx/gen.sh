#!/bin/bash
# 生成各种兼容性版本的 P12 证书

# 生成密钥和证书 - 确保使用 PKCS1 格式
openssl genrsa -out test_key.pem 2048
openssl req -new -x509 -key test_key.pem -out test_cert.pem -days 365 \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=Test/CN=test.example.com"

# 验证生成的私钥格式
echo "验证生成的私钥格式..."
head -1 test_key.pem | grep -q "RSA PRIVATE KEY" && echo "✓ 私钥格式正确 (PKCS1)" || echo "⚠ 私钥格式可能不正确"

# 生成不同版本的 P12
echo "生成 DES3 版本..."
openssl pkcs12 -export -in test_cert.pem -inkey test_key.pem -out test_des3.p12 \
    -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -macalg sha1 -password pass:123456

echo "生成 DES 版本..."
openssl pkcs12 -export -in test_cert.pem -inkey test_key.pem -out test_des.p12 \
    -certpbe PBE-SHA1-DES -keypbe PBE-SHA1-DES -macalg sha1 -password pass:123456 \
    -legacy

echo "生成 RC2 版本..."
openssl pkcs12 -export -in test_cert.pem -inkey test_key.pem -out test_rc2.p12 \
    -certpbe PBE-SHA1-RC2-40 -keypbe PBE-SHA1-RC2-40 -macalg sha1 -password pass:123456 \
    -legacy

echo "生成无加密版本..."
openssl pkcs12 -export -in test_cert.pem -inkey test_key.pem -out test_noenc.p12 \
    -nodes -password pass:123456

echo "生成 Legacy 版本..."
openssl pkcs12 -export -in test_cert.pem -inkey test_key.pem -out test_legacy.p12 \
    -legacy -password pass:123456

echo "完成！生成了以下文件："
ls -la test_*.p12