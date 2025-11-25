package yso

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yserx"
)

func TestReplaceByteArrayInJavaSerializable(t *testing.T) {
	/*
		HashMap<Object, Object>
		├── "className" → {{param0}}
		├── "bytes" → {{param1}}
		└── "students" → Student[]
			├── [0] → Student
			│   ├── name: "zhangsan"
			│   └── age: 18
			└── [1] → Student
				├── name: "lisi"
				└── age: 20
	*/
	exceptClassName := "org.example.Evil"
	exceptClassBytes := []byte("3d674b65-881a-49ec-86e1-e7bd6234298b")
	serialBase64 := "rO0ABXNyABFqYXZhLnV0aWwuSGFzaE1hcAUH2sHDFmDRAwACRgAKbG9hZEZhY3RvckkACXRocmVzaG9sZHhwP0AAAAAAAAx3CAAAABAAAAADdAAFYnl0ZXN1cgACW0Ks8xf4BghU4AIAAHhwAAAACnt7cGFyYW0xfX10AAhzdHVkZW50c3VyABZbTG9yZy5leGFtcGxlLlN0dWRlbnQ74Me+Qk/ExAsCAAB4cAAAAAJzcgATb3JnLmV4YW1wbGUuU3R1ZGVudHKMinpwLXAhAgACSQADYWdlTAAEbmFtZXQAEkxqYXZhL2xhbmcvU3RyaW5nO3hwAAAAEnQACHpoYW5nc2Fuc3EAfgAIAAAAFHQABGxpc2l0AAljbGFzc05hbWV0AAp7e3BhcmFtMH19eA=="
	serialByte, err := base64.StdEncoding.DecodeString(serialBase64)
	if err != nil {
		t.Errorf("base64.StdEncoding.DecodeString() error = %v", err)
		return
	}
	objs, err := yserx.ParseJavaSerialized(serialByte)
	if err != nil {
		t.Errorf("GenerateClassObjectFromBytes() error = %v", err)
		return
	}
	if len(objs) <= 0 {
		t.Errorf("ParseJavaSerialized() error = %v", err)
		return
	}
	obj := objs[0]
	err = ReplaceStringInJavaSerilizable(obj, "{{param0}}", exceptClassName, -1)
	if err != nil {
		t.Errorf("ReplaceStringInJavaSerilizable() error = %v", err)
		return
	}
	err = ReplaceByteArrayInJavaSerilizable(obj, []byte("{{param1}}"), exceptClassBytes, -1)
	if err != nil {
		t.Errorf("ReplaceByteArrayInJavaSerilizable() error = %v", err)
		return
	}
	objJson, _ := yserx.ToJson(obj)
	// 如果包含{{param0}}或者{{param1}}，则说明替换失败
	if strings.Contains(string(objJson), "{{param0}}") || strings.Contains(string(objJson), "{{param1}}") {
		t.Errorf("ReplaceByteArrayInJavaSerilizable() error = %v", err)
		return
	}

	if strings.Contains(string(objJson), exceptClassName) && strings.Contains(string(objJson), base64.StdEncoding.EncodeToString(exceptClassBytes)) {
		t.Logf("ReplaceByteArrayInJavaSerilizable() success")
		return
	} else {
		t.Errorf("ReplaceByteArrayInJavaSerilizable() error = %v", err)
		return
	}

}
