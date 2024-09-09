package tests

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

//go:embed bad_doc.php
var badDocPHP string

func TestBadDoc(t *testing.T) {
	validateSource(t, "bad_doc.php", badDocPHP)
}

//go:embed syntax/bad_qrcode.php
var qrcode string

// todo: 待修复
//func TestBadQrcode(t *testing.T) {
//	ssatest.Check(t, qrcode, func(prog *ssaapi.Program) error {
//		prog.Show()
//		return nil
//	}, ssaapi.WithLanguage(ssaapi.PHP))
//}

//go:embed bad/badFile_panic1.php
var bad_panic string

func TestBadPanic(t *testing.T) {
	ssatest.Check(t, bad_panic, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}

//go:embed bad/badFile_panic2.php
var bad_panic2 string

func TestBadPanic2(t *testing.T) {
	ssatest.Check(t, bad_panic2, func(prog *ssaapi.Program) error {
		prog.Show()
		return nil
	}, ssaapi.WithLanguage(ssaapi.PHP))
}
