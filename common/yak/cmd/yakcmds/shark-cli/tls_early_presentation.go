package sharkcli

import "bytes"

func tlsEndOfEarlyDataPlaintext(m map[string]any, authenticatedEarly bool) []byte {
	if !authenticatedEarly || m["Authenticated"] != true || number(m["Type"]) != 5 || number(m["Length"]) != 0 || m["End Of Early Data"] == nil {
		return nil
	}
	w := bytesValue(m["Encoded End Of Early Data"])
	if !bytes.Equal(w, []byte{5, 0, 0, 0}) {
		return nil
	}
	return w
}
