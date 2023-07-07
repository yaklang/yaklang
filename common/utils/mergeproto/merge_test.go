package mergeproto

import (
	"fmt"
	"os"
	"testing"
)

func TestGenProtoFiles(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp(os.TempDir(), "prefix-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s", err)
	}

	// 删除临时目录
	defer os.RemoveAll(tempDir)

	// 文件名和内容映射
	fileData := map[string]string{
		"file1.proto": `syntax = "proto3";

package ypb;
option go_package = "/;ypb";


service T1 {
  // 分析一个 HTTP 请求详情
  rpc HTTPRequestAnalyzer(HTTPRequestAnalysisMaterial) returns (HTTPRequestAnalysis);

}

message HTTPRequestAnalysisMaterial {
  string TypePosition = 1;
}

enum ShellType {
    Behinder = 0;
    Godzilla = 1;
}

`,
		"file2.proto": `syntax = "proto3";
package ypb;
option go_package = "/;ypb";

enum ShellType {
    Behinder = 0;
    Godzilla = 1;
}

service T2 {
	// 分析一个 HTTP 请求详情
	rpc HTTPRequestAnalyzer(HTTPRequestAnalysisMaterial) returns (HTTPRequestAnalysis);
	rpc MITM(stream MITMRequest) returns (stream MITMResponse);
}

message MapMessage {
    map<string, int32> name_to_id = 1;
	repeated string ExactKeywords = 1;
  string old_field = 1 [deprecated=true];
  string new_field = 2;
}

message HTTPRequestAnalysisMaterial {
  string TypePosition = 1;
}

`,
		//"file3.proto": "This is some data for file3.proto",
	}
	for filename, content := range fileData {
		// 在临时目录中创建新文件
		filePath := tempDir + "/" + filename
		file, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("Failed to create file %s: %s", filePath, err)
		}
		defer file.Close()

		// 写入数据
		if _, err := file.WriteString(content); err != nil {
			t.Fatalf("Failed to write to file %s: %s", filePath, err)
		}
	}

	buf, err := GenProtoBytes(tempDir, "ypb")
	if err != nil {
		t.Fatalf("Failed to generate proto files: %s", err)
	}
	fmt.Println(buf.String())
}

func TestGenProtoBytes(t *testing.T) {
	b, err := GenProtoBytes("../../yakgrpc/protos/", "ypb")
	if err != nil {
		t.Fatalf("Failed to generate proto files: %s", err)
	}
	b.WriteProtoFile()
}
