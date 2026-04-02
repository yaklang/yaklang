package antlr4util

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	envAntlrDiagnostic      = "YAK_ANTLR_DIAGNOSTIC"
	envAntlrDiagnosticExact = "YAK_ANTLR_DIAGNOSTIC_EXACT"
	envAntlrDiagnosticLimit = "YAK_ANTLR_DIAGNOSTIC_LIMIT"
)

var (
	antlrDiagnosticOnce       sync.Once
	antlrDiagnosticEnabled    bool
	antlrDiagnosticExactOnce  sync.Once
	antlrDiagnosticExact      bool
	antlrDiagnosticLimitOnce  sync.Once
	antlrDiagnosticLimitValue int
)

func antlrDiagnosticEnabledNow() bool {
	antlrDiagnosticOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv(envAntlrDiagnostic))
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "y", "on", "enable", "enabled":
			antlrDiagnosticEnabled = true
		default:
			antlrDiagnosticEnabled = false
		}
	})
	return antlrDiagnosticEnabled
}

func antlrDiagnosticExactNow() bool {
	antlrDiagnosticExactOnce.Do(func() {
		raw := strings.TrimSpace(os.Getenv(envAntlrDiagnosticExact))
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "y", "on", "enable", "enabled":
			antlrDiagnosticExact = true
		default:
			antlrDiagnosticExact = false
		}
	})
	return antlrDiagnosticExact
}

func antlrDiagnosticLimitNow() int {
	antlrDiagnosticLimitOnce.Do(func() {
		antlrDiagnosticLimitValue = 50
		raw := strings.TrimSpace(os.Getenv(envAntlrDiagnosticLimit))
		if raw == "" {
			return
		}
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			antlrDiagnosticLimitValue = v
		}
	})
	return antlrDiagnosticLimitValue
}

type loggingDiagnosticErrorListener struct {
	*antlr.DefaultErrorListener
	exactOnly bool
	remaining int
}

func newLoggingDiagnosticErrorListener(exactOnly bool, limit int) *loggingDiagnosticErrorListener {
	return &loggingDiagnosticErrorListener{
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
		exactOnly:            exactOnly,
		remaining:            limit,
	}
}

func (l *loggingDiagnosticErrorListener) emit(kind string, recognizer antlr.Parser, startIndex, stopIndex int, extra string) {
	if l == nil {
		return
	}
	if l.remaining == 0 {
		return
	}
	if l.remaining > 0 {
		l.remaining--
	}

	text := recognizer.GetTokenStream().GetTextFromInterval(antlr.NewInterval(startIndex, stopIndex))
	text = utils.ShrinkString(strings.ReplaceAll(text, "\n", "\\n"), 160)
	if extra != "" {
		extra = " " + extra
	}
	log.Infof("[antlr-diagnostic] %s %s tokens=%d..%d%s input=%q", kind, antlrDecisionDescription(recognizer), startIndex, stopIndex, extra, text)
}

func antlrDecisionDescription(recognizer antlr.Parser) string {
	if recognizer == nil {
		return "decision=?"
	}
	interp := recognizer.GetInterpreter()
	if interp == nil {
		return "decision=?"
	}
	rv := reflect.ValueOf(interp)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return "decision=?"
	}
	elem := rv.Elem()
	if !elem.IsValid() {
		return "decision=?"
	}

	dfaField := elem.FieldByName("dfa")
	if !dfaField.IsValid() || dfaField.IsNil() {
		return "decision=?"
	}
	dfa := reflect.NewAt(dfaField.Type(), unsafe.Pointer(dfaField.UnsafeAddr())).Elem().Interface()
	dfaValue := reflect.ValueOf(dfa)
	if dfaValue.Kind() != reflect.Ptr || dfaValue.IsNil() {
		return "decision=?"
	}
	dfaElem := dfaValue.Elem()

	decisionField := dfaElem.FieldByName("decision")
	if !decisionField.IsValid() {
		return "decision=?"
	}
	decision := int(reflect.NewAt(decisionField.Type(), unsafe.Pointer(decisionField.UnsafeAddr())).Elem().Int())

	ruleDesc := ""
	startStateField := dfaElem.FieldByName("atnStartState")
	if startStateField.IsValid() && !startStateField.IsNil() {
		state := reflect.NewAt(startStateField.Type(), unsafe.Pointer(startStateField.UnsafeAddr())).Elem().Interface()
		if ds, ok := state.(antlr.DecisionState); ok {
			ruleIndex := ds.GetRuleIndex()
			ruleNames := recognizer.GetRuleNames()
			if ruleIndex >= 0 && ruleIndex < len(ruleNames) && ruleNames[ruleIndex] != "" {
				ruleDesc = fmt.Sprintf(" rule=%s", ruleNames[ruleIndex])
			}
		}
	}
	return fmt.Sprintf("decision=%d%s", decision, ruleDesc)
}

func (l *loggingDiagnosticErrorListener) ReportAmbiguity(recognizer antlr.Parser, dfa *antlr.DFA, startIndex, stopIndex int, exact bool, ambigAlts *antlr.BitSet, configs antlr.ATNConfigSet) {
	if l.exactOnly && !exact {
		return
	}
	extra := fmt.Sprintf("exact=%v", exact)
	if ambigAlts != nil {
		extra += fmt.Sprintf(" ambig_alts=%s", ambigAlts.String())
	}
	l.emit("ambiguity", recognizer, startIndex, stopIndex, extra)
}

func (l *loggingDiagnosticErrorListener) ReportAttemptingFullContext(recognizer antlr.Parser, dfa *antlr.DFA, startIndex, stopIndex int, conflictingAlts *antlr.BitSet, configs antlr.ATNConfigSet) {
	l.emit("attempting_full_context", recognizer, startIndex, stopIndex, "")
}

func (l *loggingDiagnosticErrorListener) ReportContextSensitivity(recognizer antlr.Parser, dfa *antlr.DFA, startIndex, stopIndex, prediction int, configs antlr.ATNConfigSet) {
	l.emit("context_sensitivity", recognizer, startIndex, stopIndex, fmt.Sprintf("prediction=%d", prediction))
}
