//go:build no_language
// +build no_language

package yakcmds

import "github.com/urfave/cli"

// Stub implementations for Java utils when language support is excluded

// JavaUtils 桩实现 - no_language 版本返回空命令列表
var JavaUtils = []*cli.Command{}
