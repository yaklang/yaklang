package pcaputil

import "bytes"

// DecodeTLSFinished validates the complete envelope and the verify_data size
// for the supported negotiated cipher suites. It does not verify the transcript
// MAC. The result owns its bytes and never identifies an outer ciphertext span.
func DecodeTLSFinished(encoded []byte, version, suite uint16) ([]byte, error) {
	if len(encoded) < 4 || encoded[0] != 20 || tlsU24(encoded[1:4]) != len(encoded)-4 {
		return nil, protocolError(ErrMalformedMessage, "TLS Finished framing")
	}
	if !tlsSupportedSuite(version, suite) {
		return nil, protocolError(ErrUnsupportedFeature, "TLS Finished negotiated version/cipher suite unavailable or unsupported")
	}
	size := 12 // RFC5246 supported AEAD suites use the default verify_data_length.
	if version == 0x304 {
		size = 32
		if suite == 0x1302 {
			size = 48
		}
	}
	if len(encoded)-4 != size {
		return nil, protocolError(ErrMalformedMessage, "TLS Finished verify_data length disagrees with negotiated cipher suite")
	}
	return bytes.Clone(encoded[4:]), nil
}
