package tests

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssareducer"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
)

//go:embed syntax/***
var syntaxFs embed.FS

func validateSource(t *testing.T, filename string, src string) {
	t.Run(fmt.Sprintf("syntax file: %v", filename), func(t *testing.T) {
		errListener := antlr4util.NewErrorListener()
		lexer := phpparser.NewPHPLexer(antlr.NewInputStream(src))
		lexer.RemoveErrorListeners()
		lexer.AddErrorListener(errListener)
		tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
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

		if errListener.GetErrorString() != "" {
			t.Fatalf("Lexer failed: %v", errListener.GetErrorString())
		}
		spew.Dump(errListener.GetErrors())

		_, err := php2ssa.Frontend(src)
		require.Nil(t, err, "parse AST FrontEnd error: %v", err)
	})
}

func TestAllSyntaxForPHP_G4(t *testing.T) {
	entry, err := syntaxFs.ReadDir("syntax")
	if err != nil {
		t.Fatalf("no embed syntax files found: %v", err)
	}
	for _, f := range entry {
		if f.IsDir() {
			continue
		}
		syntaxPath := path.Join("syntax", f.Name())
		if !strings.HasSuffix(syntaxPath, ".php") {
			continue
		}
		raw, err := syntaxFs.ReadFile(syntaxPath)
		if err != nil {
			t.Fatalf("cannot found syntax fs: %v", syntaxPath)
		}
		//ssatest.MockSSA(t, string(raw))
		validateSource(t, syntaxPath, string(raw))
	}
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

type ParseError struct {
	Duration time.Duration
	Message  string
}

func TestProjectAst(t *testing.T) {
	path := "/home/wlz/Developer/pfsense"

	errorFiles := make(map[string]*ssareducer.FileContent)

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
				fileList = append(fileList, filePath)
				fileMap[filePath] = struct{}{}
				return nil
			}
			return nil
		}),
	)
	log.Errorf("file to parse: %+v", fileList)

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
		if fileContent.Err != nil {
			errorFiles[fileContent.Path] = fileContent
		}
	}
	end := time.Since(start)
	log.Infof("Total parse %d files cost: %v", len(fileMap), end)
	for fname, fc := range errorFiles {
		log.Errorf("Parse file %s failed: %v", fname, fc.Err)
	}
}
