// Package gomod reads explicitly supplied Go module declarations. It does not
// select a build list, read go.sum, resolve paths, or download modules.
package gomod

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrLimit = errors.New("go.mod resource limit exceeded")

type Limits struct{ MaxBytes, MaxTokenBytes, MaxStatements int }

func (l Limits) defaults() (Limits, error) {
	if l.MaxBytes < 0 || l.MaxTokenBytes < 0 || l.MaxStatements < 0 {
		return l, errors.New("negative go.mod limit")
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = 16 << 20
	}
	if l.MaxTokenBytes == 0 {
		l.MaxTokenBytes = 64 << 10
	}
	if l.MaxStatements == 0 {
		l.MaxStatements = 200000
	}
	return l, nil
}

type Module struct{ Path, Version string }
type Requirement struct {
	Path, Version string
	Indirect      bool
	Line          int
}
type Replacement struct {
	Old, New Module
	Line     int
}
type Interval struct {
	Low, High string
	Line      int
}
type File struct {
	Module, Go, Toolchain string
	Require               []Requirement
	Replace               []Replacement
	Exclude               []Module
	Retract               []Interval
	Godebug               []string
}

var goRE = regexp.MustCompile(`^[1-9][0-9]*\.(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))?([a-z]+[0-9]+)?$`)

func version(v string) bool { return canonicalVersion(v) != "" }
func modulePath(s string) bool {
	return s != "" && !strings.ContainsAny(s, "\x00\r\n\t \\") && !strings.HasPrefix(s, "/") && !strings.Contains(s, "//")
}

// toolchainName is the read-only x/mod ToolchainRE: default or go1 / go1.*
func toolchainName(s string) bool {
	return s == "default" || strings.HasPrefix(s, "go1") && (len(s) == 3 || s[3] == '.')
}

// Parse retains declarations and replacement sources separately. Returned
// versions are declarations, not evidence that a module is installed.
func Parse(ctx context.Context, data []byte, limits Limits) (result *File, err error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	limits, err = limits.defaults()
	if err != nil {
		return nil, err
	}
	if len(data) > limits.MaxBytes {
		return nil, ErrLimit
	}
	if !utf8.Valid(data) {
		return nil, errors.New("invalid UTF-8 in go.mod")
	}
	if err := budget.From(ctx).Working(int64(len(data))); err != nil {
		return nil, err
	}
	in := &input{complete: data, remaining: data, ctx: ctx, limits: limits, pos: Position{Line: 1, LineRune: 1}}
	defer func() {
		if p := recover(); p != nil {
			if f, ok := p.(parseFailure); ok {
				result = nil
				err = f.err
			} else {
				panic(p)
			}
		}
	}()
	f := new(File)
	block := ""
	count := 0
	required := map[string]bool{}
	replaced := map[Module]bool{}
	for {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		var ts []token
		comment := ""
		ended := false
		for {
			in.readToken()
			t := in.token
			if t.kind == _COMMENT {
				break
			}
			if t.kind == _EOLCOMMENT {
				comment = t.text
				break
			}
			if t.kind == '\n' {
				break
			}
			if t.kind == _EOF {
				ended = true
				break
			}
			if len(ts) >= 16 {
				return nil, fmt.Errorf("go.mod:%d: too many tokens", t.pos.Line)
			}
			ts = append(ts, t)
		}
		if len(ts) > 0 {
			if len(ts) == 1 && ts[0].text == ")" {
				if block == "" {
					return nil, errors.New("unexpected closing block")
				}
				block = ""
			} else {
				verb := block
				line := ts[0].pos.Line
				if verb == "" {
					verb = ts[0].text
					ts = ts[1:]
				}
				if len(ts) == 1 && ts[0].text == "(" {
					if block != "" || (verb != "require" && verb != "replace" && verb != "exclude" && verb != "retract" && verb != "godebug") {
						return nil, fmt.Errorf("go.mod:%d: invalid block %s", line, verb)
					}
					block = verb
				} else {
					count++
					if count > limits.MaxStatements {
						return nil, ErrLimit
					}
					args := make([]string, len(ts))
					for i, t := range ts {
						args[i] = t.text
						if t.kind == _STRING {
							v, e := strconv.Unquote(t.text)
							if e != nil {
								return nil, fmt.Errorf("go.mod:%d: %w", line, e)
							}
							args[i] = v
						}
					}
					if e := f.add(ctx, verb, args, line, comment, required, replaced); e != nil {
						return nil, e
					}
				}
			}
		}
		if ended {
			break
		}
	}
	if block != "" {
		return nil, errors.New("unterminated go.mod block")
	}
	return f, nil
}
func (f *File) add(ctx context.Context, verb string, a []string, line int, comment string, required map[string]bool, replaced map[Module]bool) error {
	bad := func() error { return fmt.Errorf("go.mod:%d: invalid or duplicate %s directive", line, verb) }
	pair := func(a []string) (Module, bool) {
		if len(a) != 2 || !modulePath(a[0]) || !version(a[1]) {
			return Module{}, false
		}
		v := canonicalVersion(a[1])
		if !matchingMajor(a[0], v) {
			return Module{}, false
		}
		return Module{a[0], v}, true
	}
	switch verb {
	case "module":
		if len(a) != 1 || f.Module != "" || !modulePath(a[0]) {
			return bad()
		}
		f.Module = a[0]
	case "go":
		if len(a) != 1 || f.Go != "" || !goRE.MatchString(a[0]) {
			return bad()
		}
		f.Go = a[0]
	case "toolchain":
		if len(a) != 1 || f.Toolchain != "" || !toolchainName(a[0]) {
			return bad()
		}
		f.Toolchain = a[0]
	case "godebug":
		if len(a) != 1 || strings.Count(a[0], "=") != 1 || strings.HasPrefix(a[0], "=") || strings.HasSuffix(a[0], "=") {
			return bad()
		}
		if err := budget.From(ctx).Result(budget.SizeOfString(a[0])); err != nil {
			return err
		}
		f.Godebug = append(f.Godebug, a[0])
	case "require":
		m, ok := pair(a)
		if !ok || required[m.Path] {
			return bad()
		}
		required[m.Path] = true
		c := strings.Fields(strings.TrimPrefix(comment, "//"))
		indirect := len(c) > 0 && (c[0] == "indirect" || c[0] == "indirect;")
		if err := budget.From(ctx).Result(budget.SizeOfRecord() + budget.SizeOfString(m.Path) + budget.SizeOfString(m.Version)); err != nil {
			return err
		}
		f.Require = append(f.Require, Requirement{m.Path, m.Version, indirect, line})
	case "exclude":
		m, ok := pair(a)
		if !ok {
			return bad()
		}
		if err := budget.From(ctx).Result(budget.SizeOfRecord() + budget.SizeOfString(m.Path) + budget.SizeOfString(m.Version)); err != nil {
			return err
		}
		f.Exclude = append(f.Exclude, m)
	case "replace":
		split := -1
		for i, x := range a {
			if x == "=>" {
				if split != -1 {
					return bad()
				}
				split = i
			}
		}
		if split < 1 || split > 2 || len(a)-split < 2 || len(a)-split > 3 {
			return bad()
		}
		old := Module{Path: a[0]}
		if !modulePath(old.Path) {
			return bad()
		}
		if split == 2 {
			old.Version = canonicalVersion(a[1])
			if !version(old.Version) || !matchingMajor(old.Path, old.Version) {
				return bad()
			}
		}
		next := Module{Path: a[split+1]}
		if len(a)-split == 3 {
			next.Version = canonicalVersion(a[split+2])
			if !modulePath(next.Path) || !version(next.Version) || !matchingMajor(next.Path, next.Version) {
				return bad()
			}
		} else if !(strings.HasPrefix(next.Path, "./") || strings.HasPrefix(next.Path, "../") || strings.HasPrefix(next.Path, "/") || strings.Contains(next.Path, ":") || strings.HasPrefix(next.Path, `\`)) {
			return bad()
		}
		if replaced[old] {
			return bad()
		}
		replaced[old] = true
		if err := budget.From(ctx).Result(budget.SizeOfRecord() + budget.SizeOfString(old.Path) + budget.SizeOfString(next.Path)); err != nil {
			return err
		}
		f.Replace = append(f.Replace, Replacement{old, next, line})
	case "retract":
		if len(a) == 1 && version(a[0]) {
			if err := budget.From(ctx).Result(budget.SizeOfRecord() + budget.SizeOfString(a[0])); err != nil {
				return err
			}
			f.Retract = append(f.Retract, Interval{a[0], a[0], line})
		} else if len(a) == 5 && a[0] == "[" && a[2] == "," && a[4] == "]" && version(a[1]) && version(a[3]) {
			if err := budget.From(ctx).Result(budget.SizeOfRecord() + budget.SizeOfString(a[1]) + budget.SizeOfString(a[3])); err != nil {
				return err
			}
			f.Retract = append(f.Retract, Interval{a[1], a[3], line})
		} else {
			return bad()
		}
	default:
		return fmt.Errorf("go.mod:%d: unsupported_syntax: %s", line, verb)
	}
	return nil
}
