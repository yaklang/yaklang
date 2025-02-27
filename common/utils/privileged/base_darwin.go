package privileged

import (
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
)

func isPrivileged() bool {
	return os.Geteuid() == 0
}

type Executor struct {
	AppName       string
	AppIcon       string
	DefaultPrompt string
}

func NewExecutor(appName string) *Executor {
	return &Executor{
		AppName:       appName,
		DefaultPrompt: "此操作需要管理员权限",
	}
}

func (p *Executor) Execute(opts ExecuteOptions) ([]byte, error) {
	if opts.Title == "" {
		opts.Title = p.AppName
	}
	if opts.Prompt == "" {
		opts.Prompt = p.DefaultPrompt
	}

	script := fmt.Sprintf(`
on decodeHex(hexString)
    set cleanHex to ""
    repeat with char in hexString
        set charCode to (id of char) as integer
        if (charCode ≥ 48 and charCode ≤ 57) or ¬
           (charCode ≥ 65 and charCode ≤ 70) or ¬
           (charCode ≥ 97 and charCode ≤ 102) then
            set cleanHex to cleanHex & (ASCII character charCode)
        end if
    end repeat
    set cleanHex to do shell script "echo " & quoted form of cleanHex & " | tr '[:lower:]' '[:upper:]'"
    
    if (length of cleanHex) mod 2 ≠ 0 then
        error "无效的HEX字符串：清理后长度为奇数"
    end if
    
    set byteList to {}
    repeat with i from 1 to (length of cleanHex) by 2
        set pair to text i thru (i + 1) of cleanHex
        set highNibble to decodeHexChar(first character of pair)
        set lowNibble to decodeHexChar(second character of pair)
        set end of byteList to (highNibble * 16) + lowNibble
    end repeat
    
    set outputText to ""
    repeat with byteValue in byteList
        try
            set outputText to outputText & (ASCII character byteValue)
        on error
            set outputText to outputText & ("�") -- 替换无效字符
        end try
    end repeat
    
    return outputText
end decodeHex

on decodeHexChar(c)
    set charCode to (id of c) as integer
    if charCode ≤ 57 then -- 0-9
        return charCode - 48
    else -- A-F
        return charCode - 55
    end if
end decodeHexChar


set titleString to decodeHex("%s")
set dialogString to decodeHex("%s")
set cmd to decodeHex("%s")
set promptString to decodeHex("%s")

tell application "System Events"
	display dialog dialogString with title titleString buttons {"取消", "确定"} default button "确定" with icon caution
	if button returned of result is "确定" then
		do shell script cmd with administrator privileges with prompt promptString
	else
		error "用户取消操作"
	end if
end tell`, hex.EncodeToString([]byte(opts.Title)), hex.EncodeToString([]byte(opts.Description)), hex.EncodeToString([]byte(opts.Command)), hex.EncodeToString([]byte(opts.Prompt)))

	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("privileged execution failed: %v, output: %s", err, output)
	}

	return output, nil
}
