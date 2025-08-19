package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/cmd/aireactdeps/promptui"
)

func main() {
	prompt := promptui.Prompt{
		Label:     "Delete Resource",
		IsConfirm: true,
	}

	result, err := prompt.Run()

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return
	}

	fmt.Printf("You choose %q\n", result)
}
