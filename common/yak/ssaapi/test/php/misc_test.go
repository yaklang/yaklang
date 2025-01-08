package php

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestSearchMember(t *testing.T) {
	code := `<?php
class xx{
	public $a = new BB();
}
$xx = new xx();
$xx->$a->cc();
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		result := prog.SyntaxFlow(`BB.cc() as $sink`)
		assert.True(t, result.GetValues("sink").Len() != 0)
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
