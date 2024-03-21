package tests

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func TestParseSSA_BasicMember(t *testing.T) {
	t.Run("slice normal", func(t *testing.T) {
		test.MockSSA(t, `<?php
		$c=[1,2,3];
		dump($c[2]);
		echo 1,2,3,5;
		`)
	})
}
