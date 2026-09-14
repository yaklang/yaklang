package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/php/php2ssa"
)

// TestFrontendModernPHPSyntax keeps inline regression cases for the modern
// PHP constructs that php-src master relies on (PHP 8.2 readonly classes,
// PHP 8.3 typed class constants, PHP 8.4 asymmetric visibility, keyword
// method names, foreach array destructuring). Each snippet must parse with
// zero syntax errors. Snippets MUST start with "<?php" so the lexer actually
// enters PHP mode — without it everything is swallowed as inline HTML and
// the test would pass vacuously.
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
			ast, err := php2ssa.Frontend(src, nil)
			require.NoError(t, err, "syntax error")
			require.NotNil(t, ast, "nil AST")
		})
	}
}