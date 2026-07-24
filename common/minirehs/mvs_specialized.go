package minirehs

import "bytes"

// A few always-on rules have no extractable literal, yet describe a rigid
// byte structure. Recognizing the exact expression lets existence-only scans
// avoid a full assertion-NFA recurrence for every byte. Located scans still
// pass a positive result through finalizeHit and therefore retain the regular
// verifier's span semantics.
type mvsSpecializedKind uint8

const (
	mvsSpecializedNone mvsSpecializedKind = iota
	mvsSpecializedCNID
	mvsSpecializedMAC
	mvsSpecializedPhone
	mvsSpecializedJSON
	mvsSpecializedAWSRegion
	mvsSpecializedWindowsPath
)

type mvsSpecializedAlwaysOn struct {
	idx  int
	kind mvsSpecializedKind
}

const (
	mvsCNIDExpr        = `[^0-9]((\d{8}(0\d|10|11|12)([0-2]\d|30|31)\d{3}$)|(\d{6}(18|19|20)\d{2}(0[1-9]|10|11|12)([0-2]\d|30|31)\d{3}(\d|X|x)))[^0-9]`
	mvsMACExpr         = `(^([a-fA-F0-9]{2}(:[a-fA-F0-9]{2}){5})|[^a-zA-Z0-9]([a-fA-F0-9]{2}(:[a-fA-F0-9]{2}){5}))`
	mvsPhoneExpr       = `(?:\+?86|0{0,2}86)?1(?:3\d|4[5-79]|5[0-35-9]|6[5-7]|7[0-8]|8\d|9[189])\d{8}`
	mvsJSONExpr        = `(?is)^{.*}$`
	mvsAWSRegionExpr   = `((us(-gov)?|ap|ca|cn|eu|sa)-(central|(north|south)?(east|west)?)-\d)`
	mvsWindowsPathExpr = `[a-zA-Z]:[\\/](?:\w+[\\/])+.*\.\w+`
)

func mvsSpecializedExpr(expr string) mvsSpecializedKind {
	switch expr {
	case mvsCNIDExpr:
		return mvsSpecializedCNID
	case mvsMACExpr:
		return mvsSpecializedMAC
	case mvsPhoneExpr:
		return mvsSpecializedPhone
	case mvsJSONExpr:
		return mvsSpecializedJSON
	case mvsAWSRegionExpr:
		return mvsSpecializedAWSRegion
	case mvsWindowsPathExpr:
		return mvsSpecializedWindowsPath
	default:
		return mvsSpecializedNone
	}
}

func (k mvsSpecializedKind) exists(data []byte) bool {
	switch k {
	case mvsSpecializedCNID:
		return mvsHasCNID(data)
	case mvsSpecializedMAC:
		return mvsHasMAC(data)
	case mvsSpecializedPhone:
		return mvsHasPhone(data)
	case mvsSpecializedJSON:
		return len(data) >= 2 && data[0] == '{' && data[len(data)-1] == '}'
	case mvsSpecializedAWSRegion:
		return mvsHasAWSRegion(data)
	case mvsSpecializedWindowsPath:
		return mvsHasWindowsPath(data)
	default:
		return false
	}
}

func mvsSpecializedMask(data []byte, wanted uint64) uint64 {
	var found uint64
	set := func(kind mvsSpecializedKind) { found |= uint64(1) << kind }
	if wanted&(uint64(1)<<mvsSpecializedJSON) != 0 && len(data) >= 2 && data[0] == '{' && data[len(data)-1] == '}' {
		set(mvsSpecializedJSON)
	}
	for i, c := range data {
		if found&wanted == wanted {
			break
		}
		if wanted&(uint64(1)<<mvsSpecializedPhone) != 0 && found&(uint64(1)<<mvsSpecializedPhone) == 0 &&
			c == '1' && mvsPhoneAt(data, i) {
			set(mvsSpecializedPhone)
		}
		if wanted&(uint64(1)<<mvsSpecializedCNID) != 0 && found&(uint64(1)<<mvsSpecializedCNID) == 0 &&
			i > 0 && mvsDigit(c) && !mvsDigit(data[i-1]) && mvsCNIDAt(data, i) {
			set(mvsSpecializedCNID)
		}
		if c == ':' {
			if wanted&(uint64(1)<<mvsSpecializedMAC) != 0 && found&(uint64(1)<<mvsSpecializedMAC) == 0 &&
				mvsMACAt(data, i-2) {
				set(mvsSpecializedMAC)
			}
			if wanted&(uint64(1)<<mvsSpecializedWindowsPath) != 0 && found&(uint64(1)<<mvsSpecializedWindowsPath) == 0 &&
				mvsWindowsPathAt(data, i-1) {
				set(mvsSpecializedWindowsPath)
			}
		}
		if c == '-' && wanted&(uint64(1)<<mvsSpecializedAWSRegion) != 0 &&
			found&(uint64(1)<<mvsSpecializedAWSRegion) == 0 {
			if (i >= 2 && mvsAWSRegionAt(data, i-2)) || (i >= 6 && mvsAWSRegionAt(data, i-6)) {
				set(mvsSpecializedAWSRegion)
			}
		}
	}
	return found
}

func mvsHasPhone(data []byte) bool {
	for i := 0; i+11 <= len(data); i++ {
		if mvsPhoneAt(data, i) {
			return true
		}
	}
	return false
}

func mvsPhoneAt(data []byte, i int) bool {
	if i < 0 || i+11 > len(data) || data[i] != '1' || !mvsPhonePrefix(data[i+1], data[i+2]) {
		return false
	}
	for j := 3; j < 11; j++ {
		if !mvsDigit(data[i+j]) {
			return false
		}
	}
	return true
}

func mvsPhonePrefix(a, b byte) bool {
	switch a {
	case '3', '8':
		return mvsDigit(b)
	case '4':
		return b == '5' || b == '6' || b == '7' || b == '9'
	case '5':
		return mvsDigit(b) && b != '4'
	case '6':
		return b == '5' || b == '6' || b == '7'
	case '7':
		return b >= '0' && b <= '8'
	case '9':
		return b == '1' || b == '8' || b == '9'
	}
	return false
}

func mvsHasAWSRegion(data []byte) bool {
	for i := 0; i < len(data); i++ {
		if mvsAWSRegionAt(data, i) {
			return true
		}
	}
	return false
}

func mvsAWSRegionAt(data []byte, i int) bool {
	prefix := 0
	if i+2 > len(data) {
		return false
	}
	p0, p1 := data[i], data[i+1]
	if (p0 == 'a' && p1 == 'p') || (p0 == 'c' && (p1 == 'a' || p1 == 'n')) ||
		(p0 == 'e' && p1 == 'u') || (p0 == 's' && p1 == 'a') {
		prefix = 2
	} else if p0 == 'u' && p1 == 's' {
		prefix = 2
		if i+6 <= len(data) && bytes.Equal(data[i+2:i+6], []byte("-gov")) {
			prefix = 6
		}
	}
	if prefix == 0 || i+prefix >= len(data) || data[i+prefix] != '-' {
		return false
	}
	p := i + prefix + 1
	for _, middle := range [...]string{"central", "northeast", "northwest", "southeast", "southwest", "north", "south", "east", "west", ""} {
		if p+len(middle)+2 <= len(data) && bytes.Equal(data[p:p+len(middle)], []byte(middle)) &&
			data[p+len(middle)] == '-' && mvsDigit(data[p+len(middle)+1]) {
			return true
		}
	}
	return false
}

func mvsWord(c byte) bool { return mvsAlphaNum(c) || c == '_' }

func mvsHasWindowsPath(data []byte) bool {
	for i := 0; i+7 <= len(data); i++ {
		if mvsWindowsPathAt(data, i) {
			return true
		}
	}
	return false
}

func mvsWindowsPathAt(data []byte, i int) bool {
	if i < 0 || i+7 > len(data) ||
		!((data[i] >= 'a' && data[i] <= 'z') || (data[i] >= 'A' && data[i] <= 'Z')) ||
		data[i+1] != ':' || (data[i+2] != '\\' && data[i+2] != '/') {
		return false
	}
	p := i + 3
	start := p
	for p < len(data) && mvsWord(data[p]) {
		p++
	}
	if p == start || p >= len(data) || (data[p] != '\\' && data[p] != '/') {
		return false
	}
	for p++; p+1 < len(data) && data[p] != '\n'; p++ {
		if data[p] == '.' && mvsWord(data[p+1]) {
			return true
		}
	}
	return false
}

func mvsDigit(c byte) bool { return c >= '0' && c <= '9' }

func mvsHasCNID(data []byte) bool {
	// The 15-digit alternative in mvsCNIDExpr places $ before the required
	// trailing non-digit and is therefore unsatisfiable. The live alternative
	// is: non-digit + 18-byte ID + non-digit.
	for s := 1; s+18 < len(data); s++ {
		if mvsCNIDAt(data, s) {
			return true
		}
	}
	return false
}

func mvsCNIDAt(data []byte, s int) bool {
	if s < 1 || s+18 >= len(data) || mvsDigit(data[s-1]) || !mvsDigit(data[s]) {
		return false
	}
	allDigits := true
	for i := 0; i < 10; i++ {
		if !mvsDigit(data[s+i]) {
			allDigits = false
			break
		}
	}
	if !allDigits || (data[s+6] != '1' && data[s+6] != '2') ||
		(data[s+6] == '1' && data[s+7] != '8' && data[s+7] != '9') ||
		(data[s+6] == '2' && data[s+7] != '0') {
		return false
	}
	month := int(data[s+10]-'0')*10 + int(data[s+11]-'0')
	day := int(data[s+12]-'0')*10 + int(data[s+13]-'0')
	if !mvsDigit(data[s+10]) || !mvsDigit(data[s+11]) || month < 1 || month > 12 ||
		!mvsDigit(data[s+12]) || !mvsDigit(data[s+13]) || day > 31 {
		return false
	}
	if !mvsDigit(data[s+14]) || !mvsDigit(data[s+15]) || !mvsDigit(data[s+16]) {
		return false
	}
	last := data[s+17]
	if !mvsDigit(last) && last != 'X' && last != 'x' {
		return false
	}
	return !mvsDigit(data[s+18])
}

func mvsHex(c byte) bool {
	return mvsDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func mvsAlphaNum(c byte) bool {
	return mvsDigit(c) || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func mvsHasMAC(data []byte) bool {
	// Search the first separator with the optimized byte primitive, then verify
	// the fixed xx:xx:xx:xx:xx:xx structure around it.
	for base := 0; base < len(data); {
		rel := bytes.IndexByte(data[base:], ':')
		if rel < 0 {
			return false
		}
		colon := base + rel
		start := colon - 2
		if mvsMACAt(data, start) {
			return true
		}
		base = colon + 1
	}
	return false
}

func mvsMACAt(data []byte, start int) bool {
	if start < 0 || start+17 > len(data) || (start != 0 && mvsAlphaNum(data[start-1])) {
		return false
	}
	for i := 0; i < 17; i++ {
		if i%3 == 2 {
			if data[start+i] != ':' {
				return false
			}
		} else if !mvsHex(data[start+i]) {
			return false
		}
	}
	return true
}
