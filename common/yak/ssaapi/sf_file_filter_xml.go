package ssaapi

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/antchfx/xpath"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/htmlquery"
	"github.com/yaklang/yaklang/common/utils/jsonquery"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"gopkg.in/yaml.v3"
	"strings"
)

type FileFilterXpathKind string

const (
	FileFilterXPathUnValid FileFilterXpathKind = "unValid"
	FileFilterXPathXML     FileFilterXpathKind = "xml"
	FileFilterXPathJson    FileFilterXpathKind = "json"
	FileFilterXPathYaml    FileFilterXpathKind = "yaml"
)

type FileXPathMatcher struct {
	Expr    string
	XMLExpr *xpath.Expr
}

func NewFileXPathMatcher(expr string) (*FileXPathMatcher, error) {
	f := &FileXPathMatcher{
		Expr: expr,
	}
	xmle, err := xpath.Compile(expr)
	if err != nil {
		return nil, err
	}
	f.XMLExpr = xmle
	return f, nil
}

func (f *FileXPathMatcher) Match(content string) ([]string, error) {
	contentType := checkFileContentType([]byte(content))
	var result []string

	switch contentType {
	case FileFilterXPathXML:
		if f.XMLExpr == nil {
			return nil, fmt.Errorf("xml expression required")
		}
		top, err := htmlquery.Parse(strings.NewReader(content))
		if err != nil {
			return nil, err
		}
		t := f.XMLExpr.Evaluate(htmlquery.CreateXPathNavigator(top))
		switch t := t.(type) {
		case *xpath.NodeIterator:
			for t.MoveNext() {
				nav := t.Current().(*htmlquery.NodeNavigator)
				node := nav.Current()
				str := htmlquery.InnerText(node)
				result = append(result, str)
			}
		default:
			str := codec.AnyToString(t)
			result = append(result, str)
		}
		return result, nil
	case FileFilterXPathJson, FileFilterXPathYaml:
		jsonContent, err := structuredContentToJSON(content)
		if err != nil {
			return nil, err
		}
		result, err = f.matchContentByJsonQuery(jsonContent)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported content type: %s", contentType)
	}
	return lo.Uniq(result), nil
}

func parseStructuredContent(content string) (any, error) {
	switch checkFileContentType([]byte(content)) {
	case FileFilterXPathJson:
		var data any
		if err := json.Unmarshal([]byte(content), &data); err != nil {
			return nil, err
		}
		return data, nil
	case FileFilterXPathYaml:
		var data any
		if err := yaml.Unmarshal([]byte(content), &data); err != nil {
			return nil, err
		}
		return normalizeYAMLValue(data), nil
	default:
		return nil, fmt.Errorf("unsupported structured content")
	}
}

func structuredContentToJSON(content string) (string, error) {
	data, err := parseStructuredContent(content)
	if err != nil {
		return "", err
	}
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func normalizeYAMLValue(raw any) any {
	switch ret := raw.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(ret))
		for k, v := range ret {
			normalized[k] = normalizeYAMLValue(v)
		}
		return normalized
	case map[any]any:
		normalized := make(map[string]any, len(ret))
		for k, v := range ret {
			normalized[codec.AnyToString(k)] = normalizeYAMLValue(v)
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(ret))
		for _, item := range ret {
			normalized = append(normalized, normalizeYAMLValue(item))
		}
		return normalized
	default:
		return raw
	}
}

func (f *FileXPathMatcher) matchContentByJsonQuery(content string) ([]string, error) {
	doc, err := jsonquery.Parse(strings.NewReader(content))
	if err != nil {
		return nil, err
	}
	nodes := jsonquery.Find(doc, f.Expr)
	var result []string
	for _, node := range nodes {
		switch ret := node.Value().(type) {
		case []string:
			result = append(result, ret...)
		case map[string]interface{}:
			for _, v := range ret {
				result = append(result, codec.AnyToString(v))
			}
		case []interface{}:
			for _, m := range ret {
				result = append(result, codec.AnyToString(m))
			}
		case []map[string]interface{}:
			for _, m := range ret {
				for _, v := range m {
					result = append(result, codec.AnyToString(v))
				}
			}
		default:
			result = append(result, codec.AnyToString(ret))
		}
	}
	return result, nil
}

func checkFileContentType(content []byte) FileFilterXpathKind {
	if isJSON(content) {
		return FileFilterXPathJson
	} else if isXML(content) {
		return FileFilterXPathXML
	} else if isYAML(content) {
		return FileFilterXPathYaml
	}
	return FileFilterXPathUnValid
}

func isJSON(data []byte) bool {
	var js map[string]interface{}
	return json.Unmarshal(data, &js) == nil
}

func isXML(data []byte) bool {
	var x interface{}
	return xml.Unmarshal(data, &x) == nil
}

func isYAML(data []byte) bool {
	var y any
	return yaml.Unmarshal(data, &y) == nil
}
