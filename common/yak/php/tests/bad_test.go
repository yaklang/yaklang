package tests

import (
	_ "embed"
	"testing"
)

//go:embed bad_doc.php
var badDocPHP string

func TestBadDoc(t *testing.T) {
	validateSource(t, "bad_doc.php", badDocPHP)
}

//go:embed syntax/bad_qrcode.php
var qrcode string

//todo: 待修复
//func TestBadQrcode(t *testing.T) {
//	ssatest.Check(t, qrcode, func(prog *ssaapi.Program) error {
//		prog.Show()
//		return nil
//	}, ssaapi.WithLanguage(ssaapi.PHP))
//}
