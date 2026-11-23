package syntaxflow_scan

import (
	"sort"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// WithAllLanguages compiles every supported language once over the same source
// tree. Source rules still inspect the tree once; struct/SSA rules then run on
// the program of their own language.
var WithAllLanguages = ssaconfig.SetOption("syntaxflow-scan/allLanguages", func(c *Config, enabled bool) {
	c.allLanguages = enabled
})

// projectLanguageConfigs returns one compile config per registered SSA
// language. Every config targets the same tree and leaves file selection to
// that language's builder, so no file is partitioned or excluded here.
func projectLanguageConfigs(cfg *ssaconfig.Config) ([]*ssaconfig.Config, error) {
	raw, err := cfg.ToJSONRaw()
	if err != nil {
		return nil, err
	}
	languages := make([]ssaconfig.Language, 0, len(ssaapi.LanguageBuilderCreater))
	for language := range ssaapi.LanguageBuilderCreater {
		languages = append(languages, language)
	}
	sort.Slice(languages, func(i, j int) bool { return languages[i] < languages[j] })

	configs := make([]*ssaconfig.Config, 0, len(languages))
	for _, language := range languages {
		child, err := ssaconfig.NewCLIScanConfig(
			ssaconfig.WithJsonRawConfig(raw),
			copyExtraInfoOption(cfg),
			ssaconfig.WithProjectLanguage(language),
			ssaconfig.WithSetProgramName(cfg.GetProgramName()+"-"+string(language)),
			// A broken file must not abort its siblings, even when the input
			// JSON requested strict compilation for a single-language run.
			ssaconfig.WithCompileStrictMode(false),
		)
		if err != nil {
			return nil, err
		}
		configs = append(configs, child)
		log.Infof("[code-scan] language=%s program=%s", language, child.GetProgramName())
	}
	return configs, nil
}

func recordCompileDiagnostics(recorder *stageOutcomeRecorder, stage ProductStage, d ssa.CompileDiagnostics) {
	if !d.Incomplete() {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.compileDiagnostics == nil {
		recorder.compileDiagnostics = make(map[ProductStage]ssa.CompileDiagnostics)
	}
	total := recorder.compileDiagnostics[stage]
	total.ASTErrors += d.ASTErrors
	total.FileErrors += d.FileErrors
	total.PreHandlerErrors += d.PreHandlerErrors
	total.ConfigErrors += d.ConfigErrors
	total.TemplatesSkipped += d.TemplatesSkipped
	for _, example := range d.Examples {
		if len(total.Examples) >= 20 {
			break
		}
		total.Examples = append(total.Examples, example)
	}
	recorder.compileDiagnostics[stage] = total
}
