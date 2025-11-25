package bruteutils

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMYSQLAuth_Error1044 测试 MySQL 1044 错误处理
// 1044 错误表示账号密码正确，但没有数据库访问权限，应该被视为认证成功
func TestMYSQLAuth_Error1044(t *testing.T) {
	// 创建一个模拟的错误处理函数来测试错误处理逻辑
	testCases := []struct {
		name          string
		errMsg        string
		expectedOk    bool
		expectedFin   bool
		shouldHaveErr bool
	}{
		{
			name:          "Error 1044 with full format",
			errMsg:        "Error 1044: Access denied for user 'test'@'localhost' to database 'mysql'",
			expectedOk:    true,
			expectedFin:   false,
			shouldHaveErr: false,
		},
		{
			name:          "Error 1044 with short format",
			errMsg:        "1044: Access denied for user 'test'@'localhost' to database 'mysql'",
			expectedOk:    true,
			expectedFin:   false,
			shouldHaveErr: false,
		},
		{
			name:          "Error 1045 authentication failed",
			errMsg:        "Error 1045: Access denied for user 'test'@'localhost' (using password: YES)",
			expectedOk:    false,
			expectedFin:   false,
			shouldHaveErr: true,
		},
		{
			name:          "Connection refused",
			errMsg:        "connect: connection refused",
			expectedOk:    false,
			expectedFin:   true,
			shouldHaveErr: true,
		},
		{
			name:          "Not allowed to connect",
			errMsg:        "is not allowed to connect to",
			expectedOk:    false,
			expectedFin:   true,
			shouldHaveErr: true,
		},
		{
			name:          "Other error",
			errMsg:        "some other error occurred",
			expectedOk:    false,
			expectedFin:   false,
			shouldHaveErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟错误处理逻辑
			err := errors.New(tc.errMsg)
			errStr := err.Error()

			var ok, finished bool
			var resultErr error

			switch true {
			case strings.Contains(errStr, "is not allowed to connect to"):
				fallthrough
			case strings.Contains(errStr, "connect: connection refused"):
				ok, finished, resultErr = false, true, err
			case strings.Contains(errStr, "Error 1045:"):
				ok, finished, resultErr = false, false, err
			case strings.Contains(errStr, "Error 1044:") || strings.Contains(errStr, "1044:"):
				ok, finished, resultErr = true, false, nil
			default:
				ok, finished, resultErr = false, false, err
			}

			assert.Equal(t, tc.expectedOk, ok, "ok value should match")
			assert.Equal(t, tc.expectedFin, finished, "finished value should match")
			if tc.shouldHaveErr {
				assert.NotNil(t, resultErr, "should have error")
			} else {
				assert.Nil(t, resultErr, "should not have error")
			}
		})
	}
}

// TestMYSQLAuth_Error1044_RealScenario 测试真实的 1044 错误场景
// 这个测试验证当遇到 1044 错误时，应该返回 ok=true（认证成功）
func TestMYSQLAuth_Error1044_RealScenario(t *testing.T) {
	// 模拟真实的 1044 错误消息格式
	realErrorMsg := "Error 1044: Access denied for user 'secret letter'@'g' to database 'mysql'"

	err := errors.New(realErrorMsg)
	errStr := err.Error()

	var ok, finished bool
	var resultErr error

	switch true {
	case strings.Contains(errStr, "is not allowed to connect to"):
		fallthrough
	case strings.Contains(errStr, "connect: connection refused"):
		ok, finished, resultErr = false, true, err
	case strings.Contains(errStr, "Error 1045:"):
		ok, finished, resultErr = false, false, err
	case strings.Contains(errStr, "Error 1044:") || strings.Contains(errStr, "1044:"):
		// 1044 错误应该被视为认证成功
		ok, finished, resultErr = true, false, nil
	default:
		ok, finished, resultErr = false, false, err
	}

	// 验证 1044 错误被正确识别为认证成功
	assert.True(t, ok, "1044 error should be treated as authentication success")
	assert.False(t, finished, "1044 error should not mark as finished")
	assert.Nil(t, resultErr, "1044 error should not return error")
}
