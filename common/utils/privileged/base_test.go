package privileged

import (
	"context"
	"os"
	"syscall"
	"testing"
)

func TestIsPrivileged(t *testing.T) {
	println(GetIsPrivileged())
}

func TestGetIsPrivileged(t *testing.T) {
	err := syscall.Setuid(0)
	if err != nil {
		panic(err)
	}
}

func TestReleaseIconToTemp(t *testing.T) {
	// 测试图标释放功能
	iconPath, err := releaseIconToTemp()
	if err != nil {
		t.Fatalf("Failed to release icon to temp: %v", err)
	}

	// 验证文件是否存在
	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		t.Fatalf("Icon file does not exist at path: %s", iconPath)
	}

	t.Logf("Icon successfully released to: %s", iconPath)

	// 测试多次调用，应该返回同一个路径（由 utils.Once 保证）
	iconPath2, err := releaseIconToTemp()
	if err != nil {
		t.Fatalf("Failed to release icon on second call: %v", err)
	}

	if iconPath != iconPath2 {
		t.Fatalf("Icon paths differ on multiple calls: %s != %s", iconPath, iconPath2)
	}

	t.Logf("Icon path consistent across multiple calls: %s", iconPath)

	// 验证文件大小
	info, err := os.Stat(iconPath)
	if err != nil {
		t.Fatalf("Failed to stat icon file: %v", err)
	}

	if info.Size() == 0 {
		t.Fatal("Icon file is empty")
	}

	t.Logf("Icon file size: %d bytes", info.Size())
}

func TestExecutorWithSkipConfirmDialog(t *testing.T) {
	// 这个测试演示如何使用 WithSkipConfirmDialog 选项
	// 注意：这个测试需要用户交互（输入管理员密码），所以标记为 Skip
	t.Skip("This test requires user interaction (admin password)")

	executor := NewExecutor("TestApp")

	// 使用 WithSkipConfirmDialog() 选项，跳过第一个确认对话框
	// 直接弹出系统的管理员权限验证对话框
	_, err := executor.Execute(
		context.Background(),
		"echo 'Hello from privileged mode'",
		WithTitle("Test Skip Confirm Dialog"),
		WithDescription("This should skip the confirm dialog and go directly to admin auth"),
		WithSkipConfirmDialog(), // 关键选项：跳过确认对话框
	)

	if err != nil {
		t.Logf("Execution failed (expected if user cancelled): %v", err)
	} else {
		t.Log("Execution succeeded")
	}
}
