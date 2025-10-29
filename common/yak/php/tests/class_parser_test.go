package tests

import (
	_ "embed"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed smtp-demo.php
var phpCase string

func TestSyntaxForClass(t *testing.T) {
	ssatest.Check(t, phpCase, func(prog *ssaapi.Program) error {
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}

func TestSyntaxForClass_SelfDoubleColon(t *testing.T) {
	ssatest.Check(t, `
<?php

// $this->{$kind}[] = [$address, $name];

$this->ReplyTo[strtolower($address)] = [$address, $name];

\array_keys;

\array_keys($allowedOptions);

$allowedOptions = \array_keys($allowedOptions);

class SMTP
{
    const VERSION = self::DEBUG_OFF;
}
`, func(prog *ssaapi.Program) error {
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}
