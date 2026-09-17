//go:build !irify_exclude

package sfbuildin

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
	"github.com/yaklang/yaklang/common/utils/resources_monitor"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var ruleFSWithHash resources_monitor.ResourceMonitor

func GetRuleFS() *embed.FS {
	return nil
}

func SyncRuleFromFileSystem(fsInstance filesys_interface.FileSystem, buildin bool, notifies ...func(process float64, ruleName string)) (err error) {
	return SyncRuleFromFileSystemToDB(consts.GetGormProfileDatabase(), fsInstance, buildin, notifies...)
}

// ruleFile is one .sf file collected from the embed before syncing.
type ruleFile struct {
	path    string
	name    string
	content string
	tags    []string
}

func SyncRuleFromFileSystemToDB(db *gorm.DB, fsInstance filesys_interface.FileSystem, buildin bool, notifies ...func(process float64, ruleName string)) (err error) {
	if db == nil {
		return utils.Errorf("profile db is nil")
	}
	var notify func(process float64, ruleName string)
	if len(notifies) != 0 {
		notify = notifies[0]
		defer notify(1, "同步SyntaxFlow规则成功！")
	}

	// Collect the rule files first. The old single-pass version compiled and
	// wrote one rule at a time, so a 1000+ rule embed re-synced every rule
	// serially and spent 3-4s in the ANTLR parser alone on every cold start.
	var files []ruleFile
	err = filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		dirName, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}
		raw, err := fsInstance.ReadFile(s)
		if err != nil {
			return utils.Wrapf(err, "read file[%s] error", s)
		}
		files = append(files, ruleFile{
			path:    s,
			name:    name,
			content: string(raw),
			tags:    ruleTagsFromDir(dirName),
		})
		return nil
	}))
	if err != nil {
		return err
	}
	totalCount := float64(len(files))

	// Rules already stored with compiled opcodes are skipped outright:
	// re-importing identical bytes only redoes the parse and rewrites rows that
	// already exist. Only rules whose content changed (or that were stored
	// before opcode caching) pay the ANTLR parse again.
	stored, err := sfdb.LoadStoredContentHashesByRuleName(db, buildin)
	if err != nil {
		log.Warnf("load stored rule hashes failed, falling back to full sync: %s", err)
		stored = nil
	}
	// The stored fingerprint only covers the rule bytes, tags and mode. A
	// change to the metadata enricher (groups, versions) without a content
	// change would not be picked up by the skip below, so allow a full
	// re-import when that needs to be forced.
	if utils.InterfaceToBoolean(os.Getenv("YAK_SYNTAXFLOW_FORCE_RULE_SYNC")) {
		log.Infof("YAK_SYNTAXFLOW_FORCE_RULE_SYNC set: re-importing every builtin rule")
		stored = nil
	}

	var handledCount float64
	progress := func(name string) {
		handledCount++
		if notify == nil {
			return
		}
		if totalCount > 0 {
			notify(handledCount/totalCount, fmt.Sprintf("更新内置SyntaxFlow规则:%s ", name))
		} else {
			notify(1, "没有内置SyntaxFlow规则需要更新。")
		}
	}

	// Split the embed into rules that need work and rules already cached. The
	// cached ones still advance the progress callback, so callers keep seeing
	// the same 0->1 progression.
	scanStart := time.Now()
	pending := make([]ruleFile, 0, len(files))
	for _, f := range files {
		if syncRuleAlreadyStored(stored, f, buildin) {
			progress(f.name)
			continue
		}
		pending = append(pending, f)
	}
	if len(pending) == 0 {
		log.Infof("sync embed rules: total=%d skipped=%d cost=%v (all rules reused from database)",
			len(files), len(files), time.Since(scanStart))
		return nil
	}

	// Compile in parallel but keep every DB write on one goroutine: the profile
	// DB is a single-writer SQLite, and concurrent writes here only trade
	// parse time for `database is locked` retries. Bounded so a large embed
	// does not spawn 1000 parser goroutines at once.
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}

	type compileResult struct {
		file ruleFile
		rule *schema.SyntaxFlowRule
		err  error
	}

	compileAll := func(input []ruleFile) []compileResult {
		out := make([]compileResult, 0, len(input))
		if workers == 1 || len(input) <= 1 {
			for _, f := range input {
				rule, err := sfdb.CompileRuleForSync(f.content)
				out = append(out, compileResult{file: f, rule: rule, err: err})
			}
			return out
		}
		jobs := make(chan ruleFile)
		results := make(chan compileResult, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for f := range jobs {
					rule, err := sfdb.CompileRuleForSync(f.content)
					results <- compileResult{file: f, rule: rule, err: err}
				}
			}()
		}
		go func() {
			defer close(jobs)
			for _, f := range input {
				jobs <- f
			}
		}()
		go func() {
			wg.Wait()
			close(results)
		}()
		for res := range results {
			out = append(out, res)
		}
		return out
	}

	var firstErr error
	compileStart := time.Now()
	compiled := compileAll(pending)
	compileCost := time.Since(compileStart)
	writeStart := time.Now()
	for _, res := range compiled {
		f := res.file
		if res.err == nil {
			res.err = sfdb.ImportCompiledRuleWithDB(db, f.name, f.content, f.path, buildin, res.rule, f.tags...)
		}
		if res.err != nil {
			log.Warnf("import rule %s error: %s", f.name, res.err)
			if firstErr == nil {
				firstErr = res.err
			}
			continue
		}
		progress(f.name)
	}
	log.Infof("sync embed rules: total=%d skipped=%d compiled=%d compile=%v write=%v",
		len(files), len(files)-len(pending), len(pending), compileCost, time.Since(writeStart))
	return firstErr
}

// storedRuleName is the rule_name a sync pass produces for a file, used to
// match the embed against rows already in the profile DB. Builtin rules are
// stored under their title (title_zh first, then title, then the file name);
// non-builtin imports keep the file's base name.
func storedRuleName(fileName, content string, buildin bool) string {
	if !buildin {
		return fileName
	}
	meta := extractRuleTitleMetadata(content)
	if meta.TitleZh != "" {
		return meta.TitleZh
	}
	if meta.Title != "" {
		return meta.Title
	}
	return fileName
}

// syncRuleAlreadyStored reports whether the stored rule set already holds this
// file's exact content with compiled opcodes. Both the title-derived name and
// the raw file name are probed: the metadata scan is a fast path that cannot
// cover every legal desc() spelling, and a false negative only costs one
// recompile, while a false positive would drop rule updates.
func syncRuleAlreadyStored(stored map[string]sfdb.StoredRuleFingerprint, f ruleFile, buildin bool) bool {
	if stored == nil {
		return false
	}
	hash := sfdb.RuleContentHash(f.content)
	// The tag string is rebuilt from the directory path on every sync, so a
	// rule moved between tag folders must be re-imported even though its bytes
	// are unchanged.
	mode := extractRuleMode(f.content)
	tags := ruleTagString(f.tags, mode)
	matches := func(got sfdb.StoredRuleFingerprint) bool {
		return got.ContentHash == hash && got.Tag == tags && got.Mode == mode
	}
	if got, ok := stored[storedRuleName(f.name, f.content, buildin)]; ok && matches(got) {
		return true
	}
	if got, ok := stored[f.name]; ok && matches(got) {
		return true
	}
	return false
}

// extractRuleMode reads the `mode:` value out of a rule's desc() block without
// compiling it, normalized the way schema.ValidRuleMode does (an absent or
// unrecognized mode means ssa) plus the sfvm mode aliases the compiler folds
// into the rule mode.
func extractRuleMode(content string) string {
	raw := ""
	for _, item := range ruleDescItems(content) {
		if item.key == "mode" && item.value != "" {
			raw = strings.ToLower(item.value)
			break
		}
	}
	switch raw {
	case "source", "pattern", "sfpattern":
		return string(schema.SFR_MODE_SOURCE)
	case "struct":
		return string(schema.SFR_MODE_STRUCT)
	default:
		return string(schema.SFR_MODE_SSA)
	}
}

// ruleTagString renders the tag string an import produces for a rule without
// compiling it: the directory tags first, then the mode tag the compiler adds
// for `desc(mode: source|struct)`. Content-level desc tags (cwe/tag keys) are
// not visible to the line scan, so the sync treats a stored rule whose tag no
// longer matches as changed and re-imports it. That costs one recompile for
// such rules but can never drop a real rule update.
func ruleTagString(tags []string, mode string) string {
	merged := ""
	for _, t := range tags {
		merged = sfvm.AppendRuleTag(merged, t)
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case string(schema.SFR_MODE_SOURCE):
		merged = sfvm.AppendRuleTag(merged, sfvm.RuleModeSource)
	case string(schema.SFR_MODE_STRUCT):
		merged = sfvm.AppendRuleTag(merged, sfvm.RuleModeStruct)
	}
	return merged
}

// ruleTagsFromDir derives the tag list from the rule's directory path, the
// same way the sync path has always done (CWE/CVE folder names become tags).
func ruleTagsFromDir(dirName string) []string {
	var tags []string
	for _, block := range utils.PrettifyListFromStringSplitEx(dirName, "/", "\\", ",", "|") {
		block = strings.ToLower(block)
		if block == "buildin" {
			continue
		}
		if strings.HasPrefix(block, "cwe-") {
			result, err := regexp_utils.NewYakRegexpUtils(`(cwe-\d+)(-(.*))?`).FindStringSubmatch(block)
			if err != nil {
				continue
			}
			tags = append(tags, strings.ToUpper(result[1]))
			tags = append(tags, result[3])
			continue
		} else if strings.HasPrefix(block, "cve-") {
			result, err := regexp_utils.NewYakRegexpUtils(`(cve-\d+-\d+)([_-\.](.*))?`).FindStringSubmatch(block)
			if err != nil {
				continue
			}
			tags = append(tags, strings.ToUpper(result[1]))
			tags = append(tags, result[3])
			continue
		}
		tags = append(tags, block)
	}
	return tags
}

func SyncEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	const key = consts.EmbedSfBuildInRuleKey
	return resources_monitor.NewEmbedResourcesMonitor(key, consts.ExistedSyntaxFlowEmbedFSHash).MonitorModifiedWithAction(func() string {
		hash, _ := SyntaxFlowRuleHash()
		return hash
	}, func() error {
		return syncEmbedRuleInternal(notifies...)
	})
}

// ForceSyncEmbedRule 强制同步嵌入规则，忽略哈希检查
func ForceSyncEmbedRule(notifies ...func(process float64, ruleName string)) (err error) {
	err = syncEmbedRuleInternal(notifies...)
	if err == nil {
		DoneEmbedRule()
	}
	return err
}

func ForceSyncEmbedRuleToDB(db *gorm.DB, notifies ...func(process float64, ruleName string)) (err error) {
	if db == nil {
		return utils.Errorf("profile db is nil")
	}
	log.Infof("start sync embed rule to custom db")
	var notify func(process float64, ruleName string)
	if len(notifies) > 0 {
		notify = notifies[0]
	}
	InitEmbedFSWithNotify(notify)
	return utils.Wrapf(SyncRuleFromFileSystemToDB(db, ruleFSWithHash, true, notifies...), "init builtin rules to custom db error")
}

// syncEmbedRuleInternal 内部同步实现（不处理 hash 更新，由调用者决定）
func syncEmbedRuleInternal(notifies ...func(process float64, ruleName string)) (err error) {
	log.Infof("start sync embed rule")
	// sfdb.DeleteBuildInRule()

	var notify func(process float64, ruleName string)
	if len(notifies) > 0 {
		notify = notifies[0]
	}

	// 对于 gzip 版本，设置进度通知回调，以便在解压过程中显示进度
	// 注意：这需要在 GetRuleFileSystem() 之前调用
	InitEmbedFSWithNotify(notify)

	err = SyncRuleFromFileSystem(ruleFSWithHash, true, notifies...)

	return utils.Wrapf(err, "init builtin rules error")
}

// SyntaxFlowRuleHash is deprecated. Use filesys.CreateEmbedFSHash(ruleFS, filesys.WithIncludeExts(".sf")) instead.
// This function is kept for backward compatibility but should not be used in new code.
func SyntaxFlowRuleHash() (string, error) {
	// Use GetHash method to calculate hash for .sf files
	hash, err := ruleFSWithHash.GetHash()
	if err != nil {
		// Check if error is due to no .sf files found
		if errors.Is(err, filesys.ErrNoFileFound) {
			return "", utils.Error("no .sf file found")
		}
		return "", err
	}
	return hash, nil
}

func NeedSyncEmbedRule() bool {
	diffHash := yakit.Get(consts.EmbedSfBuildInRuleKey) != consts.ExistedSyntaxFlowEmbedFSHash
	return diffHash
}

func DoneEmbedRule() {
	log.Infof("done sync embed rule with hash: %s", consts.ExistedSyntaxFlowEmbedFSHash)
	yakit.Set(consts.EmbedSfBuildInRuleKey, consts.ExistedSyntaxFlowEmbedFSHash)
}
