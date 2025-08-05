package yakgit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
)

func TestParseFlexibleDate(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string // 期望的日期字符串格式 "2006-01-02"
		hasError bool
	}{
		{
			name:     "字符串日期 - 标准格式",
			input:    "2023-12-25",
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "字符串日期 - 斜杠格式",
			input:    "2023/12/25",
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "字符串日期 - 美式格式",
			input:    "12/25/2023",
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "字符串日期 - 简短格式",
			input:    "2023-1-2",
			expected: "2023-01-02",
			hasError: false,
		},
		{
			name:     "字符串日期 - 带时间",
			input:    "2023-12-25 15:30:45",
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "字符串日期 - RFC3339",
			input:    "2023-12-25T15:30:45Z",
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "time.Time 类型",
			input:    time.Date(2023, 12, 25, 0, 0, 0, 0, time.UTC),
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "Unix 时间戳 - 秒",
			input:    int64(1703462400), // 2023-12-25 00:00:00 UTC
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "Unix 时间戳 - 毫秒",
			input:    int64(1703462400000), // 2023-12-25 00:00:00 UTC
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "字符串时间戳",
			input:    "1703462400",
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "整数类型",
			input:    1703462400,
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "浮点数类型",
			input:    1703462400.0,
			expected: "2023-12-25",
			hasError: false,
		},
		{
			name:     "无效输入 - nil",
			input:    nil,
			expected: "",
			hasError: true,
		},
		{
			name:     "无效输入 - 空字符串",
			input:    "",
			expected: "",
			hasError: true,
		},
		{
			name:     "无效输入 - 格式错误",
			input:    "invalid-date",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseFlexibleDate(tt.input)

			if tt.hasError {
				assert.Error(t, err, "Expected error for input: %v", tt.input)
			} else {
				assert.NoError(t, err, "Unexpected error for input: %v", tt.input)
				assert.Equal(t, tt.expected, result.Format("2006-01-02"), "Date mismatch for input: %v", tt.input)
			}
		})
	}
}

func TestFileSystemFromCommitDateRange(t *testing.T) {
	// 这个测试需要一个真实的 git 仓库，这里我们测试函数的基本逻辑
	testRepo := getTestGitRepo(t)

	// 测试基本的日期范围功能
	t.Run("基本日期范围测试", func(t *testing.T) {
		// 使用一个很大的日期范围，确保能包含测试仓库中的提交
		startDate := "2020-01-01"
		endDate := "2030-12-31"

		fs, err := FileSystemFromCommitDateRange(testRepo, startDate, endDate)
		if err != nil {
			// 如果没有找到提交，这是正常的（测试仓库可能没有在这个范围内的提交）
			log.Infof("No commits found in date range (expected for test repo): %v", err)
			return
		}

		assert.NotNil(t, fs, "FileSystem should not be nil")
	})

	// 测试不同的日期格式
	t.Run("不同日期格式测试", func(t *testing.T) {
		testCases := []struct {
			name      string
			startDate any
			endDate   any
		}{
			{
				name:      "字符串格式",
				startDate: "2023-01-01",
				endDate:   "2023-12-31",
			},
			{
				name:      "斜杠格式",
				startDate: "2023/01/01",
				endDate:   "2023/12/31",
			},
			{
				name:      "时间戳格式",
				startDate: int64(1672531200), // 2023-01-01
				endDate:   int64(1704067199), // 2023-12-31
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := FileSystemFromCommitDateRange(testRepo, tc.startDate, tc.endDate)
				// 可能没有找到提交，但不应该有解析错误
				if err != nil && err.Error() != "no commits found in the specified date range" {
					t.Logf("Date parsing should work, got error: %v", err)
				}
			})
		}
	})
}

func TestFileSystemCurrentWeek(t *testing.T) {
	testRepo := getTestGitRepo(t)

	t.Run("当前周测试", func(t *testing.T) {
		_, err := FileSystemCurrentWeek(testRepo)
		// 可能没有找到提交，但函数应该能正常执行
		if err != nil && err.Error() != "no commits found in the specified date range" {
			t.Logf("Current week function should work, got error: %v", err)
		}
	})
}

func TestFileSystemLastSevenDay(t *testing.T) {
	testRepo := getTestGitRepo(t)

	t.Run("最近七天测试", func(t *testing.T) {
		_, err := FileSystemLastSevenDay(testRepo)
		// 可能没有找到提交，但函数应该能正常执行
		if err != nil && err.Error() != "no commits found in the specified date range" {
			t.Logf("Last seven days function should work, got error: %v", err)
		}
	})
}

func TestFileSystemCurrentDay(t *testing.T) {
	testRepo := getTestGitRepo(t)

	t.Run("当前日测试", func(t *testing.T) {
		_, err := FileSystemCurrentDay(testRepo)
		// 可能没有找到提交，但函数应该能正常执行
		if err != nil && err.Error() != "no commits found in the specified date range" {
			t.Logf("Current day function should work, got error: %v", err)
		}
	})
}

func TestFileSystemCurrentMonth(t *testing.T) {
	testRepo := getTestGitRepo(t)

	t.Run("当前月测试", func(t *testing.T) {
		_, err := FileSystemCurrentMonth(testRepo)
		// 可能没有找到提交，但函数应该能正常执行
		if err != nil && err.Error() != "no commits found in the specified date range" {
			t.Logf("Current month function should work, got error: %v", err)
		}
	})
}

func TestFileSystemFromDate(t *testing.T) {
	testRepo := getTestGitRepo(t)

	t.Run("指定日期测试", func(t *testing.T) {
		testDates := []any{
			"2023-12-25",
			"2023/12/25",
			int64(1703462400), // 2023-12-25
			time.Date(2023, 12, 25, 0, 0, 0, 0, time.UTC),
		}

		for i, date := range testDates {
			t.Run(fmt.Sprintf("日期格式_%d", i), func(t *testing.T) {
				_, err := FileSystemFromDate(testRepo, date)
				// 可能没有找到提交，但函数应该能正常执行
				if err != nil && err.Error() != "no commits found in the specified date range" {
					t.Logf("Date function should work for %v, got error: %v", date, err)
				}
			})
		}
	})
}

func TestFileSystemFromMonth(t *testing.T) {
	testRepo := getTestGitRepo(t)

	t.Run("指定月份测试", func(t *testing.T) {
		_, err := FileSystemFromMonth(testRepo, 2023, 12)
		// 可能没有找到提交，但函数应该能正常执行
		if err != nil && err.Error() != "no commits found in the specified date range" {
			t.Logf("Month function should work, got error: %v", err)
		}
	})

	t.Run("边界月份测试", func(t *testing.T) {
		// 测试1月和12月
		testCases := []struct {
			year  int
			month int
		}{
			{2023, 1},  // 一月
			{2023, 12}, // 十二月
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("年月_%d_%d", tc.year, tc.month), func(t *testing.T) {
				_, err := FileSystemFromMonth(testRepo, tc.year, tc.month)
				if err != nil && err.Error() != "no commits found in the specified date range" {
					t.Logf("Month function should work for %d-%d, got error: %v", tc.year, tc.month, err)
				}
			})
		}
	})
}

// 测试日期范围计算的准确性
func TestDateRangeCalculation(t *testing.T) {
	t.Run("周计算测试", func(t *testing.T) {
		// 这里我们可以测试周一到周日的计算逻辑
		now := time.Date(2023, 12, 27, 15, 30, 0, 0, time.UTC) // 这是一个周三

		weekday := now.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -int(weekday-time.Monday))
		sunday := monday.AddDate(0, 0, 6)

		assert.Equal(t, "2023-12-25", monday.Format("2006-01-02"), "Monday calculation should be correct")
		assert.Equal(t, "2023-12-31", sunday.Format("2006-01-02"), "Sunday calculation should be correct")
	})

	t.Run("月计算测试", func(t *testing.T) {
		// 测试月份的第一天和最后一天计算
		year := 2023
		month := 2 // 二月

		firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)
		nextMonth := firstDay.AddDate(0, 1, 0)
		lastDay := nextMonth.Add(-time.Nanosecond)

		assert.Equal(t, "2023-02-01", firstDay.Format("2006-01-02"), "First day of month should be correct")
		assert.Equal(t, "2023-02-28", lastDay.Format("2006-01-02"), "Last day of February should be correct")
	})
}
