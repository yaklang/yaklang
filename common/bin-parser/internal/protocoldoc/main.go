// Command protocoldoc refreshes the inventory in PROTOCOL_TODO.md from Go data.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
)

type item map[string]string

func inventory(name, variable string) ([]item, error) {
	f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
	if err != nil {
		return nil, err
	}
	constants := make(map[string]string)
	ast.Inspect(f, func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if ok {
			for i, value := range v.Values {
				if literal, ok := value.(*ast.BasicLit); ok && literal.Kind == token.STRING && i < len(v.Names) {
					constants[v.Names[i].Name], _ = strconv.Unquote(literal.Value)
				}
			}
		}
		return true
	})
	var result []item
	ast.Inspect(f, func(n ast.Node) bool {
		v, ok := n.(*ast.ValueSpec)
		if !ok || len(v.Names) != 1 || v.Names[0].Name != variable {
			return true
		}
		list, ok := v.Values[0].(*ast.CompositeLit)
		if !ok {
			return false
		}
		for _, value := range list.Elts {
			record, ok := value.(*ast.CompositeLit)
			if !ok {
				continue
			}
			row := make(item)
			for _, field := range record.Elts {
				kv, ok := field.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				switch value := kv.Value.(type) {
				case *ast.BasicLit:
					if value.Kind == token.STRING {
						row[key.Name], _ = strconv.Unquote(value.Value)
					}
				case *ast.Ident:
					row[key.Name] = constants[value.Name]
				}
			}
			result = append(result, row)
		}
		return false
	})
	if len(result) == 0 {
		return nil, fmt.Errorf("no %s in %s", variable, name)
	}
	return result, nil
}
func cell(s string) string { return strings.NewReplacer("|", "\\|", "\n", " ", "\r", "").Replace(s) }
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	roadmap, err := inventory("protocol_roadmap.go", "ProtocolRoadmap")
	if err != nil {
		return err
	}
	catalog, err := inventory("protocol_catalog.go", "ProtocolCatalog")
	if err != nil {
		return err
	}
	counts := make(map[string]int)
	catalogCounts := make(map[string]int)
	byName := make(map[string]item)
	for _, r := range roadmap {
		counts[r["Status"]]++
	}
	for _, r := range catalog {
		catalogCounts[r["Status"]]++
		byName[r["Name"]] = r
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, "\n路线图共 **%d** 项：限定范围 `done` **%d**，`partial` **%d**，`todo` **%d**。目录共 **%d** 个入口：`stable` **%d**，`partial` **%d**，`new` **%d**。入口、规则文件和协议家族不是同一个计数。\n\n", len(roadmap), counts["done"], counts["partial"], counts["todo"], len(catalog), catalogCounts["stable"], catalogCounts["partial"], catalogCounts["new"])
	out.WriteString("### 路线图尚未完成的项目\n\n同名目录只作定位；没有同名不等于没有实现，别名与专用 Fields 入口需继续核对下表及 `protocol_catalog.go`。`done` 项的剩余范围也在后面的非 stable 目录中保留。\n\n| 优先级 | 协议 | 路线图状态 | 同名目录入口 |\n|---|---|---|---|\n")
	sort.Slice(roadmap, func(i, j int) bool {
		if roadmap[i]["Priority"] != roadmap[j]["Priority"] {
			return roadmap[i]["Priority"] < roadmap[j]["Priority"]
		}
		return roadmap[i]["Name"] < roadmap[j]["Name"]
	})
	for _, r := range roadmap {
		if r["Status"] == "done" {
			continue
		}
		link := "需核对别名/Fields 入口"
		if c := byName[r["Name"]]; c != nil {
			link = fmt.Sprintf("[%s / %s](rules/%s)", cell(c["Status"]), cell(c["EntryNode"]), c["RuleFile"])
		}
		fmt.Fprintf(&out, "| %s | %s | %s | %s |\n", r["Priority"], cell(r["Name"]), r["Status"], link)
	}
	out.WriteString("\n### 已有模型中仍非 stable 的入口\n\n以下每行按规则文件合并入口，全部保持现有 `partial` / `new` 状态，不将有限样本支持升级为完整协议。每个入口的具体未解析字段、上下文和样本依据见 `protocol_catalog.go` 的 `Notes` / `SampleFrom`。这些都是后续协议扩展的起点。\n\n| 规则 | 状态与入口 |\n|---|---|\n")
	byRule := make(map[string][]string)
	for _, c := range catalog {
		if c["Status"] == "stable" {
			continue
		}
		byRule[c["RuleFile"]] = append(byRule[c["RuleFile"]], cell(c["Name"])+" (`"+cell(c["EntryNode"])+"`, "+c["Status"]+")")
	}
	var names []string
	for name := range byRule {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sort.Strings(byRule[name])
		fmt.Fprintf(&out, "| [%s](rules/%s) | %s |\n", name, name, strings.Join(byRule[name], "<br>"))
	}
	const start = "<!-- BEGIN GENERATED PROTOCOL INVENTORY -->"
	const end = "<!-- END GENERATED PROTOCOL INVENTORY -->"
	doc, err := os.ReadFile("PROTOCOL_TODO.md")
	if err != nil {
		return err
	}
	a := bytes.Index(doc, []byte(start))
	b := bytes.Index(doc, []byte(end))
	if a < 0 || b <= a {
		return fmt.Errorf("missing protocol inventory markers")
	}
	updated := append([]byte(nil), doc[:a+len(start)]...)
	updated = append(updated, '\n')
	updated = append(updated, out.Bytes()...)
	updated = append(updated, '\n')
	updated = append(updated, doc[b:]...)
	return os.WriteFile("PROTOCOL_TODO.md", updated, 0644)
}
