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
	rule    *schema.SyntaxFlowRule
}

func SyncRuleFromFileSystemToDB(db *gorm.DB, fsInstance filesys_interface.FileSystem, buildin bool, notifies ...func(process float64, ruleName string)) (err error) {
	return syncRuleFromFileSystemToDB(db, fsInstance, buildin, false, notifies...)
}

// replaceBuiltins is only used for the complete embedded snapshot. Imports of
// individual files or directories must not delete other installed rules.
func syncRuleFromFileSystemToDB(db *gorm.DB, fsInstance filesys_interface.FileSystem, buildin, replaceBuiltins bool, notifies ...func(process float64, ruleName string)) (err error) {
	if db == nil {
		return utils.Errorf("profile db is nil")
	}
	var notify func(process float64, ruleName string)
	if len(notifies) != 0 {
		notify = notifies[0]
		defer func() {
			if err == nil && notify != nil {
				notify(1, "同步SyntaxFlow规则成功！")
			}
		}()
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

	// Reuse complete stored rules when their source content is unchanged. The
	// stored rule already contains metadata produced by the SyntaxFlow AST and
	// compiled opcodes, so there is no need for a parallel text parser or a
	// partial fingerprint type.
	stored, err := sfdb.LoadStoredRulesForSync(db, buildin)
	if err != nil {
		log.Warnf("load stored rules failed, falling back to full sync: %s", err)
		stored = nil
	}
	storedByContent := make(map[string]*schema.SyntaxFlowRule, len(stored))
	for _, rule := range stored {
		if rule != nil && rule.Content != "" {
			storedByContent[rule.Content] = rule
		}
	}
	changed := len(stored) != len(files)
	for i := range files {
		if rule, ok := storedByContent[files[i].content]; ok {
			files[i].rule = rule
			continue
		}
		changed = true
	}
	forceSync := utils.InterfaceToBoolean(os.Getenv("YAK_SYNTAXFLOW_FORCE_RULE_SYNC"))
	if forceSync {
		log.Infof("YAK_SYNTAXFLOW_FORCE_RULE_SYNC set: re-importing every builtin rule")
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

	scanStart := time.Now()
	needsWrite := make([]bool, len(files))
	writeCount := 0
	for i := range files {
		needsWrite[i] = forceSync || files[i].rule == nil || (replaceBuiltins && changed)
		if needsWrite[i] {
			writeCount++
		}
	}
	if (writeCount == 0 && !replaceBuiltins) || (replaceBuiltins && !changed && !forceSync) {
		for _, f := range files {
			progress(f.name)
		}
		log.Infof("sync embed rules: total=%d skipped=%d cost=%v (all rules reused from database)",
			len(files), len(files), time.Since(scanStart))
		return nil
	}

	// Compile only files for which no complete stored rule exists. Parsing is
	// parallel, while all database writes stay on one goroutine for SQLite.
	workers := runtime.GOMAXPROCS(0)
	if workers > 8 {
		workers = 8
	}
	if workers < 1 {
		workers = 1
	}
	type compileResult struct {
		index int
		rule  *schema.SyntaxFlowRule
		err   error
	}
	compileIndexes := make([]int, 0, len(files))
	for i := range files {
		if needsWrite[i] && files[i].rule == nil {
			compileIndexes = append(compileIndexes, i)
		}
	}
	compileAll := func(indexes []int) <-chan compileResult {
		results := make(chan compileResult, workers)
		if workers == 1 || len(indexes) <= 1 {
			go func() {
				defer close(results)
				for _, index := range indexes {
					rule, err := sfdb.CheckSyntaxFlowRuleContent(files[index].content)
					results <- compileResult{index: index, rule: rule, err: err}
				}
			}()
			return results
		}
		jobs := make(chan int)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for index := range jobs {
					rule, err := sfdb.CheckSyntaxFlowRuleContent(files[index].content)
					results <- compileResult{index: index, rule: rule, err: err}
				}
			}()
		}
		go func() {
			defer close(jobs)
			for _, index := range indexes {
				jobs <- index
			}
		}()
		go func() {
			wg.Wait()
			close(results)
		}()
		return results
	}

	compileStart := time.Now()
	compiled := compileAll(compileIndexes)
	compileErrs := make(map[int]error, len(compileIndexes))
	var firstErr error
	for res := range compiled {
		if res.err != nil {
			compileErrs[res.index] = res.err
			if firstErr == nil {
				firstErr = res.err
			}
			continue
		}
		files[res.index].rule = res.rule
	}
	compileCost := time.Since(compileStart)
	if replaceBuiltins && firstErr != nil {
		for i := range files {
			if err := compileErrs[i]; err != nil {
				return utils.Wrapf(err, "compile builtin rule %s", files[i].name)
			}
		}
	}
	if replaceBuiltins {
		if err := sfdb.DeleteBuildInRuleWithDB(db); err != nil {
			return err
		}
	}

	writeStart := time.Now()
	for i := range files {
		f := files[i]
		if !needsWrite[i] {
			progress(f.name)
			continue
		}
		if err := compileErrs[i]; err != nil {
			log.Warnf("compile rule %s error: %s", f.name, err)
			continue
		}
		rule := cloneRuleForSync(f.rule)
		if err := sfdb.ImportCompiledRuleWithDB(db, f.name, f.content, f.path, buildin, rule, f.tags...); err != nil {
			log.Warnf("import rule %s error: %s", f.name, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		progress(f.name)
	}
	log.Infof("sync embed rules: total=%d skipped=%d compiled=%d compile=%v write=%v",
		len(files), len(files)-writeCount, len(compileIndexes), compileCost, time.Since(writeStart))
	return firstErr
}

func cloneRuleForSync(rule *schema.SyntaxFlowRule) *schema.SyntaxFlowRule {
	if rule == nil {
		return nil
	}
	cloned := *rule
	cloned.Model = gorm.Model{}
	cloned.Hash = ""
	cloned.CWE = append(schema.StringArray(nil), rule.CWE...)
	cloned.Groups = nil
	return &cloned
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
	return utils.Wrapf(syncRuleFromFileSystemToDB(db, ruleFSWithHash, true, true, notifies...), "init builtin rules to custom db error")
}

// syncEmbedRuleInternal 内部同步实现（不处理 hash 更新，由调用者决定）
func syncEmbedRuleInternal(notifies ...func(process float64, ruleName string)) (err error) {
	log.Infof("start sync embed rule")

	var notify func(process float64, ruleName string)
	if len(notifies) > 0 {
		notify = notifies[0]
	}

	// 对于 gzip 版本，设置进度通知回调，以便在解压过程中显示进度
	// 注意：这需要在 GetRuleFileSystem() 之前调用
	InitEmbedFSWithNotify(notify)

	err = syncRuleFromFileSystemToDB(consts.GetGormProfileDatabase(), ruleFSWithHash, true, true, notifies...)

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
