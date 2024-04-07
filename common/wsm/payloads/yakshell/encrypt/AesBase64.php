<?
function encrypt($data, $key)
{
    if (!extension_loaded('openssl')) {
        return base64_encode($data);
    } else {
        return base64_encode(openssl_encrypt($data, "AES-128-ECB", $key, OPENSSL_RAW_DATA, ""));
    }
}