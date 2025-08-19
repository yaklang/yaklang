package main

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/aireactdeps"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/aireactdeps/promptui"
)

func main() {
	promptui.NewCursor("你好？", promptui.BlockCursor, false)
	aireactdeps.MainEntry()
}
