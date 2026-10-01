package mutate

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/fuzztagx"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/utils"
)

var httpTemplateTagName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_:-]*$`)

// RenderHTTPFields expands related URL-mode fields together, so ::labels pair
// across path/query/form/body/header values. Callers encode structured query and
// form values after rendering, exactly once. An opaque separator retains field
// boundaries without JSON-escaping template arguments or binary payloads.
func RenderHTTPFields(fields []string, variables map[string]interface{}, limit int, ctx context.Context) ([][]string, error) {
	separator := "\x00" + utils.RandStringBytes(32) + "\x00"
	rows, err := RenderHTTPTemplate(strings.Join(fields, separator), variables, limit, ctx)
	if err != nil {
		return nil, err
	}
	result := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := strings.Split(row, separator)
		if len(values) != len(fields) {
			return nil, fmt.Errorf("template changed HTTP field boundaries")
		}
		result = append(result, values)
	}
	return result, nil
}

// RenderHTTPTemplate renders request data with the native FuzzTag engine. Unlike
// permissive fuzzing, request construction rejects unknown/malformed tags and
// missing parameters, and refuses an oversized expansion instead of sending a
// silently truncated test. Variable values are never parsed as new templates.
// Bare {{name}} variables are retained for the batch HTTP tool's existing calls.
func RenderHTTPTemplate(input string, variables map[string]interface{}, limit int, ctx context.Context) ([]string, error) {
	if limit < 1 || limit > 500 {
		return nil, fmt.Errorf("HTTP template limit must be between 1 and 500")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	table := make(map[string]*parser.TagMethod, len(tagMethodMap)+3)
	for name, method := range tagMethodMap {
		table[name] = method
	}
	params := &parser.TagMethod{Name: "params", Fun: func(key string) ([]*parser.FuzzResult, error) {
		value, ok := variables[key]
		if !ok {
			return nil, fmt.Errorf("unknown template variable %q", key)
		}
		var rows []*parser.FuzzResult
		for _, s := range utils.InterfaceToStringSlice(value) {
			rows = append(rows, parser.NewFuzzResultWithData(s))
		}
		return rows, nil
	}}
	for _, name := range []string{"params", "param", "p"} {
		table[name] = params
	}
	nodes, err := fuzztagx.ParseFuzztag(input, false)
	if err != nil {
		return nil, err
	}
	var validate func([]parser.Node) error
	validate = func(nodes []parser.Node) error {
		for _, node := range nodes {
			switch tag := node.(type) {
			case parser.StringNode:
				if strings.Contains(string(tag), "{{") {
					return fmt.Errorf("malformed {{...}} template tag")
				}
			case *fuzztagx.RawTag:
				// raw contents are literal, including tag-like text.
			case *fuzztagx.FuzzTag:
				if err := validate(tag.Data); err != nil {
					return err
				}
				var call strings.Builder
				for _, child := range tag.Data {
					if s, ok := child.(parser.StringNode); ok {
						call.WriteString(string(s))
					} else {
						call.WriteString("value")
					}
				}
				content := call.String()
				name := content
				if i := strings.IndexByte(content, '('); i >= 0 {
					if !strings.HasSuffix(strings.TrimSpace(content), ")") {
						return fmt.Errorf("malformed template tag %s", tag.String())
					}
					name = strings.TrimSpace(content[:i])
				} else if _, ok := variables[content]; ok && httpTemplateTagName.MatchString(content) {
					tag.Data = []parser.Node{parser.StringNode("params(" + content + ")")}
					name = "params"
				}
				if !httpTemplateTagName.MatchString(name) {
					return fmt.Errorf("malformed template tag %s", tag.String())
				}
				name = strings.SplitN(name, "::", 2)[0]
				if table[name] == nil {
					return fmt.Errorf("unknown template tag %q", name)
				}
			}
		}
		return nil
	}
	if err := validate(nodes); err != nil {
		return nil, err
	}
	generator := parser.NewGenerator(ctx, nodes, table)
	defer generator.Cancel()
	generator.GenerateConfig.AssertError = true
	rows := make([]string, 0)
	for generator.Next() {
		if len(rows) == limit {
			return nil, fmt.Errorf("template expansion exceeds request limit %d", limit)
		}
		rows = append(rows, string(generator.Result().GetData()))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if generator.Error != nil {
		return nil, generator.Error
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("template produced no requests")
	}
	return rows, nil
}
