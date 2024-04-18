#!/bin/bash

cd $(dirname $0)
# 证书文件路径
CERT_FILE="yak-mitm-ca.crt"

# 检查操作系统类型
OS=$(uname)
if [ "$OS" == "Darwin" ]; then
    # macOS 上的证书存储路径
    CERT_STORE="/Library/Keychains/System.keychain"
    sudo security add-trusted-cert -d -r trustRoot -k "$CERT_STORE" "$CERT_FILE"
elif [ "$OS" == "Linux" ]; then
    # Linux 上的证书存储路径
    CERT_STORE="/etc/ssl/certs/ca-certificates.crt"
    # 安装证书到系统信任
    sudo cp "$CERT_FILE" "$CERT_STORE"
    sudp update-ca-certificates
else
    echo "Unsupported operating system: $OS"
fi

# 检查安装结果
if [ $? -eq 0 ]; then
  echo -e "\033[32mCertificate successfully installed.\033[0m"
else
  echo -e "\033[31mFailed to install certificate.\033[0m"
fi

read -p "Press Enter to exit"