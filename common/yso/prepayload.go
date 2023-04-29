package yso

import (
	"fmt"
	"yaklang/common/yak/yaklib/codec"
)

func BashCmdWrapper(cmd string) string {
	return fmt.Sprintf("bash -c {echo,%v}|{base64,-d}|{bash,-i}", codec.EncodeBase64(cmd))
}

func PowerShellCmdWrapper(cmd string) string {
	return fmt.Sprintf("powershell.exe -NonI -W Hidden -NoP -Exec Bypass -Enc %v", codec.EncodeBase64(cmd))
}

func PythonCmdWrapper(cmd string) string {
	return fmt.Sprintf("python -c exec('%v'.decode('base64'))", codec.EncodeBase64(cmd))
}

func PerlCmdWrapper(cmd string) string {
	return fmt.Sprintf("perl -MMIME::Base64 -e eval(decode_base64('%v'))", codec.EncodeBase64(cmd))
}

func ClojureCmdWrapper(cmd string) string {
	return fmt.Sprintf(`(use '[clojure.java.shell :only [sh]]) (sh \"%v\")`, cmd)
}

func AllCmdWrapper(cmd string) []string {
	var res []string
	res = append(res, BashCmdWrapper(cmd))
	res = append(res, PythonCmdWrapper(cmd))
	res = append(res, PowerShellCmdWrapper(cmd))
	res = append(res, PerlCmdWrapper(cmd))
	return res
}
