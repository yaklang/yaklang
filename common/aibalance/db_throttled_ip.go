package aibalance

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

// db_throttled_ip.go 实现「一键限流 IP」的持久化与热路径查询。
//
// 设计目标：管理员在面板里对某个滥用 IP 一键限流后，该 IP 的请求频率（RPM）与流式
// 输出速率（TPS）被压到一个很低的值（默认 RPM=3 / TPS=15，可配置）。限流是持久的，
// 不随每日切日清空，需手动解除。为避免热路径每次请求都查库，这里维护一个进程内缓存：
//   - ReloadThrottledIPCache 从 DB 全量加载到缓存（启动时调用一次，写操作后刷新）；
//   - lookupThrottledIP 走缓存读，O(1) 且并发安全，供 gates / writer 热路径使用。
//
// 关键词: db_throttled_ip, 一键限流 IP 持久化, 进程内缓存, per-IP RPM/TPS 热路径

// throttledIPEntry 是缓存里单个被限流 IP 的生效参数。
type throttledIPEntry struct {
	RPM int64
	TPS int64
}

var (
	throttledIPMu    sync.RWMutex
	throttledIPCache = map[string]throttledIPEntry{}
)

// EnsureThrottledIPTable ensures the AiBalanceThrottledIP table exists.
// 关键词: EnsureThrottledIPTable
func EnsureThrottledIPTable() error {
	return GetDB().AutoMigrate(&AiBalanceThrottledIP{}).Error
}

// normalizeThrottleIP 清洗 IP 输入；空 / unknown 视为非法（复用免费 IP 的忽略口径）。
// 关键词: normalizeThrottleIP, 非法 IP 拒绝
func normalizeThrottleIP(ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	if freeIPUsageIgnoredIP(ip) {
		return "", fmt.Errorf("invalid ip for throttle: %q", ip)
	}
	return ip, nil
}

// ReloadThrottledIPCache 从 DB 全量加载被限流 IP 到进程内缓存（整表 swap）。
// 被限流 IP 数量很小，整表加载成本可忽略。
// 关键词: ReloadThrottledIPCache, 整表加载 swap
func ReloadThrottledIPCache() error {
	var rows []AiBalanceThrottledIP
	if err := GetDB().Find(&rows).Error; err != nil {
		return fmt.Errorf("ReloadThrottledIPCache failed: %v", err)
	}
	next := make(map[string]throttledIPEntry, len(rows))
	for _, r := range rows {
		ip := strings.TrimSpace(r.IP)
		if ip == "" {
			continue
		}
		next[ip] = throttledIPEntry{RPM: r.RPM, TPS: r.TPS}
	}
	throttledIPMu.Lock()
	throttledIPCache = next
	throttledIPMu.Unlock()
	log.Infof("throttled ip cache reloaded: %d entries", len(next))
	return nil
}

// lookupThrottledIP 走缓存查询某 IP 是否被限流及其 RPM/TPS。热路径使用。
// 关键词: lookupThrottledIP, 缓存读, 热路径
func lookupThrottledIP(ip string) (rpm, tps int64, ok bool) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return 0, 0, false
	}
	throttledIPMu.RLock()
	entry, exist := throttledIPCache[ip]
	throttledIPMu.RUnlock()
	if !exist {
		return 0, 0, false
	}
	return entry.RPM, entry.TPS, true
}

// IsIPThrottled 返回某 IP 是否当前被限流（走缓存）。
// 关键词: IsIPThrottled
func IsIPThrottled(ip string) bool {
	_, _, ok := lookupThrottledIP(ip)
	return ok
}

// UpsertThrottledIP 新增 / 更新一个被限流 IP；rpm/tps<=0 会保留为 0（表示该维度不限）。
// 写库成功后立即把该条目写入缓存（避免热路径读到旧值）。
// 关键词: UpsertThrottledIP, 一键限流写入
func UpsertThrottledIP(ip string, rpm, tps int64, reason string) error {
	ip, err := normalizeThrottleIP(ip)
	if err != nil {
		return err
	}
	if rpm < 0 {
		rpm = 0
	}
	if tps < 0 {
		tps = 0
	}
	db := GetDB()

	var row AiBalanceThrottledIP
	qErr := db.Where("ip = ?", ip).First(&row).Error
	if qErr == nil {
		updates := map[string]interface{}{
			"rpm":        rpm,
			"tps":        tps,
			"reason":     reason,
			"updated_at": time.Now(),
		}
		if uErr := db.Model(&AiBalanceThrottledIP{}).Where("id = ?", row.ID).Updates(updates).Error; uErr != nil {
			return fmt.Errorf("UpsertThrottledIP update failed: %v", uErr)
		}
	} else if errors.Is(qErr, gorm.ErrRecordNotFound) {
		row = AiBalanceThrottledIP{IP: ip, RPM: rpm, TPS: tps, Reason: reason}
		if cErr := db.Create(&row).Error; cErr != nil {
			return fmt.Errorf("UpsertThrottledIP create failed: %v", cErr)
		}
	} else {
		return fmt.Errorf("UpsertThrottledIP query failed: %v", qErr)
	}

	throttledIPMu.Lock()
	throttledIPCache[ip] = throttledIPEntry{RPM: rpm, TPS: tps}
	throttledIPMu.Unlock()
	return nil
}

// DeleteThrottledIP 解除某 IP 的限流（删除行 + 缓存条目）。IP 不存在视为成功。
// 关键词: DeleteThrottledIP, 解除限流
func DeleteThrottledIP(ip string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("DeleteThrottledIP: ip is empty")
	}
	if err := GetDB().Where("ip = ?", ip).Delete(&AiBalanceThrottledIP{}).Error; err != nil {
		return fmt.Errorf("DeleteThrottledIP failed: %v", err)
	}
	throttledIPMu.Lock()
	delete(throttledIPCache, ip)
	throttledIPMu.Unlock()
	return nil
}

// ListThrottledIPs 返回所有被限流 IP（按更新时间倒序），供面板展示与解除。
// 关键词: ListThrottledIPs, 已限流 IP 列表
func ListThrottledIPs() ([]AiBalanceThrottledIP, error) {
	var rows []AiBalanceThrottledIP
	if err := GetDB().Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListThrottledIPs failed: %v", err)
	}
	return rows, nil
}
