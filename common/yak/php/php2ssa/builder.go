package php2ssa

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type SSABuild struct {
	ssa.DummyExtraFileAnalyzer
}

var Builder = &SSABuild{}

func (*SSABuild) Build(src string, force bool, b *ssa.FunctionBuilder) error {
	ast, err := FrondEnd(src, force)
	if err != nil {
		return err
	}
	build := builder{
		constMap:        make(map[string]ssa.Value),
		FunctionBuilder: b,
	}
	b.WithExternValue(phpBuildIn)
	build.VisitHtmlDocument(ast)
	return nil
}

func (*SSABuild) FilterFile(path string) bool {
	return filepath.Ext(path) == ".php"
}

type builder struct {
	*ssa.FunctionBuilder
	constMap   map[string]ssa.Value
	isFunction bool
}

func FrondEnd(src string, force bool) (phpparser.IHtmlDocumentContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := phpparser.NewPHPLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := phpparser.NewPHPParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.HtmlDocument()
	if force || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrorString())
}

func (b *builder) AssignConst(name string, value ssa.Value) bool {
	if ConstValue, ok := b.constMap[name]; ok {
		log.Warnf("const %v has been defined value is %v", name, ConstValue.String())
		return false
	}

	b.constMap[name] = value
	return true
}

func (b *builder) ReadConst(name string) (ssa.Value, bool) {
	v, ok := b.constMap[name]
	return v, ok
}

func (b *builder) AssignClassConst(className, key string, value ssa.Value) {
	name := fmt.Sprintf("%s_%s", className, key)
	b.AssignConst(name, value)
}
func (b *builder) ReadClassConst(className, key string) (ssa.Value, bool) {
	name := fmt.Sprintf("%s_%s", className, key)
	return b.ReadConst(name)
}

var phpBuildIn = map[string]any{
	"$_COOKIE":  map[interface{}]interface{}{},
	"$_SESSION": map[interface{}]interface{}{},
	"$_SERVER":  map[interface{}]interface{}{},
	"$_POST":    map[interface{}]interface{}{},
	"$_GET":     map[interface{}]interface{}{},
	"$_REQUEST": map[interface{}]interface{}{},
	"PHP_EOL":   "",
	"echo":      func(...any) {},
	"println":   func(...any) {},
	"phpinfo":   func() {},
	"strrev": func(value string) string {
		return ""
	},
	"array": func(...any) []any { return nil },
	"system": func(command string, resultCode ...int) (string, bool) {
		return "", false
	},
	"exec": func(command string) (string, bool) {
		return "", false
	},
	"shell_exec":             func() {},
	"abs":                    func(number float64) float64 { return 0 },
	"array_change_key_case":  func(array any, caseConv int) any { return nil },
	"array_chunk":            func(array any, size int, preserveKeys bool) any { return nil },
	"array_column":           func(array any, columnKey string, indexKey string) any { return nil },
	"array_combine":          func(keys, values any) any { return nil },
	"array_count_values":     func(array any) any { return nil },
	"array_diff":             func(arrays ...any) any { return nil },
	"array_diff_assoc":       func(arrays ...any) any { return nil },
	"array_diff_key":         func(arrays ...any) any { return nil },
	"array_diff_uassoc":      func(arrays ...any) any { return nil },
	"array_diff_ukey":        func(arrays ...any) any { return nil },
	"array_fill":             func(startIndex int, num int, value any) any { return nil },
	"array_fill_keys":        func(keys any, value any) any { return nil },
	"array_filter":           func(array any, callback func(value any) bool) any { return nil },
	"array_flip":             func(array any) any { return nil },
	"array_intersect":        func(arrays ...any) any { return nil },
	"array_intersect_assoc":  func(arrays ...any) any { return nil },
	"array_intersect_key":    func(arrays ...any) any { return nil },
	"array_intersect_uassoc": func(arrays ...any) any { return nil },
	"array_intersect_ukey":   func(arrays ...any) any { return nil },
	"array_key_exists":       func(key any, search any) bool { return false },
	"array_keys":             func(input any, searchValue any, strict bool) any { return nil },
	"array_map":              func(callback func(value any) any, arrays ...any) any { return nil },
	"array_merge":            func(arrays ...any) any { return nil },
	"array_merge_recursive":  func(arrays ...any) any { return nil },
	"array_multisort":        func(array any) {},
	"array_pad":              func(array any, size int, value any) any { return nil },
	"array_pop":              func(array any) any { return nil },
	"array_product":          func(array any) float64 { return 0 },
	"array_push":             func(array any, values ...any) int { return 0 },
	"array_rand":             func(array any, num int) any { return nil },
	"array_reduce": func(array any, callback func(accumulator, value any) any) any {
		return nil
	},
	"array_replace":              func(arrays ...any) any { return nil },
	"array_replace_recursive":    func(arrays ...any) any { return nil },
	"array_reverse":              func(array any, preserveKeys bool) any { return nil },
	"array_search":               func(needle any, haystack any, strict bool) any { return nil },
	"array_shift":                func(array any) any { return nil },
	"array_slice":                func(array any, offset int, length int, preserveKeys bool) any { return nil },
	"array_splice":               func(array any, offset int, length int, replacement any) any { return nil },
	"array_sum":                  func(array any) float64 { return 0 },
	"array_udiff":                func(arrays ...any) any { return nil },
	"array_udiff_assoc":          func(arrays ...any) any { return nil },
	"array_udiff_uassoc":         func(arrays ...any) any { return nil },
	"array_uintersect":           func(arrays ...any) any { return nil },
	"array_uintersect_assoc":     func(arrays ...any) any { return nil },
	"array_uintersect_uassoc":    func(arrays ...any) any { return nil },
	"array_unique":               func(array any) any { return nil },
	"array_unshift":              func(array any, values ...any) int { return 0 },
	"array_values":               func(array any) any { return nil },
	"array_walk":                 func(array any, callback func(value any, key any) bool) {},
	"arsort":                     func(array any) {},
	"asort":                      func(array any) {},
	"compact":                    func(vars ...string) any { return nil },
	"count":                      func(variable any, mode int) int { return 0 },
	"current":                    func(array any) any { return nil },
	"each":                       func(array any) any { return nil },
	"end":                        func(array any) any { return nil },
	"extract":                    func(array any, extractFlags int, prefix string) int { return 0 },
	"in_array":                   func(needle any, haystack any, strict bool) bool { return false },
	"key":                        func(array any) any { return nil },
	"krsort":                     func(array any) {},
	"ksort":                      func(array any) {},
	"list":                       func(vars ...any) {},
	"natcasesort":                func(array any) {},
	"natsort":                    func(array any) {},
	"next":                       func(array any) any { return nil },
	"pos":                        func(array any) any { return nil },
	"prev":                       func(array any) any { return nil },
	"range":                      func(start float64, end float64, step float64) any { return nil },
	"reset":                      func(array any) any { return nil },
	"rsort":                      func(array any) {},
	"shuffle":                    func(array any) bool { return false },
	"sizeof":                     func(variable any, mode int) int { return 0 },
	"sort":                       func(array any) {},
	"uasort":                     func(array any, callback func(value1, value2 any) int) {},
	"uksort":                     func(array any, callback func(key1, key2 any) int) {},
	"usort":                      func(array any, callback func(value1, value2 any) int) {},
	"bin2hex":                    func(str string) string { return "" },
	"chop":                       func(str string, charlist string) string { return "" },
	"chr":                        func(code int) string { return "" },
	"chunk_split":                func(body string, chunklen int, end string) string { return "" },
	"convert_cyr_string":         func(str string, from string, to string) string { return "" },
	"convert_uudecode":           func(str string) string { return "" },
	"convert_uuencode":           func(str string) string { return "" },
	"count_chars":                func(str string, mode int) any { return nil },
	"crc32":                      func(str string) int { return 0 },
	"explode":                    func(delimiter string, str string, limit int) []string { return nil },
	"fprintf":                    func(resource any, format string, args ...any) int { return 0 },
	"get_html_translation_table": func(table int, quote_style int) any { return nil },
	"hebrev":                     func(str string) string { return "" },
	"hebrevc":                    func(str string) string { return "" },
	"hex2bin":                    func(hexString string) string { return "" },
	"html_entity_decode":         func(html string, quoteStyle int, charset string) string { return "" },
	"htmlentities":               func(str string, quoteStyle int, charset string, doubleEncode bool) string { return "" },
	"htmlspecialchars_decode":    func(str string, quoteStyle int) string { return "" },
	"htmlspecialchars":           func(str string, quoteStyle int, charset string, doubleEncode bool) string { return "" },
	"implode":                    func(glue string, pieces any) string { return "" },
	"join":                       func(glue string, pieces any) string { return "" },
	"levenshtein":                func(str1 string, str2 string, cost int) int { return 0 },
	"localeconv":                 func() any { return nil },
	"ltrim":                      func(str string, charlist string) string { return "" },
	"md5_file":                   func(filename string, rawOutput bool) string { return "" },
	"md5":                        func(str string, rawOutput bool) string { return "" },
	"metaphone":                  func(str string, maxPhonemes int) string { return "" },
	"money_format":               func(format string, number float64) string { return "" },
	"nl_langinfo":                func(item int) string { return "" },
	"number_format":              func(number float64, decimals int, decimalPoint string, thousandsSeparator string) string { return "" },
	"ord":                        func(str string) int { return 0 },
	"parse_str":                  func(str string, result any) {},
	"print":                      func(args ...any) {},
	"printf":                     func(format string, args ...any) int { return 0 },
	"quotemeta":                  func(str string) string { return "" },
	"rtrim":                      func(str string, charlist string) string { return "" },
	"setlocale":                  func(category int, locale string) string { return "" },
	"sha1_file":                  func(filename string, rawOutput bool) string { return "" },
	"sha1":                       func(str string, rawOutput bool) string { return "" },
	"similar_text":               func(str1 string, str2 string, percent *float64) int { return 0 },
	"soundex":                    func(str string) string { return "" },
	"sprintf":                    func(format string, args ...any) string { return "" },
	"sscanf":                     func(str string, format string, vars ...any) int { return 0 },
	"str_getcsv":                 func(str string, delimiter string, enclosure string, escape string) [][]string { return nil },
	"str_ireplace":               func(search string, replace string, subject string, count *int) string { return "" },
	"str_pad":                    func(input string, padLength int, padString string, padType int) string { return "" },
	"str_repeat":                 func(input string, multiplier int) string { return "" },
	"str_replace":                func(search string, replace string, subject string, count *int) string { return "" },
	"str_rot13":                  func(str string) string { return "" },
	"str_shuffle":                func(str string) string { return "" },
	"str_split":                  func(str string, splitLength int) []string { return nil },
	"str_word_count":             func(str string, format int, charlist string) any { return nil },
	"strcasecmp":                 func(str1 string, str2 string) int { return 0 },
	"strchr":                     func(haystack string, needle string) string { return "" },
	"strcmp":                     func(str1 string, str2 string) int { return 0 },
	"strcoll":                    func(str1 string, str2 string) int { return 0 },
	"strcspn":                    func(str1 string, str2 string, start int, length int) int { return 0 },
	"strip_tags":                 func(str string, allowed string) string { return "" },
	"stripcslashes":              func(str string) string { return "" },
	"stripos":                    func(haystack string, needle string, offset int) int { return 0 },
	"stristr":                    func(haystack string, needle string, beforeNeedle bool) string { return "" },
	"strlen":                     func(str string) int { return 0 },
	"strnatcasecmp":              func(str1 string, str2 string) int { return 0 },
	"strnatcmp":                  func(str1 string, str2 string) int { return 0 },
	"strpbrk":                    func(str1 string, str2 string) string { return "" },
	"strpos":                     func(haystack string, needle string, offset int) int { return 0 },
	"strrchr":                    func(haystack string, needle string) string { return "" },
	"strripos":                   func(haystack string, needle string, offset int) int { return 0 },
	"strrpos":                    func(haystack string, needle string, offset int) int { return 0 },
	"strspn":                     func(str1 string, str2 string, start int, length int) int { return 0 },
	"strstr":                     func(haystack string, needle string, beforeNeedle bool) string { return "" },
	"strtok":                     func(str string, token string) string { return "" },
	"strtolower":                 func(str string) string { return "" },
	"strtoupper":                 func(str string) string { return "" },
	"strtr":                      func(str string, from string, to string) string { return "" },
	"substr_compare":             func(mainStr string, str string, offset int, length int, caseInsensitivity bool) int { return 0 },
	"basename":                   func(path string, suffix string) string { return "" },
	"chgrp":                      func(file string, group string) error { return nil },
	"chmod":                      func(file string, mode int) error { return nil },
	"chown":                      func(file string, user string) error { return nil },
	"copy":                       func(from string, to string) error { return nil },
	"dir":                        func(path string) ([]os.FileInfo, error) { return nil, nil },
	"dirname":                    func(path string) string { return "" },
	"fclose":                     func(file *os.File) error { return nil },
	"fgetc":                      func(file *os.File) (int, error) { return 0, nil },
	"fgets":                      func(file *os.File, n int) (string, error) { return "", nil },
	"file":                       func(filename string) ([]byte, error) { return nil, nil },
	"file_exists":                func(filename string) bool { return false },
	"file_get_contents":          func(filename string) (string, error) { return "", nil },
	"file_put_contents":          func(filename string, data []byte) error { return nil },
	"filesize":                   func(filename string) (int64, error) { return 0, nil },
	"filetype":                   func(filename string) (string, error) { return "", nil },
	"glob":                       func(pattern string) ([]string, error) { return []string{}, nil },
	"is_dir":                     func(filename string) bool { return false },
	"is_executable":              func(filename string) bool { return false },
	"is_file":                    func(filename string) bool { return false },
	"is_readable":                func(filename string) bool { return false },
	"is_uploaded_file":           func(filename string) bool { return false },
	"is_writable":                func(filename string) bool { return false },
	"move":                       func(from string, to string) error { return nil },
	"parse_ini_file": func(filename string, processSections bool) (map[string]map[string]interface{}, error) {
		return nil, nil
	},
	"pathinfo":  func(path string) (map[string]string, error) { return nil, nil },
	"realpath":  func(path string) (string, error) { return "", nil },
	"rename":    func(oldName string, newName string) error { return nil },
	"rmdir":     func(path string) error { return nil },
	"scandir":   func(path string) ([]os.FileInfo, error) { return nil, nil },
	"serialize": func(value ssa.Value) string { return "" },
	"unserialize": func(raw string) ssa.Value {
		return ssa.NewNil()
	},
	"eval":   func(code interface{}) {},
	"assert": func(code interface{}) {},
}
