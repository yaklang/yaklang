package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/java/javasyntax"
)

// debugInvalidMethods, when set via DEBUG_INVALID, logs the raw (pre-degradation) source of any
// method that fails post-decompile syntax validation, to aid diagnosing the post-syntax bucket.
var debugInvalidMethods = os.Getenv("DEBUG_INVALID") != ""

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

// DecompileSyntaxValidationBudget bounds how long the post-decompile syntax safety net spends
// parsing a single compilation unit (or member) before giving up. The SLL fast path returns in
// milliseconds, but ANTLR's LL fallback can blow up super-linearly on pathological decompiler
// output (e.g. a method carrying dozens of switch statements), which would otherwise make a
// single class take tens of seconds to validate and effectively hang batch scans. When the
// budget is exceeded we conservatively treat the input as invalid so the offending member is
// degraded to a stub: this both keeps decompilation time bounded and preserves the "never emit
// un-parseable Java" guarantee. Set to <= 0 to disable the budget (validate synchronously).
//
// The budget is sized to comfortably clear large-but-valid methods (e.g. a 600+ line SQL parser
// method validates in ~4s via the LL fallback) so the net does not falsely degrade them under load,
// while still bounding genuinely pathological members. Raised from 4s to 8s because borderline
// methods were intermittently timing out on busy machines, turning a valid+deterministic decompile
// into a spurious stub (false positive). 8s only matters for the rare member that exceeds it; valid
// methods return as soon as the parse finishes, well under the cap.
var DecompileSyntaxValidationBudget = 8 * time.Second

// validateJavaSyntax reports whether a full compilation unit is syntactically valid Java
// (after decompiler normalization). nil means the grammar accepts it. The parse runs under
// DecompileSyntaxValidationBudget so a single pathological input cannot stall decompilation.
func validateJavaSyntax(src string) error {
	return validateJavaSyntaxWithBudget(src, DecompileSyntaxValidationBudget)
}

// validateJavaSyntaxWithBudget runs javasyntax.Validate but abandons the parse once budget
// elapses, returning a sentinel error. A budget <= 0 means "no limit" (validate inline). The
// abandoned goroutine still finishes on its own (ANTLR has no cancellation hook), but its result
// is dropped via the buffered channel so nothing blocks or leaks permanently.
func validateJavaSyntaxWithBudget(src string, budget time.Duration) error {
	if budget <= 0 {
		return javasyntax.Validate(src)
	}
	ch := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- utils.Errorf("panic during syntax validation: %v", r)
			}
		}()
		ch <- javasyntax.Validate(src)
	}()
	// Use a stoppable timer rather than time.After so the budget timer (and the
	// src it retains via the closure) is released as soon as validation returns.
	// time.After would keep one ~budget-long timer alive per validation, which on
	// large jars (thousands of classes/members) accumulates thousands of pending
	// timers and goroutines, wasting memory and delaying GC during batch scans.
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case err := <-ch:
		return err
	case <-timer.C:
		return utils.Errorf("syntax validation exceeded budget %s (treated as invalid for safe degradation)", budget)
	}
}

// validateMemberInHeader reports whether a single member (method or field) is syntactically
// valid in the context of its real class header. Using the real header (e.g. "public interface
// Foo extends Bar") is essential for accuracy: an interface rejects `static {}` initializers and
// a constructor body only parses when the enclosing type name matches, so a generic `class X`
// wrapper would give wrong answers.
func validateMemberInHeader(header, memberCode string) error {
	return validateJavaSyntaxWithBudget(header+" {\n"+memberCode+"\n}", DecompileSyntaxValidationBudget)
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
		if debugInvalidMethods {
			log.Errorf("DEBUG_INVALID method %s%s:\n%s", m.methodName, m.descriptor, m.code)
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
