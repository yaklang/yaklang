// Command rulemode compiles every SyntaxFlow rule and classifies the opcodes
// it actually emits. It does not trust a textual search for "->".
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	_ "github.com/mattn/go-sqlite3"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

const (
	classPure    = "pure-pattern"
	classFlow    = "dataflow"
	classMixed   = "mixed"
	classSSAOnly = "ssa-search"
	classFail    = "compile-failed"
)

func main() {
	dbPath := flag.String("db", "", "syntax flow rule sqlite database")
	outPath := flag.String("out", "", "classification json")
	flag.Parse()
	if *dbPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "need -db and -out")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite3", *dbPath+"?mode=ro")
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT id, IFNULL(rule_name,''), IFNULL(content,''), IFNULL(mode,'')
		FROM syntax_flow_rules WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		fatal(err)
	}
	defer rows.Close()

	var items []item
	counts := map[string]int{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.RuleName, &it.Content, &it.Mode); err != nil {
			fatal(err)
		}
		it.Class, it.Opcodes, it.Natives, it.Err = classify(it.Content)
		counts[it.Class]++
		counts["mode:"+it.Mode+"/"+it.Class]++
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		fatal(err)
	}
	raw, err := json.MarshalIndent(struct {
		Counts []count `json:"counts"`
		Rules  []item  `json:"rules"`
	}{flatten(counts), items}, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*outPath, raw, 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("rules=%d\n", len(items))
	for _, c := range flatten(counts) {
		fmt.Printf("%s %d\n", c.Key, c.N)
	}
}

type item struct {
	ID       int64    `json:"id"`
	RuleName string   `json:"rule_name"`
	Mode     string   `json:"mode"`
	Class    string   `json:"class"`
	Opcodes  []string `json:"opcodes,omitempty"`
	Natives  []string `json:"natives,omitempty"`
	Err      string   `json:"error,omitempty"`
	Content  string   `json:"-"`
}

type count struct {
	Key string `json:"key"`
	N   int    `json:"n"`
}

func flatten(m map[string]int) []count {
	out := make([]count, 0, len(m))
	for k, n := range m {
		out = append(out, count{k, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func classify(content string) (class string, opcodes, natives []string, errText string) {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(content)
	if err != nil || frame == nil {
		if err == nil {
			err = fmt.Errorf("nil frame")
		}
		return classFail, nil, nil, err.Error()
	}
	var dataflow, member, ssaSearch, pattern bool
	opset := map[string]struct{}{}
	nset := map[string]struct{}{}
	for _, code := range frame.Codes {
		if code == nil {
			continue
		}
		name := sfvm.Opcode2String[code.OpCode]
		switch code.OpCode {
		case sfvm.OpGetUsers, sfvm.OpGetBottomUsers, sfvm.OpGetDefs, sfvm.OpGetTopDefs,
			sfvm.OpGetCall, sfvm.OpGetCallArgs, sfvm.OpCompareOpcode:
			dataflow = true
			opset[name] = struct{}{}
		case sfvm.OpRecursiveSearchExact, sfvm.OpRecursiveSearchGlob, sfvm.OpRecursiveSearchRegexp, sfvm.OpListIndex:
			member = true
			opset[name] = struct{}{}
		case sfvm.OpPushSearchExact, sfvm.OpPushSearchGlob, sfvm.OpPushSearchRegexp:
			if code.UnaryInt&int(ssadb.KeyMatch) != 0 {
				member = true
			} else {
				ssaSearch = true
			}
			opset[name] = struct{}{}
		case sfvm.OpFileFilterReg, sfvm.OpFileFilterXpath, sfvm.OpFileFilterJsonPath, sfvm.OpReMatch, sfvm.OpGlobMatch:
			pattern = true
			opset[name] = struct{}{}
		case sfvm.OpNativeCall:
			nset[code.UnaryStr] = struct{}{}
			if patternNative(code.UnaryStr) {
				pattern = true
			} else if flowNative(code.UnaryStr) {
				dataflow = true
			} else {
				// Unknown native calls can hide value-graph work. Keep them out of pure-pattern.
				ssaSearch = true
			}
			opset["OpNativeCall:"+code.UnaryStr] = struct{}{}
		}
	}
	opcodes = keys(opset)
	natives = keys(nset)
	graph := dataflow || member || ssaSearch
	switch {
	case graph && pattern:
		return classMixed, opcodes, natives, ""
	case dataflow || member:
		return classFlow, opcodes, natives, ""
	case pattern:
		return classPure, opcodes, natives, ""
	case ssaSearch:
		return classSSAOnly, opcodes, natives, ""
	default:
		// Description-only or empty match. Not a source-scan candidate.
		return classSSAOnly, opcodes, natives, ""
	}
}

func patternNative(name string) bool {
	switch name {
	case "regexp", "re", "pattern_regex", "pattern-regex", "pattern_regex_not", "pattern_not_regex":
		return true
	default:
		return false
	}
}

func flowNative(name string) bool {
	switch name {
	case "dataflow", "getCall", "getCallee", "getCaller", "getReturns", "getFormalParams",
		"getFunc", "getObject", "getMembers", "getMember", "searchFunc", "source", "sink":
		return true
	default:
		return false
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
