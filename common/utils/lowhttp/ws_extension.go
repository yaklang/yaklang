package lowhttp

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	websocketPermessageDeflate       = "permessage-deflate"
	websocketServerNoContextTakeover = "server_no_context_takeover"
	websocketClientNoContextTakeover = "client_no_context_takeover"
	websocketServerMaxWindowBits     = "server_max_window_bits"
	websocketClientMaxWindowBits     = "client_max_window_bits"
	websocketDefaultWindowBits       = 15
)

type PermessageDeflateParameters struct {
	ServerNoContextTakeover bool
	ClientNoContextTakeover bool
	ServerMaxWindowBits     int
	ClientMaxWindowBits     int
	ServerMaxWindowBitsSet  bool
	ClientMaxWindowBitsSet  bool
	ClientMaxWindowBitsBare bool
}

type WebsocketExtensions struct {
	Extensions            []string
	ClientContextTakeover bool
	ServerContextTakeover bool
	ClientMaxWindowBits   int
	ServerMaxWindowBits   int
	IsDeflate             bool
	PermessageDeflate     *PermessageDeflateParameters
}

type websocketExtensionParameter struct {
	name     string
	value    string
	hasValue bool
}

type websocketExtensionOption struct {
	name       string
	parameters []websocketExtensionParameter
}

type websocketExtensionScanner struct {
	raw string
	pos int
}

func isWebsocketTokenByte(b byte) bool {
	if b <= 0x20 || b >= 0x7f {
		return false
	}
	return !strings.ContainsRune("()<>@,;:\\\"/[]?={}\t", rune(b))
}

func (s *websocketExtensionScanner) skipOWS() {
	for s.pos < len(s.raw) && (s.raw[s.pos] == ' ' || s.raw[s.pos] == '\t') {
		s.pos++
	}
}

func (s *websocketExtensionScanner) token() (string, bool) {
	start := s.pos
	for s.pos < len(s.raw) && isWebsocketTokenByte(s.raw[s.pos]) {
		s.pos++
	}
	if start == s.pos {
		return "", false
	}
	return s.raw[start:s.pos], true
}

func (s *websocketExtensionScanner) quotedString() (string, bool) {
	if s.pos >= len(s.raw) || s.raw[s.pos] != '"' {
		return "", false
	}
	s.pos++
	var value strings.Builder
	for s.pos < len(s.raw) {
		b := s.raw[s.pos]
		s.pos++
		switch b {
		case '"':
			return value.String(), true
		case '\\':
			if s.pos >= len(s.raw) {
				return "", false
			}
			b = s.raw[s.pos]
			s.pos++
			if b < 0x20 || b >= 0x7f {
				return "", false
			}
			value.WriteByte(b)
		default:
			if b == '\r' || b == '\n' || b < 0x20 && b != '\t' {
				return "", false
			}
			value.WriteByte(b)
		}
	}
	return "", false
}

func websocketHeaderValues(headers http.Header, name string) []string {
	var values []string
	for key, current := range headers {
		if strings.EqualFold(key, name) {
			values = append(values, current...)
		}
	}
	return values
}

func parseWebsocketExtensionHeader(headers http.Header) ([]websocketExtensionOption, error) {
	// Some lowhttp parsers preserve the wire spelling (WebSocket), while
	// net/http canonicalizes it as Websocket. Header.Get/Values cannot bridge
	// that mismatch because map lookup is case-sensitive.
	values := websocketHeaderValues(headers, "Sec-WebSocket-Extensions")
	if len(values) == 0 {
		return nil, nil
	}
	s := websocketExtensionScanner{raw: strings.Join(values, ",")}
	options := make([]websocketExtensionOption, 0, 1)
	for {
		s.skipOWS()
		name, ok := s.token()
		if !ok {
			return nil, fmt.Errorf("websocket: invalid extension token at byte %d", s.pos)
		}
		option := websocketExtensionOption{name: strings.ToLower(name)}
		for {
			s.skipOWS()
			if s.pos >= len(s.raw) || s.raw[s.pos] == ',' {
				break
			}
			if s.raw[s.pos] != ';' {
				return nil, fmt.Errorf("websocket: invalid extension separator at byte %d", s.pos)
			}
			s.pos++
			s.skipOWS()
			parameterName, ok := s.token()
			if !ok {
				return nil, fmt.Errorf("websocket: invalid extension parameter at byte %d", s.pos)
			}
			parameter := websocketExtensionParameter{name: strings.ToLower(parameterName)}
			s.skipOWS()
			if s.pos < len(s.raw) && s.raw[s.pos] == '=' {
				s.pos++
				s.skipOWS()
				parameter.hasValue = true
				if s.pos < len(s.raw) && s.raw[s.pos] == '"' {
					parameter.value, ok = s.quotedString()
					if ok {
						for i := 0; i < len(parameter.value); i++ {
							if !isWebsocketTokenByte(parameter.value[i]) {
								ok = false
								break
							}
						}
					}
				} else {
					parameter.value, ok = s.token()
				}
				if !ok {
					return nil, fmt.Errorf("websocket: invalid value for extension parameter %q", parameter.name)
				}
			}
			option.parameters = append(option.parameters, parameter)
		}
		options = append(options, option)
		s.skipOWS()
		if s.pos == len(s.raw) {
			return options, nil
		}
		if s.raw[s.pos] != ',' {
			return nil, fmt.Errorf("websocket: invalid extension list at byte %d", s.pos)
		}
		s.pos++
		if s.pos == len(s.raw) {
			return nil, fmt.Errorf("websocket: trailing comma in extension list")
		}
	}
}

func parseWebsocketWindowBits(parameter websocketExtensionParameter) (int, error) {
	if !parameter.hasValue || len(parameter.value) == 0 || len(parameter.value) > 1 && parameter.value[0] == '0' {
		return 0, fmt.Errorf("websocket: invalid %s value %q", parameter.name, parameter.value)
	}
	bits, err := strconv.Atoi(parameter.value)
	if err != nil || bits < 8 || bits > 15 {
		return 0, fmt.Errorf("websocket: invalid %s value %q", parameter.name, parameter.value)
	}
	return bits, nil
}

func parsePermessageDeflate(option websocketExtensionOption, response bool) (PermessageDeflateParameters, error) {
	var result PermessageDeflateParameters
	seen := make(map[string]struct{}, len(option.parameters))
	for _, parameter := range option.parameters {
		if _, ok := seen[parameter.name]; ok {
			return result, fmt.Errorf("websocket: duplicate permessage-deflate parameter %q", parameter.name)
		}
		seen[parameter.name] = struct{}{}
		switch parameter.name {
		case websocketServerNoContextTakeover:
			if parameter.hasValue {
				return result, fmt.Errorf("websocket: %s must not have a value", parameter.name)
			}
			result.ServerNoContextTakeover = true
		case websocketClientNoContextTakeover:
			if parameter.hasValue {
				return result, fmt.Errorf("websocket: %s must not have a value", parameter.name)
			}
			result.ClientNoContextTakeover = true
		case websocketServerMaxWindowBits:
			bits, err := parseWebsocketWindowBits(parameter)
			if err != nil {
				return result, err
			}
			result.ServerMaxWindowBitsSet = true
			result.ServerMaxWindowBits = bits
		case websocketClientMaxWindowBits:
			result.ClientMaxWindowBitsSet = true
			if !parameter.hasValue {
				if response {
					return result, fmt.Errorf("websocket: client_max_window_bits requires a value in a response")
				}
				result.ClientMaxWindowBitsBare = true
				continue
			}
			bits, err := parseWebsocketWindowBits(parameter)
			if err != nil {
				return result, err
			}
			result.ClientMaxWindowBits = bits
		default:
			return result, fmt.Errorf("websocket: unsupported permessage-deflate parameter %q", parameter.name)
		}
	}
	return result, nil
}

func parsedWebsocketExtensions(headers http.Header, response bool) (*WebsocketExtensions, []PermessageDeflateParameters, error) {
	options, err := parseWebsocketExtensionHeader(headers)
	if err != nil {
		return nil, nil, err
	}
	extensions := &WebsocketExtensions{Extensions: make([]string, 0, len(options))}
	offers := make([]PermessageDeflateParameters, 0, 1)
	for _, option := range options {
		extensions.Extensions = append(extensions.Extensions, option.name)
		if option.name != websocketPermessageDeflate {
			continue
		}
		parameters, parseErr := parsePermessageDeflate(option, response)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		offers = append(offers, parameters)
	}
	if response && len(offers) > 1 {
		return nil, nil, fmt.Errorf("websocket: multiple permessage-deflate responses")
	}
	if response && len(offers) == 1 {
		extensions.applyPermessageDeflate(offers[0])
	}
	return extensions, offers, nil
}

func (ext *WebsocketExtensions) applyPermessageDeflate(parameters PermessageDeflateParameters) {
	ext.IsDeflate = true
	ext.PermessageDeflate = &parameters
	ext.ClientContextTakeover = !parameters.ClientNoContextTakeover
	ext.ServerContextTakeover = !parameters.ServerNoContextTakeover
	ext.ClientMaxWindowBits = websocketDefaultWindowBits
	ext.ServerMaxWindowBits = websocketDefaultWindowBits
	if parameters.ClientMaxWindowBitsSet {
		ext.ClientMaxWindowBits = parameters.ClientMaxWindowBits
	}
	if parameters.ServerMaxWindowBitsSet {
		ext.ServerMaxWindowBits = parameters.ServerMaxWindowBits
	}
}

func permessageDeflateResponseMatchesOffer(response, offer PermessageDeflateParameters) bool {
	if offer.ServerNoContextTakeover && !response.ServerNoContextTakeover {
		return false
	}
	if offer.ServerMaxWindowBitsSet {
		if !response.ServerMaxWindowBitsSet || response.ServerMaxWindowBits > offer.ServerMaxWindowBits {
			return false
		}
	}
	if response.ClientMaxWindowBitsSet {
		if !offer.ClientMaxWindowBitsSet {
			return false
		}
		if !offer.ClientMaxWindowBitsBare && response.ClientMaxWindowBits > offer.ClientMaxWindowBits {
			return false
		}
	}
	return true
}

func ValidateWebsocketExtensions(requestHeader, responseHeader http.Header) (*WebsocketExtensions, error) {
	extensions, responses, err := parsedWebsocketExtensions(responseHeader, true)
	if err != nil {
		return nil, err
	}
	if len(responses) == 0 {
		if len(extensions.Extensions) != 0 {
			return nil, fmt.Errorf("websocket: response contains an extension without a matching offer")
		}
		return extensions, nil
	}
	offeredExtensions, offers, err := parsedWebsocketExtensions(requestHeader, false)
	if err != nil {
		return nil, fmt.Errorf("websocket: invalid extension offer: %w", err)
	}
	for _, responseName := range extensions.Extensions {
		matched := false
		for _, offerName := range offeredExtensions.Extensions {
			if strings.EqualFold(responseName, offerName) {
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("websocket: response extension %q was not offered", responseName)
		}
	}
	response := responses[0]
	for _, offer := range offers {
		if !permessageDeflateResponseMatchesOffer(response, offer) {
			continue
		}
		return extensions, nil
	}
	return nil, fmt.Errorf("websocket: permessage-deflate response does not match any offer")
}

func GetWebsocketExtensions(headers http.Header) *WebsocketExtensions {
	extensions, _, err := parsedWebsocketExtensions(headers, true)
	if err != nil {
		return &WebsocketExtensions{}
	}
	return extensions
}

func (ext *WebsocketExtensions) readFlateContextTakeover(serverMode bool) bool {
	if ext == nil || !ext.IsDeflate {
		return false
	}
	if serverMode {
		return ext.ClientContextTakeover
	}
	return ext.ServerContextTakeover
}

func (ext *WebsocketExtensions) writeFlateContextTakeover(serverMode bool) bool {
	if ext == nil || !ext.IsDeflate {
		return false
	}
	if serverMode {
		return ext.ServerContextTakeover
	}
	return ext.ClientContextTakeover
}

func (ext *WebsocketExtensions) readFlateWindowBits(serverMode bool) int {
	if ext == nil || !ext.IsDeflate {
		return websocketDefaultWindowBits
	}
	if serverMode {
		return ext.ClientMaxWindowBits
	}
	return ext.ServerMaxWindowBits
}

func (ext *WebsocketExtensions) writeFlateWindowBits(serverMode bool) int {
	if ext == nil || !ext.IsDeflate {
		return websocketDefaultWindowBits
	}
	if serverMode {
		return ext.ServerMaxWindowBits
	}
	return ext.ClientMaxWindowBits
}

func formatPermessageDeflateOffer(parameters PermessageDeflateParameters) string {
	parts := []string{websocketPermessageDeflate}
	if parameters.ClientNoContextTakeover {
		parts = append(parts, websocketClientNoContextTakeover)
	}
	if parameters.ClientMaxWindowBitsSet {
		value := websocketClientMaxWindowBits
		if !parameters.ClientMaxWindowBitsBare {
			value += "=" + strconv.Itoa(parameters.ClientMaxWindowBits)
		}
		parts = append(parts, value)
	}
	if parameters.ServerNoContextTakeover {
		parts = append(parts, websocketServerNoContextTakeover)
	}
	if parameters.ServerMaxWindowBitsSet {
		parts = append(parts, websocketServerMaxWindowBits+"="+strconv.Itoa(parameters.ServerMaxWindowBits))
	}
	return strings.Join(parts, "; ")
}
