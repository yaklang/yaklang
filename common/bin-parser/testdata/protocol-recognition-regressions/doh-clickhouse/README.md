# Synthetic DoH and ClickHouse recognition regressions

These five PCAPNG files are **synthetic regression fixtures**, not network captures from a deployed resolver or ClickHouse server. The DoH carriers are plaintext HTTP/1.1 and HTTP/2; they do not imply TLS decryption. Addresses, hostnames, and credentials are test values.

`manifest.json` records each file's SHA-256, construction, standards references, and expected behavior. `TestDoHClickHouseRecognitionRegressionCaptures` verifies every hash and replays the fixtures. The protocol-specific exchange/admission tests additionally construct the same message classes with one-byte and other TCP splits, deferred decoding, trailers, reset, and unsupported content types.

ClickHouse support remains the existing 23.8 / revision 54401 Hello/Ping profile. An initial packet that can be a server Hello or a client Hello with a one-byte password reports `context-required` without asserting a role. Observing an unambiguous client Hello first preserves server Hello and Pong parsing, including coalesced messages. These fixtures do not claim ClickHouse query execution or TLS decryption.
