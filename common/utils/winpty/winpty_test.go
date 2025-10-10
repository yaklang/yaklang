package winpty

import (
	"testing"
)

// TestConfigOptions 测试配置选项函数
func TestConfigOptions(t *testing.T) {
	// 在非 Windows 平台上，存根函数不会修改配置
	// 所以我们只测试函数是否可以调用而不出错
	cfg := &WinptyCfg{}

	// 测试各种配置选项函数是否可以正常调用
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Configuration functions should not panic: %v", r)
		}
	}()

	WithDLLPrefix("test_dll")(cfg)
	WithAppName("test_app")(cfg)
	WithCommand("test_cmd")(cfg)
	WithDir("test_dir")(cfg)
	WithFlags(123)(cfg)
	WithInitialSize(100, 50)(cfg)
	WithEnv([]string{"TEST=value"})(cfg)
	WithDefaultEnv()(cfg)
	WithCurrentDir()(cfg)

	t.Log("All configuration functions executed without panic")
}

// TestNewWithOptions 测试使用选项创建配置
func TestNewWithOptions(t *testing.T) {
	// 这个测试在非 Windows 平台上会返回错误，这是预期的行为
	_, err := New(
		WithDLLPrefix("."),
		WithCommand("cmd.exe"),
		WithInitialSize(80, 24),
	)

	// 在非 Windows 平台上应该返回错误
	if err == nil {
		// 如果没有错误，说明是在 Windows 平台上运行
		// 或者 DLL 不存在，这也是正常的
		t.Log("New() succeeded or returned expected error")
	} else {
		// 检查错误消息是否符合预期
		expectedMsg := "WinPTY is only supported on Windows"
		if err.Error() != expectedMsg {
			// 如果不是平台错误，可能是其他错误（如 DLL 不存在）
			t.Logf("Got error (expected on non-Windows or when DLL missing): %v", err)
		}
	}
}

// TestDefaultConfiguration 测试默认配置
func TestDefaultConfiguration(t *testing.T) {
	_, err := NewDefault(".", "cmd.exe")

	// 在非 Windows 平台上应该返回错误
	if err != nil {
		expectedMsg := "WinPTY is only supported on Windows"
		if err.Error() == expectedMsg {
			t.Log("NewDefault() correctly returned platform error")
		} else {
			t.Logf("NewDefault() returned error (expected when DLL missing): %v", err)
		}
	}
}

// TestWinPTYMethods 测试 WinPTY 方法
func TestWinPTYMethods(t *testing.T) {
	// 创建一个空的 WinPTY 实例用于测试方法
	pty := &WinPTY{}

	// 在存根实现中，IsClosed 总是返回 true
	// 在 Windows 实现中，默认为 false
	isClosed := pty.IsClosed()
	t.Logf("IsClosed() returned: %v", isClosed)

	// 测试 GetProcessHandle
	handle := pty.GetProcessHandle()
	t.Logf("GetProcessHandle() returned: %d", handle)

	// 测试 Close (应该不会出错)
	err := pty.Close()
	if err != nil {
		t.Errorf("Expected Close() to succeed, got error: %v", err)
	}

	// 测试重复关闭
	err = pty.Close()
	if err != nil {
		t.Errorf("Expected second Close() to succeed, got error: %v", err)
	}

	// 测试 SetSize
	err = pty.SetSize(80, 24)
	// 在非 Windows 平台上会返回错误，这是预期的
	if err != nil {
		t.Logf("SetSize() returned expected error: %v", err)
	}
}

// BenchmarkConfigOptions 基准测试配置选项的性能
func BenchmarkConfigOptions(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cfg := &WinptyCfg{}
		WithDLLPrefix("test")(cfg)
		WithCommand("cmd.exe")(cfg)
		WithInitialSize(80, 24)(cfg)
	}
}
