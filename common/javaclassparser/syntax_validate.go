package javaclassparser

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/java/javasyntax"
)

// EnableDecompileSyntaxValidation controls the post-decompile syntax safety net. When enabled
// (default), the fully assembled class is parsed with the same grammar + normalization the SSA
// frontend/jdsc use; if it is not valid Java the offending members are degraded (method bodies
// stubbed, then dropped; field initializers neutralized, then dropped) until the class parses.
// This is what lets Decompile guarantee it never emits a class that fails to parse, even when an
// upstream rewriter produces subtly malformed output without returning an error.
//
// It can be turned off by callers that do their own validation (or want raw output) to avoid the
// extra parse per class.
var EnableDecompileSyntaxValidation = true

// validateJavaSyntax reports whether a full compilation unit is syntactically valid Java
// (after decompiler normalization). nil means the grammar accepts it.
func validateJavaSyntax(src string) error {
	return javasyntax.Validate(src)
}

// validateMemberInHeader reports whether a single member (method or field) is syntactically
// valid in the context of its real class header. Using the real header (e.g. "public interface
// Foo extends Bar") is essential for accuracy: an interface rejects `static {}` initializers and
// a constructor body only parses when the enclosing type name matches, so a generic `class X`
// wrapper would give wrong answers.
func validateMemberInHeader(header, memberCode string) error {
	return javasyntax.Validate(header + " {\n" + memberCode + "\n}")
}

// degradeInvalidMethods returns methods whose generated source is valid Java in the class
// header's context. A method that does not parse is first replaced by a throwing stub; if even
// the stub is un-parseable (e.g. an un-representable signature such as a method literally named
// "$", which the grammar rejects), the method is dropped entirely so the class stays valid.
func (c *ClassObjectDumper) degradeInvalidMethods(header string, methods []*dumpedMethods) []*dumpedMethods {
	out := make([]*dumpedMethods, 0, len(methods))
	for _, m := range methods {
		if m == nil {
			continue
		}
		if validateMemberInHeader(header, m.code) == nil {
			out = append(out, m)
			continue
		}
		// Try degrading to a stub (only possible when we kept the member metadata).
		if m.bodyCode != "stub" && m.member != nil {
			if stub := c.dumpStubMethod(m.member, m.methodName, m.descriptor, "post-decompile syntax validation failed"); stub != nil {
				if validateMemberInHeader(header, stub.code) == nil {
					traitId := fmt.Sprintf("name:%s,desc:%s", m.methodName, m.descriptor)
					c.dumpedMethodsSet[traitId] = stub
					out = append(out, stub)
					log.Warnf("decompiled method %s%s produced invalid Java, replaced with stub", m.methodName, m.descriptor)
					continue
				}
			}
		}
		// Even a stub will not parse (signature itself is un-representable); drop the method.
		log.Warnf("decompiled method %s%s is un-representable as valid Java, dropping it", m.methodName, m.descriptor)
	}
	return out
}

// degradeInvalidFields returns fields whose generated source is valid Java in the header's
// context. A field that does not parse (e.g. an initializer that leaked an internal placeholder)
// is first reduced to a bare declaration without initializer; if that still does not parse the
// field is dropped. Enum constants are left untouched: they are rendered specially by the caller
// (as `A, B, C;`) rather than via field.code, so they are covered by the whole-class fast path.
func (c *ClassObjectDumper) degradeInvalidFields(header, className string, isEnum bool, fields []dumpedFields) []dumpedFields {
	out := make([]dumpedFields, 0, len(fields))
	for _, f := range fields {
		if isEnum && f.typeName == className && (f.modifier == "public static final enum" || f.modifier == "public static final") {
			out = append(out, f)
			continue
		}
		if validateMemberInHeader(header, f.code) == nil {
			out = append(out, f)
			continue
		}
		// Reduce to a bare declaration (strip any initializer / malformed tail).
		bare := strings.TrimSpace(strings.Join([]string{f.modifier, f.typeName, f.fieldName}, " ")) + ";"
		if validateMemberInHeader(header, bare) == nil {
			f.code = bare
			out = append(out, f)
			log.Warnf("decompiled field %s produced invalid Java, reduced to bare declaration", f.fieldName)
			continue
		}
		log.Warnf("decompiled field %s is un-representable as valid Java, dropping it", f.fieldName)
	}
	return out
}
