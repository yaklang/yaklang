package bruteutils

import (
	"bytes"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var telnetHostlock sync.Map

var telnetAuth = &DefaultServiceAuthInfo{
	ServiceName:  "telnet",
	DefaultPorts: "23",
	DefaultUsernames: []string{
		"admin", "cisco", "test", "root",
	},
	DefaultPasswords: []string{
		"123456", "123", "admin",
		"cisco", "cisco123", "cisco123$", "Cisco", "Cisco123",
		"Cisco123$",
	},
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 23)

		conn, err := defaultDialer.DialContext(utils.TimeoutContext(defaultTimeout), "tcp", i.Target)
		if err != nil {
			log.Errorf("telnet:%v conn failed: %s", i.Target, err)
			res := i.Result()
			res.Finished = true
			return res
		}
		defer conn.Close()

		raw := telnetReadStable(conn, defaultTimeout, 1024)
		if raw == nil {
			res := i.Result()
			res.Finished = true
			return res
		}

		conn.Write([]byte("?\n"))
		raw = telnetReadStable(conn, defaultTimeout, 4096)
		if raw == nil {
			return i.Result()
		}

		if utils.MatchAllOfRegexp(string(raw), "(?)route", "aaa", "ip") ||
			utils.MatchAllOfSubString(string(raw), "UNAUTHORIZED ACCESS TO THIS DEVICE") ||
			utils.MatchAnyOfSubString(string(raw), `prompt for`) ||
			utils.MatchAllOfSubString(string(raw), "We Monitor Our Traffic") ||
			utils.MatchAllOfSubString(string(raw), "THDCR001SW23>") {
			r := i.Result()
			r.Ok = true
			r.Username = ""
			r.Password = ""
			r.ExtraInfo = raw
			return r
		} else {
			log.Infof("===============%v================", i.Target)
			log.Info("===========================================")
		}

		return i.Result()
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		if i.Password == "" && i.Username == "" {
			log.Info("empty username and password")
		}
		log.Debugf("telnet client start to handle: %s", i.String())
		defer log.Debugf("telnet finished to handle: %s", i.String())

		result := i.Result()

		defer func() {
			if err := recover(); err != nil {
				log.Errorf("telnet panic: %s", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		var mutex *sync.Mutex
		val, ok := telnetHostlock.Load(i.Target)
		if ok {
			mutex = val.(*sync.Mutex)
		} else {
			mutex = new(sync.Mutex)
			telnetHostlock.Store(i.Target, mutex)
		}
		mutex.Lock()
		defer mutex.Unlock()

		conn, err := defaultDialer.DialContext(utils.TimeoutContext(defaultTimeout), "tcp", i.Target)
		if err != nil {
			log.Errorf("get auto proxy conn ex failed: %s", err)
			if utils.MatchAnyOfRegexp(err.Error(), `(?i)timeout`) {
				return result
			}
			return result
		}

		defer conn.Close()

		doPassword := func() *BruteItemResult {
			passRaw := telnetReadUntil(conn, defaultTimeout, `(?i)password`, `(?i)verification code:`)
			if utils.MatchAnyOfRegexp(string(passRaw), `(?i)password`, `(?i)verification code:`) {
				conn.Write([]byte(i.Password + "\n"))
				bruteResult := telnetReadUntil(conn, defaultTimeout,
					`(?i)invalid`, `(?i)incorrect`, `(?i)fail`,
					`(?i)correct`, `(?i)logged`, `(?i)succe`)
				if utils.MatchAnyOfRegexp(string(bruteResult), `(?i)invalid`, `(?i)incorrect`, `(?i)fail`) {
					return result
				}
				if utils.MatchAnyOfRegexp(string(bruteResult), `(?i)correct`, `(?i)logged`, `(?i)succe`) {
					result.Ok = true
					result.ExtraInfo = bruteResult
					return result
				}
				return result
			}
			return result
		}

		bannerAndFinished := telnetReadUntil(conn, defaultTimeout, `(?i)login`, `(?i)user`, `(?i)password`)
		u := strings.TrimSpace(string(bannerAndFinished))
		if !utils.MatchAnyOfRegexp(u, `(?i)login`, `(?i)user`) {
			// 没有匹配到 login 或者 user，看是不是匹配到 password
			if utils.MatchAnyOfRegexp(u, `(?i)password`) {
				finalResult := doPassword()
				finalResult.OnlyNeedPassword = true
				return finalResult
			}
			return result
		}

		conn.Write([]byte(i.Username + "\n"))
		return doPassword()
	},
}

// telnetReadUntil 流式读取并在内容匹配任一模式时立即返回。
// 旧实现用 StableReaderEx 读完整阶段（提示符不带换行，只能等超时
// 稳定 + 异步 goroutine 退出，单阶段 ~10s，三次交互近 30s/凭证，
// 大字典不可用）；提示符/判定词即协议边界，读到即返回。
func telnetReadUntil(conn net.Conn, timeout time.Duration, patterns ...string) []byte {
	var buf bytes.Buffer
	ddl := time.Now().Add(timeout)
	_ = conn.SetReadDeadline(ddl)
	defer conn.SetReadDeadline(time.Time{})
	ch := make([]byte, 1)
	for time.Now().Before(ddl) {
		n, err := conn.Read(ch)
		if n > 0 {
			buf.Write(ch[:n])
			content := buf.String()
			if utils.MatchAnyOfRegexp(content, patterns...) {
				return buf.Bytes()
			}
		}
		if err != nil {
			return buf.Bytes() // EOF / 超时：返回已读内容
		}
	}
	return buf.Bytes()
}

// telnetReadStable 读取直到静默稳定（连续两个静默窗口无新数据）或 EOF。
// 用于未授权探测的 banner 检查；替代 StableReaderEx（其内部 goroutine
// 需等满外层超时才退出，单次读取 ~10s）。
func telnetReadStable(conn net.Conn, timeout time.Duration, maxSize int) []byte {
	var buf bytes.Buffer
	ddl := time.Now().Add(timeout)
	ch := make([]byte, 1)
	quiet := 0
	for time.Now().Before(ddl) && buf.Len() < maxSize {
		_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		n, err := conn.Read(ch)
		if n > 0 {
			buf.Write(ch[:n])
			quiet = 0
			continue
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				if buf.Len() > 0 {
					quiet++
					if quiet >= 2 {
						break // 已读数据 + 600ms 静默 → 稳定
					}
				}
				continue
			}
			break // EOF / 连接错误：返回已读内容
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	return buf.Bytes()
}
