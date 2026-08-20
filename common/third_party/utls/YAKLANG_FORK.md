# Yaklang uTLS fork

This directory is based on `github.com/refraction-networking/utls` v1.6.7 and
retains its BSD-3-Clause license.

Yaklang carries a narrow backport of the ALPS codepoint 17613 support merged
upstream in [refraction-networking/utls#333](https://github.com/refraction-networking/utls/pull/333).
The upstream releases containing that change require Go 1.24, while Yaklang's
module remains on Go 1.22.12.

The backport adds:

- `ApplicationSettingsExtensionNew` for ClientHello codepoint 17613;
- parsing of server ALPS 17613 in EncryptedExtensions;
- the matching Client EncryptedExtensions response after TLS Finished;
- connection-state and extension-dictionary plumbing for both ALPS codepoints.

Keep changes in this fork limited to compatibility fixes that cannot be
provided by the upstream version supported by Yaklang's Go toolchain.
