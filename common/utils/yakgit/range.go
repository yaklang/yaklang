package yakgit

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 支持的日期格式列表
var supportedDateFormats = []string{
	"2006-01-02",
	"2006/01/02",
	"2006-1-2",
	"2006/1/2",
	"06-01-02",
	"06/01/02",
	"2006-01-02 15:04:05",
	"2006/01/02 15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05",
	"01/02/2006",
	"1/2/2006",
	"02-01-2006",
	"2-1-2006",
	time.RFC3339,
	time.RFC822,
	time.RFC1123,
}

// parseFlexibleDate 灵活解析日期，支持多种格式和 any 类型
func parseFlexibleDate(dateInput any) (time.Time, error) {
	if dateInput == nil {
		return time.Time{}, utils.Error("date input is nil")
	}

	// 如果已经是 time.Time 类型，直接返回
	if t, ok := dateInput.(time.Time); ok {
		return t, nil
	}

	// 转换为字符串
	var dateStr string
	switch v := dateInput.(type) {
	case string:
		dateStr = v
	case int, int8, int16, int32, int64:
		// 尝试作为 Unix 时间戳解析
		timestamp := reflect.ValueOf(v).Int()
		if timestamp > 1e10 { // 毫秒时间戳
			return time.Unix(timestamp/1000, (timestamp%1000)*1e6), nil
		} else { // 秒时间戳
			return time.Unix(timestamp, 0), nil
		}
	case uint, uint8, uint16, uint32, uint64:
		timestamp := reflect.ValueOf(v).Uint()
		if timestamp > 1e10 { // 毫秒时间戳
			return time.Unix(int64(timestamp/1000), int64((timestamp%1000)*1e6)), nil
		} else { // 秒时间戳
			return time.Unix(int64(timestamp), 0), nil
		}
	case float32, float64:
		timestamp := int64(reflect.ValueOf(v).Float())
		return time.Unix(timestamp, 0), nil
	default:
		dateStr = fmt.Sprintf("%v", v)
	}

	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return time.Time{}, utils.Error("empty date string")
	}

	// 特殊处理：纯数字字符串可能是时间戳
	if num, err := strconv.ParseInt(dateStr, 10, 64); err == nil {
		if num > 1e10 { // 毫秒时间戳
			return time.Unix(num/1000, (num%1000)*1e6), nil
		} else if num > 1e8 { // 秒时间戳
			return time.Unix(num, 0), nil
		}
	}

	// 尝试各种格式解析
	for _, format := range supportedDateFormats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, utils.Errorf("unable to parse date: %s", dateStr)
}

// getCommitsByDateRange 根据日期范围获取提交列表
// startDate: 起始日期（包含该日期的最早时间）
// endDate: 结束日期（包含该日期的最晚时间）
func getCommitsByDateRange(repos string, startDate, endDate time.Time) ([]*object.Commit, error) {
	log.Infof("start to get commits by date range: %v to %v", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	repo, err := GitOpenRepositoryWithCache(repos)
	if err != nil {
		return nil, utils.Wrap(err, "open repository failed")
	}

	// 获取HEAD引用
	head, err := repo.Head()
	if err != nil {
		return nil, utils.Wrap(err, "get HEAD reference failed")
	}

	// 获取提交历史
	commitIter, err := repo.Log(&git.LogOptions{
		From: head.Hash(),
	})
	if err != nil {
		return nil, utils.Wrap(err, "get commit log failed")
	}

	var commits []*object.Commit
	err = commitIter.ForEach(func(commit *object.Commit) error {
		commitTime := commit.Author.When
		// 检查提交时间是否在指定范围内
		if (commitTime.After(startDate) || commitTime.Equal(startDate)) &&
			(commitTime.Before(endDate) || commitTime.Equal(endDate)) {
			commits = append(commits, commit)
		}
		return nil
	})

	if err != nil {
		return nil, utils.Wrap(err, "iterate commits failed")
	}

	log.Infof("found %d commits in date range", len(commits))
	return commits, nil
}

// FileSystemFromCommitDateRange 根据日期范围获取文件系统
// startDate: 起始日期，支持多种格式和类型
// endDate: 结束日期，支持多种格式和类型
func FileSystemFromCommitDateRange(repos string, startDate, endDate any) (filesys_interface.FileSystem, error) {
	log.Infof("start to get filesystem from commit date range: %v to %v", startDate, endDate)

	// 解析起始日期
	start, err := parseFlexibleDate(startDate)
	if err != nil {
		return nil, utils.Wrapf(err, "parse start date %v failed", startDate)
	}

	// 解析结束日期
	end, err := parseFlexibleDate(endDate)
	if err != nil {
		return nil, utils.Wrapf(err, "parse end date %v failed", endDate)
	}

	// 设置时间范围：起始日期的00:00:00到结束日期的23:59:59
	startTime := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	endTime := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 999999999, end.Location())

	return fromCommitDateRangeTime(repos, startTime, endTime)
}

// fromCommitDateRangeTime 根据时间范围获取文件系统（内部函数）
func fromCommitDateRangeTime(repos string, startTime, endTime time.Time) (filesys_interface.FileSystem, error) {
	// 获取日期范围内的提交
	commits, err := getCommitsByDateRange(repos, startTime, endTime)
	if err != nil {
		return nil, err
	}

	if len(commits) == 0 {
		return nil, utils.Error("no commits found in the specified date range")
	}

	// 收集所有提交的哈希值
	var commitHashes []string
	for _, commit := range commits {
		commitHashes = append(commitHashes, commit.Hash.String())
	}

	log.Infof("merging %d commits into filesystem", len(commitHashes))

	// 使用现有的 FromCommits 函数合并所有提交
	return FromCommits(repos, commitHashes...)
}

// FileSystemCurrentWeek 获取当前自然周（周一到周天）的文件系统
func FileSystemCurrentWeek(repos string) (filesys_interface.FileSystem, error) {
	log.Infof("start to get filesystem from current week")

	now := time.Now()

	// 计算本周一的日期
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7 // 将周日设为7，方便计算
	}
	monday := now.AddDate(0, 0, -int(weekday-time.Monday))

	// 计算本周日的日期
	sunday := monday.AddDate(0, 0, 6)

	// 设置时间范围：周一的00:00:00到周日的23:59:59
	startTime := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
	endTime := time.Date(sunday.Year(), sunday.Month(), sunday.Day(), 23, 59, 59, 999999999, sunday.Location())

	log.Infof("current week range: %v to %v", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))

	return fromCommitDateRangeTime(repos, startTime, endTime)
}

// FileSystemLastSevenDay 获取最近七天的文件系统
func FileSystemLastSevenDay(repos string) (filesys_interface.FileSystem, error) {
	log.Infof("start to get filesystem from last seven days")

	now := time.Now()

	// 计算七天前的日期
	sevenDaysAgo := now.AddDate(0, 0, -6) // -6是因为包含今天，总共7天

	// 设置时间范围：七天前的00:00:00到今天的23:59:59
	startTime := time.Date(sevenDaysAgo.Year(), sevenDaysAgo.Month(), sevenDaysAgo.Day(), 0, 0, 0, 0, sevenDaysAgo.Location())
	endTime := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())

	log.Infof("last seven days range: %v to %v", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))

	return fromCommitDateRangeTime(repos, startTime, endTime)
}

// FileSystemCurrentDay 获取当前自然日的文件系统
func FileSystemCurrentDay(repos string) (filesys_interface.FileSystem, error) {
	log.Infof("start to get filesystem from current day")

	now := time.Now()

	// 设置时间范围：今天的00:00:00到23:59:59
	startTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endTime := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())

	log.Infof("current day range: %v", startTime.Format("2006-01-02"))

	return fromCommitDateRangeTime(repos, startTime, endTime)
}

// FileSystemCurrentMonth 获取当前自然月的文件系统
func FileSystemCurrentMonth(repos string) (filesys_interface.FileSystem, error) {
	log.Infof("start to get filesystem from current month")

	now := time.Now()

	// 计算本月第一天
	firstDay := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// 计算本月最后一天
	nextMonth := firstDay.AddDate(0, 1, 0)
	lastDay := nextMonth.Add(-time.Nanosecond)

	log.Infof("current month range: %v to %v", firstDay.Format("2006-01-02"), lastDay.Format("2006-01-02"))

	return fromCommitDateRangeTime(repos, firstDay, lastDay)
}

// FileSystemFromDate 根据指定日期获取该日的文件系统
func FileSystemFromDate(repos string, date any) (filesys_interface.FileSystem, error) {
	log.Infof("start to get filesystem from date: %v", date)

	// 解析日期
	targetDate, err := parseFlexibleDate(date)
	if err != nil {
		return nil, utils.Wrapf(err, "parse date %v failed", date)
	}

	// 设置时间范围：指定日期的00:00:00到23:59:59
	startTime := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 0, 0, 0, 0, targetDate.Location())
	endTime := time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(), 23, 59, 59, 999999999, targetDate.Location())

	log.Infof("target date range: %v", startTime.Format("2006-01-02"))

	return fromCommitDateRangeTime(repos, startTime, endTime)
}

// FileSystemFromMonth 根据指定年月获取该月的文件系统
func FileSystemFromMonth(repos string, year int, month int) (filesys_interface.FileSystem, error) {
	log.Infof("start to get filesystem from month: %d-%02d", year, month)

	// 计算指定月的第一天
	firstDay := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local)

	// 计算指定月的最后一天
	nextMonth := firstDay.AddDate(0, 1, 0)
	lastDay := nextMonth.Add(-time.Nanosecond)

	log.Infof("target month range: %v to %v", firstDay.Format("2006-01-02"), lastDay.Format("2006-01-02"))

	return fromCommitDateRangeTime(repos, firstDay, lastDay)
}
