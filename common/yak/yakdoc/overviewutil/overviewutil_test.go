package overviewutil

import (
	"strings"
	"testing"
)

// TestFirstParagraph 验证"一句话库定位"派生: 跳过前导标题/空行/代码围栏,
// 取首个正文段落, 归一化空白并按 maxShortRunes 截断。
func TestFirstParagraph(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want string
	}{
		{
			name: "skip heading then take first paragraph",
			md:   "# file\n\nfile 库用于文件系统读写操作。\n\n第二段内容不应被包含。",
			want: "file 库用于文件系统读写操作。",
		},
		{
			name: "merge multiline paragraph",
			md:   "## http\n\nhttp 库\n用于发送请求。\n\n下一段",
			want: "http 库 用于发送请求。",
		},
		{
			name: "skip leading code fence",
			md:   "```\ncode block\n```\n\n这是首段。",
			want: "这是首段。",
		},
		{
			name: "empty input",
			md:   "",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FirstParagraph(c.md)
			if got != c.want {
				t.Fatalf("FirstParagraph() = %q, want %q", got, c.want)
			}
		})
	}
}

// TestFirstParagraph_Truncate 验证超长段落会被截断并追加省略号。
func TestFirstParagraph_Truncate(t *testing.T) {
	long := strings.Repeat("阿", maxShortRunes+50)
	got := FirstParagraph(long)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated summary to end with ..., got suffix %q", got[len(got)-10:])
	}
	if got == long {
		t.Fatal("expected summary to be truncated")
	}
}
