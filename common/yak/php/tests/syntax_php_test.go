package tests

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
)

//go:embed syntax/***
var syntaxFs embed.FS

//go:embed large/***
var largeSyntaxFs embed.FS

var phpTestAntlrCache = func() *ssa.AntlrCache {
	return php2ssa.CreateBuilder().GetAntlrCache()
}()

const phpFreshSyntaxCacheMinBytes = 64 * 1024

var phpTestCacheResetState = struct {
	mu          sync.Mutex
	filesParsed int
	resetEvery  int
}{
	resetEvery: phpTestCacheResetEveryFiles(),
}

func newPHPTestAntlrCache() *ssa.AntlrCache {
	return php2ssa.CreateBuilder().GetAntlrCache()
}

func phpTestCacheResetEveryFiles() int {
	raw := strings.TrimSpace(os.Getenv("YAK_ANTLR_CACHE_RESET_FILES"))
	if raw == "" {
		return 100
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func nextPHPTestAntlrCache(src string) *ssa.AntlrCache {
	if len(src) >= phpFreshSyntaxCacheMinBytes {
		return newPHPTestAntlrCache()
	}

	phpTestCacheResetState.mu.Lock()
	defer phpTestCacheResetState.mu.Unlock()

	phpTestCacheResetState.filesParsed++
	if phpTestAntlrCache != nil && phpTestCacheResetState.resetEvery > 0 && phpTestCacheResetState.filesParsed%phpTestCacheResetState.resetEvery == 0 {
		phpTestAntlrCache.ResetRuntimeCaches()
	}
	return phpTestAntlrCache
}

var syntaxNonASTAssets = map[string]struct{}{
	"syntax/composer.lock": {},
}

var largeProjectSlowFiles = map[string]map[string]struct{}{
	"filament": {
		"docs-assets/app/app/Livewire/TablesDemo.php":   {},
		"tests/src/Forms/Components/SelectTest.php":     {},
		"tests/src/Tables/ColumnTest.php":               {},
		"tests/src/Tables/Filters/QueryBuilderTest.php": {},
	},
	"PrestaShop": {
		"classes/controller/AdminController.php": {},
	},
	"QloApps": {
		"classes/controller/AdminController.php":              {},
		"controllers/admin/AdminImportController.php":         {},
		"controllers/admin/AdminNormalProductsController.php": {},
		"controllers/admin/AdminOrdersController.php":         {},
		"controllers/admin/AdminProductsController.php":       {},
		"tools/tcpdf/tcpdf.php":                               {},
	},
}

func phpFixtureParseBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv("YAK_PHP_FIXTURE_PARSE_BUDGET_SEC"))
	if raw == "" {
		return 30 * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func phpLargeFixtureParseBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv("YAK_PHP_LARGE_FIXTURE_PARSE_BUDGET_SEC"))
	if raw == "" {
		return 0
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}

func isKnownLargeProjectSlowFile(projectRoot, filePath string) bool {
	entries, ok := largeProjectSlowFiles[filepath.Base(projectRoot)]
	if !ok {
		return false
	}
	_, ok = entries[filePath]
	return ok
}

func projectFixtureDir(projectRoot string) string {
	return strings.ToLower(filepath.Base(projectRoot))
}

func flattenedProjectFixtureName(filePath string) string {
	return strings.NewReplacer("/", "__", "\\", "__").Replace(filePath)
}

func hasSavedProjectSyntaxFixture(projectRoot, filePath string) bool {
	dir := projectFixtureDir(projectRoot)
	name := flattenedProjectFixtureName(filePath)
	for _, candidate := range []struct {
		root fs.FS
		path string
	}{
		{root: syntaxFs, path: filepath.ToSlash(filepath.Join("syntax", dir, name))},
		{root: syntaxFs, path: filepath.ToSlash(filepath.Join("syntax", dir+"_slow", name))},
		{root: largeSyntaxFs, path: filepath.ToSlash(filepath.Join("large", dir, name))},
	} {
		if _, err := fs.Stat(candidate.root, candidate.path); err == nil {
			return true
		}
	}
	return false
}

func isSyntaxASTFixture(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".php", ".inc", ".php1":
		return true
	default:
		return false
	}
}

func validateSourceWithBudget(t *testing.T, filename string, src string, budget time.Duration) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		errListener := antlr4util.NewErrorListener()
		lexer := phpparser.NewPHPLexer(antlr.NewInputStream(src))
		lexer.RemoveErrorListeners()
		lexer.AddErrorListener(errListener)
		tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
		if os.Getenv("YAK_PHP_DEBUG_TOKENS") != "" {
			source := tokenStream.GetTokenSource()
			for {
				t := source.NextToken()
				if t == nil || t.GetTokenType() == antlr.TokenEOF {
					break
				}
				ty := t.GetTokenType()
				switch t.GetText() {
				case "=":
					fmt.Print("ASSIGN ")
					switch ty {
					case phpparser.PHPLexerEq:
						fmt.Print("EQ ")
					case phpparser.PHPLexerHtmlEquals:
						fmt.Print("HTML_EQ ")
					}
				case "<<<":
					fmt.Print("HEREDOC ")
					if ty != phpparser.PHPLexerStartNowDoc {
						fmt.Print("NOT_START_NOWDOC BAD... ")
					}
				case "EOF":
					fmt.Print("EOF ")
					switch ty {
					case phpparser.PHPLexerHereDocIdentiferName:
						fmt.Print("HERE_DOC_NAME ")
					}
				case "\nEOF":
					fmt.Print("HERE DOC END ")
					if ty != phpparser.PHPLexerEndDoc {
						fmt.Print("NOT_END_NOWDOC BAD... ")
					}
				}
				fmt.Println(t)
			}
		}

		if errListener.GetErrorString() != "" {
			t.Fatalf("Lexer failed: %v", errListener.GetErrorString())
		}

		cache := nextPHPTestAntlrCache(src)

		start := time.Now()
		_, err := php2ssa.Frontend(src, cache)
		elapsed := time.Since(start)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
		if budget > 0 && elapsed > budget {
			t.Fatalf("parse AST exceeded budget for %s: elapsed=%s budget=%s", filename, elapsed, budget)
		}
	})
}

func validateSource(t *testing.T, filename string, src string) {
	validateSourceWithBudget(t, filename, src, phpFixtureParseBudget())
}

func TestAllSyntaxForPHP_G4(t *testing.T) {
	err := fs.WalkDir(syntaxFs, "syntax", func(syntaxPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isSyntaxASTFixture(syntaxPath) {
			return nil
		}
		raw, err := syntaxFs.ReadFile(syntaxPath)
		if err != nil {
			return fmt.Errorf("cannot found syntax fs %s: %w", syntaxPath, err)
		}
		validateSource(t, syntaxPath, string(raw))
		return nil
	})
	require.NoError(t, err, "walk syntax fixtures")
}

func TestAllLargeSyntaxForPHP_G4(t *testing.T) {
	if os.Getenv("YAK_PHP_RUN_LARGE_FIXTURES") == "" {
		t.Skip("set YAK_PHP_RUN_LARGE_FIXTURES=1 to run deferred large PHP syntax fixtures")
	}

	err := fs.WalkDir(largeSyntaxFs, "large", func(syntaxPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isSyntaxASTFixture(syntaxPath) {
			return nil
		}
		raw, err := largeSyntaxFs.ReadFile(syntaxPath)
		if err != nil {
			return fmt.Errorf("cannot found large syntax fs %s: %w", syntaxPath, err)
		}
		validateSourceWithBudget(t, syntaxPath, string(raw), phpLargeFixtureParseBudget())
		return nil
	})
	require.NoError(t, err, "walk large syntax fixtures")
}

func TestSyntaxFixtureCoverage(t *testing.T) {
	var missing []string
	err := fs.WalkDir(syntaxFs, "syntax", func(syntaxPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isSyntaxASTFixture(syntaxPath) {
			return nil
		}
		if _, ok := syntaxNonASTAssets[syntaxPath]; ok {
			return nil
		}
		missing = append(missing, syntaxPath)
		return nil
	})
	require.NoError(t, err)
	sort.Strings(missing)
	require.Empty(t, missing, "syntax directory contains files not covered by AST fixtures or explicit non-AST assets: %v", missing)
}

func TestSyntax_(t *testing.T) {
	validateSource(t, "class member access", `<?php $c->fn = 1; ?>`)
	validateSource(t, `string as class identifier`, `
<?php 
class foo { static $bar = 'baz'; }
var_dump('foo'::$bar);`)
}

// func TestPHPFront(t *testing.T) {
// path := "/home/wlz/Developer/pfsense"
// file := []string{"src/etc/inc/captiveportal.inc", "src/etc/inc/ipsec.inc", "src/usr/local/www/classes/Form/SelectInputCombo.class.php"}
// }

func TestPHPInterpolatedCurlyBraces(t *testing.T) {
	phpCode := `<?php
function build_rules($cpips, $cpiplist, $interfaces, $rdrtag, $authtag) {
	$rules = "table <{$cpips}> { " . join(' ', $cpiplist)  . "}\n";
	$rules .= "ether pass in on { {$interfaces} } tag \"{$rdrtag}\"\n";
	$rules .= "pass in quick on {$interfaces} proto tcp from any to <{$cpips}> port {$rdrtag} ridentifier {$authtag}\n";
	return $rules;
}

$rules = build_rules('cpzone', ['127.0.0.1', '127.0.0.2'], 'igb0 igb1', 'rdr', 'auto');
echo $rules;
`

	ast, err := php2ssa.Frontend(phpCode)
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestPHPLexerInterpolatedCurlyTokenization(t *testing.T) {
	input := `<?php $value = "prefix-{$interface}-suffix"; ?>`

	lexer := phpparser.NewPHPLexer(antlr.NewInputStream(input))
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokens.Fill()
	all := tokens.GetAllTokens()
	require.NotEmpty(t, all, "expected tokens from lexer")

	for _, tok := range all {
		if tok.GetText() == "{" {
			require.Equal(t, phpparser.PHPLexerCurlyOpen, tok.GetTokenType(), "expected interpolation '{' as CurlyOpen token")
		}
		if tok.GetText() == "}" {
			require.Equal(t, phpparser.PHPLexerCloseCurlyBracket, tok.GetTokenType(), "expected interpolation '}' as CloseCurlyBracket token")
		}
	}
}

func TestPHPInterpolatedStringFunctionCall(t *testing.T) {
	code := `<?php
if (is_array(config_get_path("interfaces/{$interface}"))) {
	return get_real_interface($interface, $family);
}
`
	ast, err := php2ssa.Frontend(code)
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestPHPInterpolatedStringWithLiteralCurly(t *testing.T) {
	code := `<?php
$lease = "lease {$_POST['deleteip']} {\n";
$script = "} else {\n";
print("{");
`
	ast, err := php2ssa.Frontend(code)
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestPHPEscapedDollarWithCurlyInterpolation(t *testing.T) {
	code := `<?php
$rule = "pass in \${$oc['descr']} keep state";
`
	ast, err := php2ssa.Frontend(code)
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestPHPDefinedWithExpression(t *testing.T) {
	code := `<?php
class A {
	function check($name) {
		if (defined($this->order[$name])) {
			return true;
		}
		return defined($name);
	}
}
`
	ast, err := php2ssa.Frontend(code)
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestPHPIndentedHereDoc(t *testing.T) {
	code := `<?php
class A {
	function render($input) {
		return <<<EOT
		<div class="inputselectcombo">
			{$this->_select}
			<span>$input</span>
		</div>
		EOT;
	}
}
`
	ast, err := php2ssa.Frontend(code)
	require.NoError(t, err)
	require.NotNil(t, ast)
}

func TestPHPPfsenseFilterRuleString(t *testing.T) {
	code := `<?php
$rules_temp[] = "pass in {$log_preferences['default_pass']} quick on \${$oc['descr']} proto udp from any port = 67 to any port = 68 tag \"dhcpin\" no state ridentifier {$increment_tracker()} {$make_rule_label_string("allow dhcp replies in {$oc['descr']}")}";
`
	validateSource(t, "pfsense-filter-rule", code)
}

func TestPHPIndexedInterpolationBasic(t *testing.T) {
	validateSource(t, "indexed-interpolation-basic", `<?php $s = "{$a['b']}";`)
}

func TestPHPIndexedInterpolationPfsenseVar(t *testing.T) {
	validateSource(t, "indexed-interpolation-pfsense-var", `<?php $s = "{$log_preferences['default_pass']}";`)
}

func TestPHPPrefixedIndexedInterpolationPfsenseVar(t *testing.T) {
	validateSource(t, "prefixed-indexed-interpolation-pfsense-var", `<?php $s = "pass in {$log_preferences['default_pass']}";`)
}

func TestPHPMixedInterpolationWithEscapedDollarTarget(t *testing.T) {
	validateSource(t, "mixed-interpolation-escaped-dollar-target", `<?php $s = "pass in {$log_preferences['default_pass']} quick on \${$oc['descr']}";`)
}

func TestPHPMixedInterpolationWithFunctionCall(t *testing.T) {
	validateSource(t, "mixed-interpolation-function-call", `<?php $s = "pass in {$log_preferences['default_pass']} quick on \${$oc['descr']} ridentifier {$increment_tracker()}";`)
}

func TestPHPMixedInterpolationWithNestedStringInterpolation(t *testing.T) {
	validateSource(t, "mixed-interpolation-nested-string-interpolation", `<?php $s = "pass in {$log_preferences['default_pass']} quick on \${$oc['descr']} ridentifier {$increment_tracker()} {$make_rule_label_string("allow dhcp replies in {$oc['descr']}")}";`)
}

func TestPHPArraySpreadElement(t *testing.T) {
	validateSource(t, "array-spread-element", `<?php $mod_dirs = ['/boot/kernel', ...$add_dirs];`)
}

func TestPHPBackQuoteWithEscapedBacktick(t *testing.T) {
	validateSource(t, "backquote-escaped-backtick", "<?php $key = trim(`KEY=\\`dd count=1 2>/dev/null\\`; echo \\$KEY`);")
}

func TestPHPUseFunctionDefineDefinedImport(t *testing.T) {
	validateSource(t, "use-function-define-defined-import", `<?php
namespace Grav\Common;

use function define;
use function defined;

if (!defined('GRAV_REQUEST_TIME')) {
	define('GRAV_REQUEST_TIME', microtime(true));
}
`)
}

func TestPHPDefineMethodAndQualifiedCall(t *testing.T) {
	validateSource(t, "define-method-and-qualified-call", `<?php
class YamlUpdater {
	public function define(string $variable, $value): void {
	}
}

\define('GRAV_CLI', true);
if (\defined('GRAV_CLI')) {
	$yaml = new YamlUpdater();
	$yaml->define('twig.autoescape', false);
}
`)
}

func TestPHPStaticPropertyNestedIndexAssignment(t *testing.T) {
	validateSource(t, "static-property-nested-index-assignment", `<?php
class NonceStore {
	protected static $nonces = [];

	public static function getNonce($action, $previousTick = false) {
		if (isset(static::$nonces[$action][$previousTick])) {
			return static::$nonces[$action][$previousTick];
		}
		$nonce = md5($action);
		static::$nonces[$action][$previousTick] = $nonce;

		return static::$nonces[$action][$previousTick];
	}
}
`)
}

type ParseError struct {
	Duration time.Duration
	Message  string
}

type phpProjectSlowCandidate struct {
	Path       string
	Content    []byte
	Concurrent time.Duration
}

func phpProjectASTRoot(t *testing.T) string {
	t.Helper()

	if root := os.Getenv("YAK_PHP_PROJECT_AST_TARGET"); root != "" {
		return root
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	return filepath.Join(home, "Target", "pfsense")
}

func TestProjectAst(t *testing.T) {
	if os.Getenv("YAK_PHP_RUN_PROJECT_AST") == "" {
		t.Skip("set YAK_PHP_RUN_PROJECT_AST=1 to run local pfsense project AST integration")
	}

	path := phpProjectASTRoot(t)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("project ast target unavailable: %s: %v", path, err)
	}

	errorFiles := make(map[string]*ssareducer.FileContent)
	var slowFiles []string
	var deferredSlowFiles []string
	var slowCandidates []phpProjectSlowCandidate

	fileList := make([]string, 0, 100)
	fileMap := make(map[string]struct{})

	refFs := filesys.NewRelLocalFs(path)
	filesys.Recursive(".",
		filesys.WithFileSystem(refFs),
		filesys.WithDirStat(func(s string, fi fs.FileInfo) error {
			if s == ".git" {
				return fs.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(filePath string, fi fs.FileInfo) error {
			extern := filepath.Ext(filePath)
			if extern == ".php" || extern == ".inc" {
				if isKnownLargeProjectSlowFile(path, filePath) {
					log.Warnf("skip deferred large project file: %s/%s", filepath.Base(path), filePath)
					return nil
				}
				fileList = append(fileList, filePath)
				fileMap[filePath] = struct{}{}
				return nil
			}
			return nil
		}),
	)
	log.Infof("project AST files queued under %s: %d", path, len(fileList))

	config, err := ssaapi.DefaultConfig(
		ssaapi.WithFileSystem(refFs),
		ssaapi.WithLanguage(ssaconfig.PHP),
	)
	require.NoError(t, err)
	require.NotNil(t, config)

	start := time.Now()
	ch := config.GetFileHandler(
		refFs, fileList, fileMap,
	)

	for fileContent := range ch {
		log.Errorf("file parse: %s: size[%s] time: %s", fileContent.Path, ssaapi.Size(len(fileContent.Content)), fileContent.Duration)
		if budget := phpFixtureParseBudget(); budget > 0 && fileContent.Err == nil && fileContent.Duration > budget {
			slowCandidates = append(slowCandidates, phpProjectSlowCandidate{
				Path:       fileContent.Path,
				Content:    append([]byte(nil), fileContent.Content...),
				Concurrent: fileContent.Duration,
			})
		}
		if fileContent.Err != nil {
			errorFiles[fileContent.Path] = fileContent
		}
	}
	end := time.Since(start)
	log.Infof("Total parse %d files cost: %v", len(fileMap), end)
	failedFiles := make([]string, 0, len(errorFiles))
	for fname, fc := range errorFiles {
		failedFiles = append(failedFiles, fname)
		log.Errorf("Parse file %s failed: %v", fname, fc.Err)
	}
	if budget := phpFixtureParseBudget(); budget > 0 {
		for _, candidate := range slowCandidates {
			if hasSavedProjectSyntaxFixture(path, candidate.Path) {
				log.Infof("skip saved project slow fixture: %s/%s", filepath.Base(path), candidate.Path)
				continue
			}
			start := time.Now()
			_, isolatedErr := php2ssa.Frontend(string(candidate.Content), newPHPTestAntlrCache())
			isolatedDuration := time.Since(start)
			if isolatedErr != nil {
				failedFiles = append(failedFiles, candidate.Path)
				log.Errorf("isolated parse failed for %s after concurrent slow-path detection: %v", candidate.Path, isolatedErr)
				continue
			}
			if isolatedDuration > budget {
				if isKnownLargeProjectSlowFile(path, candidate.Path) {
					deferredSlowFiles = append(deferredSlowFiles, candidate.Path)
					log.Warnf("deferred large-file budget miss for %s: concurrent=%s isolated=%s budget=%s", candidate.Path, candidate.Concurrent, isolatedDuration, budget)
					continue
				}
				slowFiles = append(slowFiles, candidate.Path)
				log.Errorf("isolated parse exceeded budget for %s: concurrent=%s isolated=%s budget=%s", candidate.Path, candidate.Concurrent, isolatedDuration, budget)
			}
		}
	}
	sort.Strings(failedFiles)
	sort.Strings(deferredSlowFiles)
	sort.Strings(slowFiles)
	if len(deferredSlowFiles) > 0 {
		log.Warnf("deferred large slow files for %s: %v", path, deferredSlowFiles)
	}
	require.Empty(t, failedFiles, "project AST parse failed for %d files under %s: %v", len(failedFiles), path, failedFiles)
	require.Empty(t, slowFiles, "project AST parse exceeded budget for %d files under %s: %v", len(slowFiles), path, slowFiles)
}
