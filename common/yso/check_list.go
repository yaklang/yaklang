package yso

import embeddata "github.com/yaklang/yaklang/common/yso/embed_data"

// 以下常量通过 embed + XOR 加载，避免二进制中出现明文 Java 反序列化类名。
// 原始值参见 embed_data/check_list.json。

var allGadgetsCheckList = embeddata.LoadCheckList()

func GetGadgetChecklist() map[string]string {
	return allGadgetsCheckList
}
