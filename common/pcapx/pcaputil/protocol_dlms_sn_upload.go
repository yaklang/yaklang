package pcaputil

func dlmsSNUploadWireProfile(w []byte) string {
	if len(w) < 3 || w[1]&8 != 0 {
		return ""
	}
	info := dlmsSNInformationPrefix(w)
	if len(info) < 5 || info[0] != 0xe6 || info[2] != 0 || info[1] != 0xe6 && info[1] != 0xe7 {
		return ""
	}
	return dlmsSNUploadAPDUProfile(info[3:])
}

func dlmsSNUploadAPDUProfile(p []byte) string {
	if len(p) < 2 {
		return ""
	}
	_, at, ok := wrapperSNDefinitePrefix(p, 1)
	if !ok || at >= len(p) {
		return ""
	}
	if p[0] == 5 && p[at] == 6 || p[0] == 12 && p[at] == 3 {
		return "dlms-hdlc-sn-read-upload"
	}
	if p[0] == 6 && p[at] == 7 || p[0] == 13 && p[at] == 2 {
		return "dlms-hdlc-sn-write-upload"
	}
	return ""
}

func dlmsSNUploadRefusalHint(w []byte) bool {
	profile := dlmsSNUploadWireProfile(w)
	if profile == "" {
		return false
	}
	p := dlmsSNInformationPrefix(w)[3:]
	n, at, ok := wrapperSNDefinitePrefix(p, 1)
	if !ok || n != 1 {
		return false
	}
	if p[0] == 12 || p[0] == 13 {
		return len(p) == at+3
	}
	if len(p) < at+5 || p[at+1] > 1 {
		return false
	}
	if p[0] == 5 {
		n, end, ok := wrapperSNDefinitePrefix(p, at+4)
		return ok && n == uint64(len(p)-end)
	}
	n, at, ok = wrapperSNDefinitePrefix(p, at+4)
	if !ok || n != 1 || at >= len(p) || p[at] != 9 {
		return false
	}
	n, end, ok := wrapperSNDefinitePrefix(p, at+1)
	return ok && n == uint64(len(p)-end)
}
