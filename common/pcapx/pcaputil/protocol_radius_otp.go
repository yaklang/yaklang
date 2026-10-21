package pcaputil

import (
	"bytes"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The prefix capture supplies RE2's equivalent of the RFC context boundary.
// Only two bounded matches are needed to detect an ambiguous display message.
var radiusOTPChallengePattern = regexp.MustCompile(`(^|[^A-Za-z0-9_-])otp-([^ \t\r\n]+)[ \t]+([^ \t\r\n]+)[ \t]+([^ \t\r\n]+)`)
var radiusOTPExtPattern = regexp.MustCompile(`^[ \t]+(ext(?:,[^ \t\r\n]*)?)($|[ \t\r\n])`)

func radiusOTPASCII(data []byte) string {
	var s strings.Builder
	for _, b := range data {
		if b < 128 {
			s.WriteByte(b)
		} else {
			s.WriteRune(utf8.RuneError)
		}
	}
	return s.String()
}
func radiusOTPFields(inner map[string]any, code byte, data []byte, max int, note func(string, bool)) (bool, error) {
	inner["Type Name"] = "OTP"
	note("eap-otp-context-not-verified", false)
	if len(data) > 0 && data[len(data)-1] == 0 {
		note("eap-otp-null-terminated", true)
		return true, nil
	}
	if code == 1 {
		return radiusOTPChallengeFields(inner, data, max, note)
	}
	return radiusOTPResponseFields(inner, data, max, note)
}
func radiusOTPChallengeFields(inner map[string]any, data []byte, max int, note func(string, bool)) (bool, error) {
	matches := radiusOTPChallengePattern.FindAllSubmatchIndex(data, 2)
	if len(matches) == 0 {
		note("eap-otp-challenge-syntax", true)
		return true, nil
	}
	if len(matches) > 1 {
		note("eap-otp-challenge-ambiguous", false)
		return true, protocolError(ErrUnsupportedFeature, "Multiple OTP challenge contexts")
	}
	m := matches[0]
	start, end := m[3], m[1]
	algo, sequence, seed := data[m[4]:m[5]], data[m[6]:m[7]], data[m[8]:m[9]]
	extended := false
	var capsData []byte
	count := 0
	if e := radiusOTPExtPattern.FindSubmatchIndex(data[end:]); e != nil {
		extended = true
		cap := data[end+e[2] : end+e[3]]
		end += e[3]
		if len(cap) > 3 {
			capsData = cap[4:]
			count = bytes.Count(capsData, []byte{','}) + 1
		}
	}
	inner["OTP Challenge"], inner["OTP Prefix"], inner["OTP Suffix"] = bytes.Clone(data[start:end]), bytes.Clone(data[:start]), bytes.Clone(data[end:])
	inner["OTP Algorithm"], inner["OTP Sequence"] = radiusOTPASCII(algo), radiusOTPASCII(sequence)
	inner["OTP Extended"], inner["OTP Capability Count"] = extended, count
	if !extended && (end == len(data) || data[end] != ' ' && data[end] != '\r' && data[end] != '\n') {
		note("eap-otp-challenge-termination", true)
	}
	decimal := len(sequence) > 0
	for _, b := range sequence {
		decimal = decimal && b >= '0' && b <= '9'
	}
	if !decimal {
		note("eap-otp-challenge-sequence", true)
	}
	ascii := true
	invalidASCIISeed := false
	alnum := len(seed) >= 1 && len(seed) <= 16
	for _, b := range seed {
		ascii = ascii && b < 128
		if b < 128 && !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9') {
			invalidASCIISeed = true
		}
		alnum = alnum && (b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9')
	}
	var err error
	if !ascii {
		inner["OTP Seed Raw"] = bytes.Clone(seed)
		if invalidASCIISeed {
			note("eap-otp-challenge-seed", true)
		}
		note("eap-otp-seed-encoding-unimplemented", false)
		err = protocolError(ErrUnsupportedFeature, "OTP seed encoding requires separate context")
	} else {
		inner["OTP Seed"] = strings.ToLower(string(seed))
		if !alnum {
			note("eap-otp-challenge-seed", true)
		}
	}
	if !bytes.Equal(algo, []byte("md4")) && !bytes.Equal(algo, []byte("md5")) && !bytes.Equal(algo, []byte("sha1")) {
		note("eap-otp-algorithm-unimplemented", false)
		err = protocolError(ErrUnsupportedFeature, "OTP algorithm is unimplemented")
	}
	invalidCaps := false
	for _, b := range capsData {
		if b != ',' && (b <= 32 || b >= 127) {
			invalidCaps = true
		}
	}
	if invalidCaps {
		note("eap-otp-capability-syntax", true)
		return true, err
	}
	if count > max {
		return true, protocolError(ErrResourceExceeded, "OTP capability count exceeds collection budget")
	}
	caps := make([]string, 0, count)
	if count > 0 {
		for _, part := range bytes.Split(capsData, []byte{','}) {
			caps = append(caps, string(part))
		}
	}
	inner["OTP Capabilities"] = caps
	return true, err
}
func radiusOTPResponseFields(inner map[string]any, data []byte, max int, note func(string, bool)) (bool, error) {
	// RFC 2243 tokens may contain SP. Remove only the context line ending
	// before validating extension tokens; TrimSpace would hide empty arguments
	// or turn an unknown type with leading SP into a known type.
	line := bytes.TrimSuffix(data, []byte("\n"))
	line = bytes.TrimSuffix(line, []byte("\r"))
	body := bytes.Trim(data, " \t\r\n\v\f")
	format := "standard"
	explicit := ""
	if at := bytes.IndexByte(line, ':'); at >= 0 {
		explicit = strings.ToLower(radiusOTPASCII(line[:at]))
		body = bytes.Trim(line[at+1:], " \t\r\n\v\f")
		format = "extended-" + explicit
		if !radiusOTPExtendedSyntax(line, explicit == "hex" || explicit == "word") {
			inner["OTP Format"], inner["OTP Candidate Count"], inner["OTP Candidates"] = format, 0, []map[string]any{}
			inner["OTP Extended Arguments"] = bytes.Clone(line[at+1:])
			note("eap-otp-extended-response-syntax", true)
			return true, nil
		}
		if explicit != "hex" && explicit != "word" {
			inner["OTP Format"], inner["OTP Candidate Count"], inner["OTP Candidates"] = format, 0, []map[string]any{}
			inner["OTP Extended Arguments"] = bytes.Clone(line[at+1:])
			note("eap-otp-extension-unimplemented", false)
			return true, protocolError(ErrUnsupportedFeature, "OTP response extension requires initialization context")
		}
	}
	inner["OTP Format"] = format
	var compact [16]byte
	n := 0
	hexPossible := true
	var tokenCount int
	inToken := false
	for _, b := range body {
		if radiusOTPWhite(b) {
			inToken = false
			continue
		}
		if !inToken {
			tokenCount++
			inToken = true
		}
		if n < 16 {
			compact[n] = b
		} else {
			hexPossible = false
		}
		n++
	}
	var hexValue [8]byte
	hexValid := false
	if explicit != "word" && n == 16 && hexPossible {
		_, err := hex.Decode(hexValue[:], compact[:])
		hexValid = err == nil
	}
	var wordValue [8]byte
	var normalized string
	var wordsFound, wordChecksum bool
	if explicit != "hex" {
		wordValue, normalized, wordsFound, wordChecksum = radiusOTPDecodeWords(body)
	}
	count := 0
	if hexValid {
		count++
	}
	if wordsFound {
		count++
	}
	inner["OTP Candidate Count"] = count
	checksumFatal := wordsFound && !wordChecksum && !hexValid
	if checksumFatal {
		note("eap-otp-checksum", true)
	}
	if count > max || wordsFound && 6 > max {
		return true, protocolError(ErrResourceExceeded, "OTP candidate/word count exceeds collection budget")
	}
	candidates := make([]map[string]any, 0, count)
	if hexValid {
		candidates = append(candidates, map[string]any{"Format": "hex", "Value": bytes.Clone(hexValue[:]), "Checksum": "not-applicable", "EAP Applicability": "unconfirmed"})
	}
	if wordsFound {
		check := "invalid"
		if wordChecksum {
			check = "valid"
		}
		applicability := "unconfirmed"
		if explicit == "" {
			applicability = "specified"
		}
		candidates = append(candidates, map[string]any{"Format": "six-word", "Value": bytes.Clone(wordValue[:]), "Checksum": check, "Words": strings.Fields(normalized), "EAP Applicability": applicability})
	}
	inner["OTP Candidates"] = candidates
	if wordsFound && wordChecksum && explicit == "" {
		return true, nil
	}
	if hexValid || wordsFound && wordChecksum {
		note("eap-otp-eap-encoding-not-verified", false)
		return true, protocolError(ErrUnsupportedFeature, "Generic OTP encoding observed; EAP applicability unconfirmed")
	}
	if wordsFound {
		return true, nil
	}
	if tokenCount == 6 && explicit != "hex" {
		if explicit == "word" {
			note("eap-otp-dictionary-word", true)
			return true, nil
		}
		if tokenCount > max {
			delete(inner, "OTP Candidates")
			return true, protocolError(ErrResourceExceeded, "OTP uninterpreted word count exceeds collection budget")
		}
		words := make([]string, 0, 6)
		for at := 0; at < len(body); {
			if radiusOTPWhite(body[at]) {
				at++
				continue
			}
			end := at
			for end < len(body) && !radiusOTPWhite(body[end]) {
				end++
			}
			words = append(words, strings.ToValidUTF8(string(body[at:end]), "\uFFFD"))
			at = end
		}
		inner["OTP Uninterpreted Words"] = words
		note("eap-otp-alternate-dictionary-unimplemented", false)
		return true, protocolError(ErrUnsupportedFeature, "Alternate OTP words require an observed algorithm context")
	}
	note("eap-otp-response-format", true)
	return true, nil
}

// RFC 2243 section 2.1: type and each colon-separated argument are nonempty
// CHAR sequences excluding CTLs. This checks the bounded input without
// allocating a token slice, even when the extension type is unknown. Known
// hex/word arguments additionally admit HTAB through their specific LWSP
// productions in section 3.1; embedded line endings remain invalid.
func radiusOTPExtendedSyntax(line []byte, allowArgumentTab bool) bool {
	start := 0
	for i, b := range line {
		if b == ':' {
			if i == start {
				return false
			}
			start = i + 1
		} else if (b < 32 || b >= 127) && !(allowArgumentTab && start > 0 && b == '\t') {
			return false
		}
	}
	return start < len(line)
}
