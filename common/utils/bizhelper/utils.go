package bizhelper

import (
	"fmt"
	"strings"

	"github.com/segmentio/ksuid"
)

// fixCustomFileName 修复自定义文件名
// 如果文件名为空，则生成一个随机的JSON文件名
// 如果文件名没有.json后缀，则添加.json后缀
// name: 原始文件名
// 返回: 修复后的文件名
func fixCustomFileName(name string) string {
	if name == "" {
		name = fmt.Sprintf("%s.json", ksuid.New().String())
	} else if !strings.HasSuffix(name, ".json") {
		name = fmt.Sprintf("%s.json", name)
	}
	return name
}
