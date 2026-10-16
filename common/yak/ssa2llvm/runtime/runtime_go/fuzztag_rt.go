package main

// Inline fuzztag expansion for x`...` templates.
//
// The full mutate.FuzzTagExec engine drags common/consts (and its database
// schema init) into the AOT runtime's init graph; elfsplit's section pruning
// then keeps consts.init alive for scripts that reference any runtime_go
// symbol in the same section, and consts.init crashes when its schema patch
// table was pruned. So the AOT runtime implements the tags the mustpass suite
// actually needs (trim/substr/gb18030/gb18030toUTF8/hexd/crlf/list/int,
// including nested {{...}} arguments) without importing common/mutate.

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// runtimeFuzztagExpand expands a yak fuzztag template into its value list.
// Plain text passes through unchanged; {{tag(args)}} placeholders are
// replaced by their expansion (nested placeholders resolve inside-out).
func runtimeFuzztagExpand(input string) []string {
	var results []string
	rest := input
	for {
		idx := strings.Index(rest, "{{")
		if idx < 0 {
			if rest != "" {
				results = append(results, rest)
			}
			break
		}
		if idx > 0 {
			results = append(results, rest[:idx])
		}
		rest = rest[idx+2:]
		end := runtimeFuzztagCloseIndex(rest)
		if end < 0 {
			results = append(results, rest)
			break
		}
		body := rest[:end]
		rest = rest[end+2:]
		expanded := runtimeFuzztagExpandTag(body)
		results = append(results, expanded...)
	}
	return results
}

// runtimeFuzztagCloseIndex finds the matching "}}" for a "{{" at the start
// of s, counting nested "{{" so inner placeholders do not truncate the body
// (e.g. {{gb18030toUTF8({{hexd(c4e3bac3)}})}}).
func runtimeFuzztagCloseIndex(s string) int {
	depth := 1
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '{' && s[i+1] == '{' {
			depth++
			i++
			continue
		}
		if s[i] == '}' && s[i+1] == '}' {
			depth--
			if depth == 0 {
				return i
			}
			i++
		}
	}
	return -1
}

func runtimeFuzztagExpandTag(body string) []string {
	name := body
	args := ""
	if idx := strings.Index(body, "("); idx >= 0 && strings.HasSuffix(body, ")") {
		name = body[:idx]
		args = body[idx+1 : len(body)-1]
	}
	// Nested placeholders in the argument expand into a cartesian product:
	// surrounding text combines with each expanded value, and the outer tag
	// applies to every combination ({{trim(\n  {{int(1-5)}}\n)}} -> trim of
	// "1", "2", ... with the whitespace, [0] == "1").
	if strings.Contains(args, "{{") {
		combos := runtimeFuzztagExpandArgCombos(args)
		var out []string
		for _, c := range combos {
			out = append(out, runtimeFuzztagExpandTag(name+"("+c+")")...)
		}
		return out
	}
	switch name {
	case "crlf":
		return []string{"\r\n"}
	case "null":
		return []string{""}
	case "trim":
		return []string{strings.TrimSpace(args)}
	case "substr":
		// substr(s|start,length): split on the LAST '|', then start,length
		// are comma-separated.
		index := strings.LastIndexByte(args, '|')
		if index < 0 {
			return []string{args}
		}
		before, after := args[:index], args[index+1:]
		bounds := strings.SplitN(after, ",", 2)
		if len(bounds) != 2 {
			return []string{args}
		}
		start, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
		length, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
		if err1 != nil || err2 != nil || start < 0 {
			return []string{args}
		}
		runes := []rune(before)
		if start >= len(runes) {
			return []string{""}
		}
		end := start + length
		if length <= 0 || end > len(runes) {
			end = len(runes)
		}
		return []string{string(runes[start:end])}
	case "hexd":
		s := strings.TrimPrefix(strings.TrimSpace(args), "0x")
		decoded, err := hex.DecodeString(s)
		if err != nil {
			return []string{args}
		}
		return []string{string(decoded)}
	case "gb18030":
		encoded, _, err := transform.String(simplifiedchinese.GB18030.NewEncoder(), args)
		if err != nil {
			return []string{args}
		}
		return []string{encoded}
	case "gb18030toUTF8":
		decoded, _, err := transform.String(simplifiedchinese.GB18030.NewDecoder(), args)
		if err != nil {
			return []string{args}
		}
		return []string{decoded}
	case "int":
		return runtimeFuzztagIntRange(args)
	case "list":
		// array alias: split on "|" into separate values.
		parts := strings.Split(args, "|")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	case "list:comma":
		// array:comma alias: split on "," into separate values.
		parts := strings.Split(args, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	case "list:auto":
		// array:auto alias: split on both "," and "|".
		parts := strings.FieldsFunc(args, func(r rune) bool { return r == ',' || r == '|' })
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	default:
		// Unknown tags keep the original text (like yakvm's error path,
		// which yields the raw string).
		return []string{"{{" + body + "}}"}
	}
}

// runtimeFuzztagExpandArgCombos expands a tag argument containing {{...}}
// placeholders into its cartesian product: literal text is a single-choice
// segment, each placeholder contributes its value list.
func runtimeFuzztagExpandArgCombos(args string) []string {
	var segments [][]string
	rest := args
	for {
		idx := strings.Index(rest, "{{")
		if idx < 0 {
			if rest != "" {
				segments = append(segments, []string{rest})
			}
			break
		}
		if idx > 0 {
			segments = append(segments, []string{rest[:idx]})
		}
		rest = rest[idx+2:]
		end := runtimeFuzztagCloseIndex(rest)
		if end < 0 {
			segments = append(segments, []string{rest})
			break
		}
		body := rest[:end]
		rest = rest[end+2:]
		segments = append(segments, runtimeFuzztagExpandTag(body))
	}
	out := []string{""}
	for _, seg := range segments {
		var next []string
		for _, combo := range out {
			for _, v := range seg {
				next = append(next, combo+v)
			}
		}
		out = next
	}
	return out
}

// runtimeFuzztagIntRange expands "1-10|3|1|4" style numeric ranges into
// their value list (e.g. int(1-10) -> 1..10).
func runtimeFuzztagIntRange(args string) []string {
	first := strings.SplitN(args, "|", 2)[0]
	first = strings.TrimSpace(first)
	if idx := strings.Index(first, "-"); idx > 0 {
		low, err1 := strconv.Atoi(first[:idx])
		high, err2 := strconv.Atoi(first[idx+1:])
		if err1 != nil || err2 != nil {
			return []string{args}
		}
		if high < low {
			low, high = high, low
		}
		out := make([]string, 0, high-low+1)
		for i := low; i <= high; i++ {
			out = append(out, fmt.Sprintf("%d", i))
		}
		return out
	}
	if _, err := strconv.Atoi(first); err == nil {
		return []string{first}
	}
	return []string{args}
}
