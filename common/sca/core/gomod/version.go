// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license in LICENSE.
// Selected read-only canonicalization and path-major functions from x/mod
// v0.19.0 semver/semver.go and module/module.go. Comparison, sorting, download,
// escaping and module-cache operations are not included.
package gomod

import "strings"

type parsed struct {
	major      string
	minor      string
	patch      string
	short      string
	prerelease string
	build      string
}

func Canonical(v string) string {
	p, ok := parse(v)
	if !ok {
		return ""
	}
	if p.build != "" {
		return v[:len(v)-len(p.build)]
	}
	if p.short != "" {
		return v + p.short
	}
	return v
}

func parse(v string) (p parsed, ok bool) {
	if v == "" || v[0] != 'v' {
		return
	}
	p.major, v, ok = parseInt(v[1:])
	if !ok {
		return
	}
	if v == "" {
		p.minor = "0"
		p.patch = "0"
		p.short = ".0.0"
		return
	}
	if v[0] != '.' {
		ok = false
		return
	}
	p.minor, v, ok = parseInt(v[1:])
	if !ok {
		return
	}
	if v == "" {
		p.patch = "0"
		p.short = ".0"
		return
	}
	if v[0] != '.' {
		ok = false
		return
	}
	p.patch, v, ok = parseInt(v[1:])
	if !ok {
		return
	}
	if len(v) > 0 && v[0] == '-' {
		p.prerelease, v, ok = parsePrerelease(v)
		if !ok {
			return
		}
	}
	if len(v) > 0 && v[0] == '+' {
		p.build, v, ok = parseBuild(v)
		if !ok {
			return
		}
	}
	if v != "" {
		ok = false
		return
	}
	ok = true
	return
}

func parseInt(v string) (t, rest string, ok bool) {
	if v == "" {
		return
	}
	if v[0] < '0' || '9' < v[0] {
		return
	}
	i := 1
	for i < len(v) && '0' <= v[i] && v[i] <= '9' {
		i++
	}
	if v[0] == '0' && i != 1 {
		return
	}
	return v[:i], v[i:], true
}

func parsePrerelease(v string) (t, rest string, ok bool) {
	// "A pre-release version MAY be denoted by appending a hyphen and
	// a series of dot separated identifiers immediately following the patch version.
	// Identifiers MUST comprise only ASCII alphanumerics and hyphen [0-9A-Za-z-].
	// Identifiers MUST NOT be empty. Numeric identifiers MUST NOT include leading zeroes."
	if v == "" || v[0] != '-' {
		return
	}
	i := 1
	start := 1
	for i < len(v) && v[i] != '+' {
		if !isIdentChar(v[i]) && v[i] != '.' {
			return
		}
		if v[i] == '.' {
			if start == i || isBadNum(v[start:i]) {
				return
			}
			start = i + 1
		}
		i++
	}
	if start == i || isBadNum(v[start:i]) {
		return
	}
	return v[:i], v[i:], true
}

func parseBuild(v string) (t, rest string, ok bool) {
	if v == "" || v[0] != '+' {
		return
	}
	i := 1
	start := 1
	for i < len(v) {
		if !isIdentChar(v[i]) && v[i] != '.' {
			return
		}
		if v[i] == '.' {
			if start == i {
				return
			}
			start = i + 1
		}
		i++
	}
	if start == i {
		return
	}
	return v[:i], v[i:], true
}

func isIdentChar(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-'
}

func isBadNum(v string) bool {
	i := 0
	for i < len(v) && '0' <= v[i] && v[i] <= '9' {
		i++
	}
	return i == len(v) && i > 1 && v[0] == '0'
}

func SplitPathVersion(path string) (prefix, pathMajor string, ok bool) {
	if strings.HasPrefix(path, "gopkg.in/") {
		return splitGopkgIn(path)
	}

	i := len(path)
	dot := false
	for i > 0 && ('0' <= path[i-1] && path[i-1] <= '9' || path[i-1] == '.') {
		if path[i-1] == '.' {
			dot = true
		}
		i--
	}
	if i <= 1 || i == len(path) || path[i-1] != 'v' || path[i-2] != '/' {
		return path, "", true
	}
	prefix, pathMajor = path[:i-2], path[i-2:]
	if dot || len(pathMajor) <= 2 || pathMajor[2] == '0' || pathMajor == "/v1" {
		return path, "", false
	}
	return prefix, pathMajor, true
}

// splitGopkgIn is like SplitPathVersion but only for gopkg.in paths.
func splitGopkgIn(path string) (prefix, pathMajor string, ok bool) {
	if !strings.HasPrefix(path, "gopkg.in/") {
		return path, "", false
	}
	i := len(path)
	if strings.HasSuffix(path, "-unstable") {
		i -= len("-unstable")
	}
	for i > 0 && ('0' <= path[i-1] && path[i-1] <= '9') {
		i--
	}
	if i <= 1 || path[i-1] != 'v' || path[i-2] != '.' {
		// All gopkg.in paths must end in vN for some N.
		return path, "", false
	}
	prefix, pathMajor = path[:i-2], path[i-2:]
	if len(pathMajor) <= 2 || pathMajor[2] == '0' && pathMajor != ".v0" {
		return path, "", false
	}
	return prefix, pathMajor, true
}

func canonicalVersion(v string) string {
	c := Canonical(v)
	if c != "" && strings.HasSuffix(v, "+incompatible") {
		c += "+incompatible"
	}
	return c
}
func matchingMajor(path, v string) bool {
	_, major, ok := SplitPathVersion(path)
	if !ok {
		return false
	}
	p, valid := parse(v)
	if !valid {
		return false
	}
	if strings.HasPrefix(major, ".v") {
		major = strings.TrimSuffix(major, "-unstable")
	}
	if strings.HasPrefix(v, "v0.0.0-") && major == ".v1" {
		return true
	}
	if major == "" {
		return p.major == "0" || p.major == "1" || p.build == "+incompatible"
	}
	return major[1:] == "v"+p.major
}
