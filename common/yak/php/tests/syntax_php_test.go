package tests

import (
	"embed"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

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

func TestExpressionAllInONE(t *testing.T) {
	code := `<?php
// Clone expression
$originalObject = new stdClass;
$clonedObject = clone $originalObject;

// New expression
$newObject = new stdClass;

// Variable name expression
$varName = "test";

// Variable expression and dynamic variable expression
$identifier = "dynamicVar";
$$identifier = "Hello, dynamic!";

// Array creation expression
$array = [1, 2, 3];
$associativeArray = ["a" => 1, "b" => 2];

// Print expression
print "Hello, world!\n";

// Scalar expressions
$constant = PHP_INT_MAX;
$string = "string";
$label = true;

// BackQuoteString expression
$shellOutput = ` + "`ls -al`" + `;

// Parenthesis expression
$parenthesis = (5 * 3) + 2;

// Yield expression
function generator() {
    yield 1;
    yield 2;
}

// Special word expressions (List, IsSet, Empty, Eval, Exit, Include, Require)
list($a, $b) = [1, 2];
$issetExample = isset($a);
$emptyExample = empty($nonexistent);
eval('$evalResult = "Evaluated";');
// Exit; // Uncommenting this will stop the script
include 'nonexistentfile.php'; // Warning suppressed with @
require 'anothernonexistentfile.php'; // Warning suppressed with @

// Lambda function expression
$lambda = function($x) { return $x * 2; };
echo $lambda(5);

// Match expression (PHP 8+)
$matchResult = match($a) {
    1 => 'one',
    2 => 'two',
    default => 'other',
};

// Cast expression
$casted = (int) "123";

// Unary operator expression
$unary = ~$a; // bitwise NOT
$negation = !$issetExample;

// Arithmetic expression
$sum = 1 + 2;
$product = 2 * $a;
$exponentiation = 2 ** 3;

// InstanceOf expression
$instanceOfExample = $newObject instanceof stdClass;

// Bitwise expressions
$and = $a & $b;
$or = $a | $b;
$xor = $a ^ $b;

// Logical expressions
$logicalAnd = $a && $b;
$logicalOr = $a || $b;
$logicalXor = $a xor $b; // Note: 'xor' is not as commonly used

// Conditional expression
$ternary = $a == 2 ? "equals 2" : "does not equal 2";

// Null coalescing expression
$nullCoalesce = $undefinedVar ?? "default";

// Spaceship expression
$spaceship = 1 <=> 2;

// Throw expression (PHP 7.1+)
// throw new Exception("This is an exception");

// Assignment operator expression
$a += 5;

// LogicalAnd, LogicalOr, LogicalXor
$logicalAndSimple = $a and $b;
$logicalOrSimple = $a or $b;
$logicalXorSimple = $a xor $b;

echo "Script execution completed.\n";
`
	_ = code
	// ssatest.MockSSA(t, code)
}

func TestValidatePHPHereDoc(t *testing.T) {
	validateSource(t, "", `<?php


	$abb = <<<EOF
Hello World
EOF."CCCCCCCC";



?>
`)
}

func TestValidatePHPHereDoc_1(t *testing.T) {
	validateSource(t, "", `<?php
 $aaa=<<<EOT 
ac
EOT;
`)
}

func TestValidatePHPHereWithVariableDoc(t *testing.T) {
	validateSource(t, "", `<?php
	$var=<<<EOF
Hello World $var1
EOF;
?>
`)
}
