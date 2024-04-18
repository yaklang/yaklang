@echo off

set CERT_FILE="yak-mitm-ca.crt"

certutil -addstore -user -f "Root" %CERT_FILE%

if %errorlevel% neq 0 (
    echo Certificate installation failed.
) else (
    echo Certificate successfully installed.
)

pause