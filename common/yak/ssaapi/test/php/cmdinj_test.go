package php

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPHP_CMDInj(t *testing.T) {
	code := `<?php
$command = 'ping -c 1 '.$_GET['ip'];
system($command); //system函数特性 执行结果会自动打印
?>`

	// TODO: handler extern-function
	ssatest.CheckSyntaxFlow(t, code,
		`system( * as $command)`,
		map[string][]string{
			"command": {`add("ping -c 1 ", ParameterMember-parameter[0].'ip')`},
		},
		ssaapi.WithLanguage(ssaapi.PHP),
	)
}
