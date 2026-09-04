package php2ssa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

)

// TestFrontendParseCorpus parses every .php file under the directory named by
// YAK_PHP_PARSE_CORPUS (recursively) and reports per-file syntax error counts
// plus parse timings. It is a manual reproduction/benchmark harness for
// real-world corpora such as php-src stub files; it is skipped when the env
// var is unset so CI does not depend on external files.
func TestFrontendParseCorpus(t *testing.T) {
	root := os.Getenv("YAK_PHP_PARSE_CORPUS")
	if root == "" {
		t.Skip("YAK_PHP_PARSE_CORPUS not set; skipping external corpus parse")
	}

	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if ext := filepath.Ext(path); ext == ".php" || ext == ".inc" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no php files found under corpus root")
	}

	totalErrors := 0
	totalMs := int64(0)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		start := time.Now()
		ast, parseErr := Frontend(string(src), nil)
		elapsed := time.Since(start)
		totalMs += elapsed.Milliseconds()

		errCount := 0
		if parseErr != nil {
			errCount = strings.Count(parseErr.Error(), "reason:")
		}
		totalErrors += errCount
		rel, _ := filepath.Rel(root, f)
		t.Logf("parse %-45s %8d bytes %8d ms  errors=%d", filepath.ToSlash(rel), len(src), elapsed.Milliseconds(), errCount)
		if parseErr != nil && os.Getenv("YAK_PHP_PARSE_DEBUG") != "" {
			for _, line := range strings.Split(strings.TrimSpace(parseErr.Error()), "\n") {
				if i := strings.Index(line, "reason:"); i >= 0 {
					t.Logf("  %s", line[i:])
				}
			}
		}
		if ast == nil {
			t.Errorf("file %s produced nil AST", rel)
		}
	}
	t.Logf("TOTAL files=%d errors=%d time=%dms", len(files), totalErrors, totalMs)
	if totalErrors > 0 {
		t.Errorf("corpus has %d syntax errors", totalErrors)
	}
}

// TestFrontendModernPHPSyntax keeps inline regression cases for the modern
// PHP constructs that php-src master relies on (PHP 8.2 readonly classes,
// PHP 8.3 typed class constants, PHP 8.4 asymmetric visibility, keyword
// method names, foreach array destructuring). Each snippet must parse with
// zero syntax errors.
func TestFrontendModernPHPSyntax(t *testing.T) {
	cases := map[string]string{
		"final readonly class": `
<?php
final readonly class Duration
{
    public readonly int $seconds;
    public function __construct(int $seconds) {}
}
`,
		"readonly class": `
<?php
readonly class Point
{
    public int $x;
    public int $y;
}
`,
		"abstract readonly class": `
<?php
abstract readonly class Shape
{
    public int $sides;
}
`,
		"nested readonly class in namespace": `
<?php
namespace Foo\Bar {
    final readonly class Number implements \Stringable
    {
        public function floor(): Number {}
    }
}
`,
		"asymmetric visibility": `
<?php
class Node
{
    public private(set) string $name;
    protected public(set) int $count;
    public private(set) ?string $internalSubset;
}
`,
		"keyword method name throw": `
<?php
final class Fiber
{
    public function throw(Throwable $exception): mixed {}
    public static function suspend(mixed $value = null): mixed {}
}
`,
		"set_include_path function": `
<?php
function set_include_path(string $include_path): string|false {}
`,
		"typed const with keyword name": `
<?php
class SQLite3
{
    public const int FUNCTION = 3;
    public const int SAVEPOINT = 4;
}
`,
		"foreach keyed destructuring": `
<?php
foreach ($curlConstants as $name => [$introduced, $deprecated, $removed]) {
    echo $name;
}
foreach ($notInPHP as $name => [$introduced, $removed]) {
    echo $name;
}
foreach ($data as [$a, $b]) {
    echo $a;
}
foreach ($data as $k => $v) {
    echo $v;
}
`,
		"throw statement still parses": `
<?php
function f() {
    throw new Exception("x");
}
`,
	}

	for name, src := range cases {
		src := src
		t.Run(name, func(t *testing.T) {
			ast, err := Frontend(src, nil)
			if err != nil {
				t.Fatalf("syntax error: %v", err)
			}
			if ast == nil {
				t.Fatalf("nil AST")
			}
		})
	}
}

