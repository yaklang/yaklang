package core

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/log"
)

type decompileTraceConfig struct {
	classFilter  string
	methodFilter string
	varTable     bool
	varFold      bool
	rewriteVar   bool
	slotVersion  bool
}

func currentTraceConfig() decompileTraceConfig {
	return decompileTraceConfig{
		classFilter:  os.Getenv("JDEC_TRACE_CLASS"),
		methodFilter: os.Getenv("JDEC_TRACE_METHOD"),
		varTable:     os.Getenv("JDEC_TRACE_VAR_TABLE") != "",
		varFold:      os.Getenv("JDEC_TRACE_VAR_FOLD") != "",
		rewriteVar:   os.Getenv("JDEC_TRACE_REWRITE_VAR") != "",
		slotVersion:  os.Getenv("JDEC_TRACE_SLOT_VERSION") != "",
	}
}

func (d *Decompiler) traceEnabled(kind string) bool {
	cfg := currentTraceConfig()
	switch kind {
	case "var-table":
		if !cfg.varTable {
			return false
		}
	case "var-fold":
		if !cfg.varFold {
			return false
		}
	case "slot-version":
		if !cfg.slotVersion {
			return false
		}
	default:
		return false
	}
	if cfg.classFilter != "" && !strings.Contains(d.FunctionContext.ClassName, cfg.classFilter) {
		return false
	}
	if cfg.methodFilter != "" && !strings.Contains(d.traceMethodName(), cfg.methodFilter) {
		return false
	}
	return true
}

func TraceRewriteVarEnabled(className, methodName string) bool {
	cfg := currentTraceConfig()
	if !cfg.rewriteVar {
		return false
	}
	if cfg.classFilter != "" && !strings.Contains(className, cfg.classFilter) {
		return false
	}
	if cfg.methodFilter != "" && !strings.Contains(methodName, cfg.methodFilter) {
		return false
	}
	return true
}

func TraceRewriteVar(className, methodName, format string, args ...any) {
	if !TraceRewriteVarEnabled(className, methodName) {
		return
	}
	log.Infof("[jdec-trace][rewrite-var] %s.%s "+format, append([]any{className, methodName}, args...)...)
}

func (d *Decompiler) traceMethodName() string {
	if d.FunctionContext == nil || d.FunctionContext.FunctionName == "" {
		return "<unknown>"
	}
	return d.FunctionContext.FunctionName
}

func (d *Decompiler) tracef(kind, format string, args ...any) {
	if !d.traceEnabled(kind) {
		return
	}
	log.Infof("[jdec-trace][%s] %s.%s "+format, append([]any{kind, d.FunctionContext.ClassName, d.traceMethodName()}, args...)...)
}

func traceRef(ref *values.JavaRef, ctx *class_context.ClassContext) string {
	if ref == nil {
		return "<nil>"
	}
	name := "<nil-id>"
	if ref.Id != nil {
		name = ref.Id.String()
	}
	typ := "<nil-type>"
	if t := ref.Type(); t != nil {
		typ = t.String(ctx)
	}
	return fmt.Sprintf("%s/%s/%s", name, ref.VarUid, typ)
}

func traceValue(v values.JavaValue, ctx *class_context.ClassContext) string {
	if v == nil {
		return "<nil>"
	}
	if ref, ok := v.(*values.JavaRef); ok {
		return traceRef(ref, ctx)
	}
	typ := "<nil-type>"
	if t := v.Type(); t != nil {
		typ = t.String(ctx)
	}
	return fmt.Sprintf("%T/%s/%s", v, typ, v.String(ctx))
}

func traceVarTable(varTable map[int]*values.JavaRef, ctx *class_context.ClassContext) string {
	slots := make([]int, 0, len(varTable))
	for slot := range varTable {
		slots = append(slots, slot)
	}
	sort.Ints(slots)
	parts := make([]string, 0, len(slots))
	for _, slot := range slots {
		parts = append(parts, fmt.Sprintf("%d=%s", slot, traceRef(varTable[slot], ctx)))
	}
	return strings.Join(parts, ",")
}
