package php

import (
	"fmt"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

func TestMiscSyntaxPrompt(t *testing.T) {
	_, err := ssaapi.Parse(`<?php
echo 1
1+1

`, ssaapi.WithLanguage(ssaapi.PHP))
	if err != nil {
		fmt.Println(err)
	}
}
