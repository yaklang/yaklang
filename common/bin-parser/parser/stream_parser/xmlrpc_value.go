package stream_parser

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	pcre2 "github.com/VillanCh/go-pcre2-lite"
)

// XMLRPCValue retains the source spelling and typed value. Struct members stay
// ordered; duplicate member names are not silently collapsed into a map.
type XMLRPCValue struct {
	Kind        string
	Raw         string
	Text        string
	IntValue    int32
	BoolValue   bool
	DoubleValue float64
	Bytes       []byte
	Elements    []*XMLRPCValue
	Members     []XMLRPCMember
}

type XMLRPCMember struct {
	Name  string
	Value *XMLRPCValue
}

// XMLRPCMessage describes a document, not an invocation. No method is called.
type XMLRPCMessage struct {
	Kind   string
	Method string
	Params []*XMLRPCValue
	Fault  *XMLRPCValue
}

type xmlrpcElement struct {
	name     string
	text     string
	raw      string
	children []*xmlrpcElement
}

func xmlrpcSpace(text string) bool { return strings.Trim(text, " \t\r\n") == "" }

// encoding/xml accepts processing-instruction text without enforcing the XML
// declaration grammar (including a missing version or duplicate attributes).
// Check the original declaration, not entity-decoded synthetic attributes.
// DollarEndOnly preserves RE2's strict end anchor. UTF-8 and the 1 MiB input
// bound are checked before this byte-oriented matcher is called. This single
// process-lifetime compiled object is shared safely; PCRE2 reuses a separate
// scratch per concurrent match, rather than compiling/allocating per document.
// Its C code and peak-concurrency scratch pool remain live until process exit.
var xmlrpcDeclaration = pcre2.MustCompile(`^<\?xml[ \t\r\n]+version[ \t\r\n]*=[ \t\r\n]*("1\.0"|'1\.0')([ \t\r\n]+encoding[ \t\r\n]*=[ \t\r\n]*("[Uu][Tt][Ff]-8"|'[Uu][Tt][Ff]-8'))?([ \t\r\n]+standalone[ \t\r\n]*=[ \t\r\n]*("(yes|no)"|'(yes|no)'))?[ \t\r\n]*\?>$`, pcre2.CompileOptions{DollarEndOnly: true})

func validateXMLRPCDeclaration(matcher *pcre2.Regexp, declaration string) error {
	matched, err := matcher.MatchString(declaration)
	if err != nil {
		return fmt.Errorf("xmlrpc: declaration match failed: %w", err)
	}
	if !matched {
		return fmt.Errorf("xmlrpc: invalid declaration or unsupported processing instruction")
	}
	return nil
}

// The XML-RPC vocabulary has no attributes or namespaces. This bounded reader
// accepts UTF-8 XML, comments and CDATA, but no DTD, custom entities or processing
// instructions other than an initial XML declaration. It performs no I/O.
func decodeXMLRPCText(text string, depthLimit int) (*XMLRPCMessage, error) {
	if len(text) > 1<<20 || !utf8.ValidString(text) || depthLimit < 1 || depthLimit > 128 {
		return nil, fmt.Errorf("xmlrpc: invalid text size, encoding or depth limit")
	}
	text = strings.TrimPrefix(text, "\ufeff")
	decoder := xml.NewDecoder(strings.NewReader(text))
	nodes := 0
	var read func(xml.StartElement, int, int) (*xmlrpcElement, error)
	read = func(start xml.StartElement, offset, depth int) (*xmlrpcElement, error) {
		nodes++
		if depth > depthLimit || nodes > 100000 || start.Name.Space != "" || strings.Contains(start.Name.Local, ":") || len(start.Attr) != 0 {
			return nil, fmt.Errorf("xmlrpc: unsupported attributes/namespace or structure limit exceeded")
		}
		node := &xmlrpcElement{name: start.Name.Local}
		var content strings.Builder
		for {
			childOffset := int(decoder.InputOffset())
			token, err := decoder.Token()
			if err != nil {
				return nil, fmt.Errorf("xmlrpc: %w", err)
			}
			switch token := token.(type) {
			case xml.StartElement:
				child, err := read(token, childOffset, depth+1)
				if err != nil {
					return nil, err
				}
				node.children = append(node.children, child)
			case xml.EndElement:
				node.text = content.String()
				node.raw = text[offset:int(decoder.InputOffset())]
				return node, nil
			case xml.CharData:
				content.Write(token)
			case xml.Comment:
			default:
				return nil, fmt.Errorf("xmlrpc: unsupported XML directive or processing instruction")
			}
		}
	}
	var root *xmlrpcElement
	for {
		offset := int(decoder.InputOffset())
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("xmlrpc: %w", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			if root != nil {
				return nil, fmt.Errorf("xmlrpc: multiple document roots")
			}
			root, err = read(token, offset, 1)
			if err != nil {
				return nil, err
			}
		case xml.CharData:
			if !xmlrpcSpace(string(token)) {
				return nil, fmt.Errorf("xmlrpc: text outside document root")
			}
		case xml.Comment:
		case xml.ProcInst:
			if offset != 0 || token.Target != "xml" {
				return nil, fmt.Errorf("xmlrpc: invalid declaration or unsupported processing instruction")
			}
			if err := validateXMLRPCDeclaration(xmlrpcDeclaration, text[:int(decoder.InputOffset())]); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("xmlrpc: unsupported XML directive")
		}
	}
	if root == nil || !xmlrpcSpace(root.text) {
		return nil, fmt.Errorf("xmlrpc: missing or mixed-content document root")
	}
	message := &XMLRPCMessage{}
	switch root.name {
	case "methodCall":
		message.Kind = "request"
		if len(root.children) < 1 || len(root.children) > 2 || root.children[0].name != "methodName" || len(root.children[0].children) != 0 {
			return nil, fmt.Errorf("xmlrpc: invalid methodCall children")
		}
		message.Method = root.children[0].text
		if message.Method == "" || strings.IndexFunc(message.Method, func(c rune) bool {
			return !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_.:/", c))
		}) >= 0 {
			return nil, fmt.Errorf("xmlrpc: invalid method name")
		}
		if len(root.children) == 2 {
			var err error
			message.Params, err = xmlrpcParams(root.children[1])
			if err != nil {
				return nil, err
			}
		}
	case "methodResponse":
		if len(root.children) != 1 {
			return nil, fmt.Errorf("xmlrpc: response requires exactly one params or fault element")
		}
		child := root.children[0]
		if child.name == "params" {
			message.Kind = "success"
			var err error
			message.Params, err = xmlrpcParams(child)
			if err != nil || len(message.Params) != 1 {
				return nil, fmt.Errorf("xmlrpc: response requires one result: %v", err)
			}
		} else if child.name == "fault" && xmlrpcSpace(child.text) && len(child.children) == 1 {
			message.Kind = "fault"
			var err error
			message.Fault, err = xmlrpcDecodeValue(child.children[0])
			if err != nil || message.Fault.Kind != "struct" || len(message.Fault.Members) != 2 {
				return nil, fmt.Errorf("xmlrpc: invalid fault structure: %v", err)
			}
			code, description := false, false
			for _, member := range message.Fault.Members {
				switch member.Name {
				case "faultCode":
					code = member.Value.Kind == "int" || member.Value.Kind == "i4"
				case "faultString":
					description = member.Value.Kind == "string"
				}
			}
			if !code || !description {
				return nil, fmt.Errorf("xmlrpc: missing or mistyped fault fields")
			}
		} else {
			return nil, fmt.Errorf("xmlrpc: invalid response children")
		}
	default:
		return nil, fmt.Errorf("xmlrpc: unrecognized document root")
	}
	return message, nil
}

func xmlrpcParams(node *xmlrpcElement) ([]*XMLRPCValue, error) {
	if node.name != "params" || !xmlrpcSpace(node.text) {
		return nil, fmt.Errorf("xmlrpc: invalid params element")
	}
	var values []*XMLRPCValue
	for _, param := range node.children {
		if param.name != "param" || !xmlrpcSpace(param.text) || len(param.children) != 1 {
			return nil, fmt.Errorf("xmlrpc: invalid param element")
		}
		value, err := xmlrpcDecodeValue(param.children[0])
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func xmlrpcDecodeValue(node *xmlrpcElement) (*XMLRPCValue, error) {
	if node.name != "value" {
		return nil, fmt.Errorf("xmlrpc: expected value element")
	}
	value := &XMLRPCValue{Raw: node.raw}
	if len(node.children) == 0 {
		value.Kind, value.Text = "string", node.text
		return value, nil
	}
	if len(node.children) != 1 || !xmlrpcSpace(node.text) {
		return nil, fmt.Errorf("xmlrpc: mixed or multiple value types")
	}
	typed := node.children[0]
	value.Kind, value.Text = typed.name, typed.text
	if typed.name != "array" && typed.name != "struct" && len(typed.children) != 0 {
		return nil, fmt.Errorf("xmlrpc: scalar contains elements")
	}
	switch typed.name {
	case "int", "i4":
		integer, err := strconv.ParseInt(typed.text, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("xmlrpc: invalid signed 32-bit integer")
		}
		value.IntValue = int32(integer)
	case "boolean":
		if typed.text != "0" && typed.text != "1" {
			return nil, fmt.Errorf("xmlrpc: invalid boolean")
		}
		value.BoolValue = typed.text == "1"
	case "string":
	case "double":
		// XML-RPC's original scalar grammar is decimal, not Go hex floats,
		// non-finite values or implementation-specific integer extensions.
		if !xmlrpcDecimal(typed.text) {
			return nil, fmt.Errorf("xmlrpc: invalid decimal double")
		}
		number, err := strconv.ParseFloat(typed.text, 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return nil, fmt.Errorf("xmlrpc: double out of range")
		}
		value.DoubleValue = number
	case "dateTime.iso8601":
		if _, err := time.Parse("20060102T15:04:05", typed.text); err != nil || len(typed.text) != 17 {
			return nil, fmt.Errorf("xmlrpc: unsupported or invalid date spelling")
		}
		// No timezone is specified by XML-RPC. Preserve the text, not a
		// misleading UTC conversion. Extended ISO-8601 spellings are not claimed.
	case "base64":
		encoded := strings.Map(func(c rune) rune {
			if strings.ContainsRune(" \t\r\n", c) {
				return -1
			}
			return c
		}, typed.text)
		var err error
		value.Bytes, err = base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("xmlrpc: invalid base64")
		}
	case "array":
		if !xmlrpcSpace(typed.text) || len(typed.children) != 1 || typed.children[0].name != "data" || !xmlrpcSpace(typed.children[0].text) {
			return nil, fmt.Errorf("xmlrpc: invalid array data")
		}
		for _, child := range typed.children[0].children {
			item, err := xmlrpcDecodeValue(child)
			if err != nil {
				return nil, err
			}
			value.Elements = append(value.Elements, item)
		}
	case "struct":
		if !xmlrpcSpace(typed.text) {
			return nil, fmt.Errorf("xmlrpc: text in struct")
		}
		for _, member := range typed.children {
			if member.name != "member" || !xmlrpcSpace(member.text) || len(member.children) != 2 {
				return nil, fmt.Errorf("xmlrpc: invalid struct member")
			}
			name, child := member.children[0], member.children[1]
			// The specification does not assign semantic meaning to member
			// order. Accept either name/value order, retaining member order.
			if name.name == "value" {
				name, child = child, name
			}
			if name.name != "name" || len(name.children) != 0 {
				return nil, fmt.Errorf("xmlrpc: missing member name")
			}
			item, err := xmlrpcDecodeValue(child)
			if err != nil {
				return nil, err
			}
			value.Members = append(value.Members, XMLRPCMember{Name: name.text, Value: item})
		}
	default:
		return nil, fmt.Errorf("xmlrpc: unsupported value type")
	}
	return value, nil
}

func xmlrpcDecimal(text string) bool {
	if len(text) > 0 && (text[0] == '+' || text[0] == '-') {
		text = text[1:]
	}
	digits, dot := 0, false
	for _, c := range text {
		if c >= '0' && c <= '9' {
			digits++
		} else if c == '.' && !dot {
			dot = true
		} else {
			return false
		}
	}
	return digits > 0
}
