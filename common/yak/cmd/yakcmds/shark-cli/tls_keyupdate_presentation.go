package sharkcli

// A plaintext source needs the complete authenticated KeyUpdate, rather than
// a connection label or a byte range borrowed from its encrypted record.
func tlsKeyUpdatePlaintext(m map[string]any, authenticated bool) []byte {
	request, present := m["Key Update Request"]
	if !authenticated || m["Authenticated"] != true || number(m["Type"]) != 24 || number(m["Length"]) != 1 || !present || number(request) > 1 {
		return nil
	}
	w := bytesValue(m["Encoded Key Update"])
	if len(w) != 5 || w[0] != 24 || w[1] != 0 || w[2] != 0 || w[3] != 1 || uint64(w[4]) != number(request) {
		return nil
	}
	return w
}

func tlsKeyUpdateRequestName(value any) string {
	switch number(value) {
	case 0:
		return "update_not_requested"
	case 1:
		return "update_requested"
	}
	return "unknown"
}
