package php

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCFG(t *testing.T) {
	t.Run("test condition1", func(t *testing.T) {
		code := `<?php
	$data = $_POST['data'] ??"aa";
	println($data);
	`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			result, err := prog.SyntaxFlowWithError(`println(* #-> * as $param)`)
			if err != nil {
				return err
			}
			values := result.GetValues("param")
			require.Contains(t, values.String(), "aa")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
	t.Run("check no variables declare", func(t *testing.T) {
		code := `<?php
$a = $a??12312;
println($a);`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			result, err := prog.SyntaxFlowWithError(`println(* #-> * as $param)`)
			if err != nil {
				return err
			}
			values := result.GetValues("param")
			require.Contains(t, values.String(), "12312")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.PHP))
	})
}
func TestCfgAssignment(t *testing.T) {
	code := `<?php
if ($_SERVER['REQUEST_METHOD'] === 'POST' && isset($_FILES['image'])) {
    $uploaded = $_FILES['image'];
    
    $encoded_name = pathinfo($uploaded['name'], PATHINFO_FILENAME);
    $hex = '';
    for ($i = 0; $i < strlen($encoded_name); $i += 2) {
        $hex .= chr(hexdec(substr($encoded_name, $i, 2)));
    }
    $decoded_name = '';
    foreach (str_split($hex) as $char) {
        $decoded_name .= chr(ord($char) ^ 0xAA);
    }
    $extension = pathinfo($uploaded['name'], PATHINFO_EXTENSION);
    
    $encoded_content = file_get_contents($uploaded['tmp_name']);
    $decoded_content = '';
    for ($i = 0; $i < strlen($encoded_content); $i++) {
        $decoded_content .= chr(ord($encoded_content[$i]) ^ 0xAA);
    }
    
    $final_name = $decoded_name . ($extension ? ".$extension" : '');
    file_put_contents($final_name, $decoded_content);
    echo "File decoded successfully: $final_name";
} else {
    echo "Please upload file using POST with 'image' parameter";
}
?>`
	ssatest.CheckSyntaxFlow(t, code, `
_FILES.* as $param
file_put_contents(,* #{
	include: <<<CODE
* & $param
CODE
}-> as $sink)
`,
		map[string][]string{
			"sink": {"Undefined-_FILES"},
		},
		ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestTypeCast(t *testing.T) {
	code := `<?php
$a = (int)$_GET[1];
echo($a);
`
	ssatest.CheckSyntaxFlow(t, code, `
_GET as $source
echo?(* #{
include: <<<INCLUDE
* & $source
INCLUDE,
exclude: <<<EXCLUDE
*?{opcode: typecast}
EXCLUDE
}->) as $sink
`, map[string][]string{"sink": {}}, ssaapi.WithLanguage(ssaconfig.PHP))
}
