package php

import (
	_ "embed"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed UploadFile.class.php
var uploadCase1 string

func TestUploadParsing(t *testing.T) {
	name := uuid.New().String()
	prog, err := ssaapi.Parse(uploadCase1, ssaapi.WithLanguage(ssaconfig.PHP), ssaapi.WithProgramName(name))
	if err != nil {
		t.Fatal(err)
	}
	_ = prog
	prog, err = ssaapi.FromDatabase(name)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, prog.SyntaxFlowChain("move* as $a").Show().Len())
}

func TestUploadParsingPart2(t *testing.T) {
	code := `<?php
move_uploaded_file($file['tmp_name'], auto_charset($filename,'utf-8','gbk'));`
	ssatest.CheckSyntaxFlow(t, code,
		`move_uploaded_file as $target`,
		map[string][]string{
			"target": {"Undefined-move_uploaded_file"},
		},
		ssaapi.WithLanguage(ssaconfig.PHP),
	)
}

func TestUploadParsingPart1(t *testing.T) {
	code := `      <?php

if(!move_uploaded_file($file['tmp_name'], auto_charset($filename,'utf-8','gbk'))) {
            $this->error = '文件上传保存错误！';
            return false;
        }`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		if prog.SyntaxFlowChain("move_uploaded_file as $param").Show().Len() != 1 {
			t.Fatal("compiling failed")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP))
}
