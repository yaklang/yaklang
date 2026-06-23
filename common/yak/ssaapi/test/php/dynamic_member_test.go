package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPHP_DynamicMemberCall(t *testing.T) {
	code := `<?php
class ParsedownMini
{
    protected function blockHeader($Line)
    {
        println("header");
    }

    protected function linesElements(array $lines)
    {
        $blockTypes = array('Header');
        foreach ($lines as $line)
        {
            foreach ($blockTypes as $blockType)
            {
                $this->{"block$blockType"}($line);
            }
        }
    }

    protected function handle(array $Element)
    {
        $function = $Element['handler']['function'];
        $argument = $Element['handler']['argument'];
        return $this->$function($argument);
    }
}

$parser = new ParsedownMini();
$parser->linesElements(array('# title'));
$parser->handle(array(
    'handler' => array(
        'function' => 'blockHeader',
        'argument' => 'arg',
    ),
));
`
	ssatest.CheckSyntaxFlowPrintWithPhp(t, code, []string{"header", "header"})
}

func TestPHP_DynamicMemberCallCompileStability(t *testing.T) {
	code := `<?php
class ParsedownMini
{
    protected function blockHeader($Line)
    {
        println("header");
    }

    protected function linesElements(array $lines)
    {
        foreach ($lines as $line)
        {
            $blockTypes = array('Header');
            foreach ($blockTypes as $blockType)
            {
                $this->{"block$blockType"}($line);
            }
        }
    }
}
$parser = new ParsedownMini();
$parser->linesElements(array('# title'));
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}
