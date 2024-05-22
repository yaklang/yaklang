package php

import (
	_ "embed"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"
)

//go:embed UploadFile.class.php
var uploadCase1 string

func TestUploadParsing(t *testing.T) {
	name := uuid.New().String()
	prog, err := ssaapi.Parse(uploadCase1, ssaapi.WithLanguage(ssaapi.PHP), ssaapi.WithDatabaseProgramName(name))
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
	prog, err := ssaapi.Parse(`      <?php
move_uploaded_file($file['tmp_name'], auto_charset($filename,'utf-8','gbk'));
`, ssaapi.WithLanguage(ssaapi.PHP))
	if err != nil {
		t.Fatal(err)
	}
	if prog.SyntaxFlowChain("move_uploaded_file as $param").Show().Len() != 1 {
		t.Fatal("compiling failed")
	}
}

func TestUploadParsingPart1(t *testing.T) {
	prog, err := ssaapi.Parse(`      <?php

if(!move_uploaded_file($file['tmp_name'], auto_charset($filename,'utf-8','gbk'))) {
            $this->error = '文件上传保存错误！';
            return false;
        }`, ssaapi.WithLanguage(ssaapi.PHP))
	if err != nil {
		t.Fatal(err)
	}
	if prog.SyntaxFlowChain("move_uploaded_file as $param").Show().Len() != 1 {
		t.Fatal("compiling failed")
	}
}
