package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ReconcileSessionLanguageFromMarkers updates session.Language when build markers disagree with the current value.
func ReconcileSessionLanguageFromMarkers(ctx context.Context, rt *Runtime) (changed bool, warnings []string, err error) {
	if rt == nil || rt.Repo == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return false, nil, nil
	}
	sess := rt.Session
	rec, rerr := ReconcileLanguage(sess.CodeRootPath, sess.Language)
	if rerr != nil {
		return false, nil, rerr
	}
	warnings = append(warnings, rec.Warnings...)
	newLang := string(rec.Language)
	if strings.EqualFold(newLang, strings.TrimSpace(sess.Language)) {
		return false, warnings, nil
	}
	oldLang := sess.Language
	sess.Language = newLang
	if uerr := rt.Repo.UpdateSession(sess); uerr != nil {
		return false, warnings, uerr
	}
	_ = rt.Repo.AppendEvent(sess.ID, "info", "language_reconcile", string(utils.Jsonify(map[string]any{
		"old": oldLang, "new": newLang, "source": rec.Source, "warnings": rec.Warnings,
	})))
	log.Infof("ssa_api_discovery: language reconciled %q -> %q (source=%s)", oldLang, newLang, rec.Source)
	if rerr := RecompileSessionSSA(ctx, rt); rerr != nil {
		log.Warnf("ssa_api_discovery: SSA recompile after language reconcile: %v", rerr)
	}
	rt.Session = sess
	return true, warnings, nil
}

// RecompileSessionSSA compiles the project with session.Language and updates session SSA fields.
func RecompileSessionSSA(ctx context.Context, rt *Runtime) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return utils.Error("nil runtime")
	}
	sess := rt.Session
	if !sess.CodePathOK || strings.TrimSpace(sess.CodeRootPath) == "" {
		return utils.Errorf("invalid code root")
	}
	if strings.TrimSpace(sess.Language) == "" {
		sess.SSACompileOK = false
		sess.SSACompileError = "language not resolved"
		_ = rt.Repo.UpdateSession(sess)
		return utils.Errorf("language not resolved")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	langEnum := ssaconfig.Language(sess.Language)
	progName := fmt.Sprintf("disc_%s", sess.UUID[:8])
	progs, perr := ssaapi.ParseProjectFromPath(sess.CodeRootPath,
		ssaapi.WithLanguage(langEnum),
		ssaapi.WithProgramName(progName),
		ssaapi.WithContext(ctx),
	)
	if perr != nil || len(progs) == 0 {
		sess.SSACompileOK = false
		if perr != nil {
			sess.SSACompileError = perr.Error()
		} else {
			sess.SSACompileError = "compilation produced no programs"
		}
		log.Warnf("ssa_api_discovery: SSA compile failed: %v", perr)
	} else {
		prog := progs[0]
		sess.SSACompileOK = true
		sess.SSAProgramName = prog.GetProgramName()
		sess.SSAFileCount = len(prog.Program.FileList)
		sess.SSACompileError = ""
		log.Infof("ssa_api_discovery: SSA ok program=%s files=%d lang=%s", sess.SSAProgramName, sess.SSAFileCount, sess.Language)
	}
	if err := rt.Repo.UpdateSession(sess); err != nil {
		return err
	}
	_ = rt.Repo.AppendEvent(sess.ID, "info", "ssa_compile", string(utils.Jsonify(map[string]any{
		"ok": sess.SSACompileOK, "program": sess.SSAProgramName, "files": sess.SSAFileCount,
		"err": sess.SSACompileError, "language": sess.Language,
	})))
	rt.Session = sess
	return nil
}
