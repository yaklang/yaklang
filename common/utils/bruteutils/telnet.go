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

		raw := telnetReadUntil(conn, defaultTimeout)
		if raw == nil {
			res := i.Result()
			res.Finished = true
			return res
		}
		if res := classifyTelnetRefusal(i.Result(), raw); res != nil {
			return res
		}

		conn.Write([]byte("?\n"))
		raw = append(raw, telnetReadUntil(conn, defaultTimeout)...)
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
					`(?i)correct`, `(?i)logged`, `(?i)succe`,
					`(?i)lockout`, `(?i)locked`)
				// 连续失败当场触发锁定（真实设备行为）：置锁定信号，
				// 调度器按锁定预算短路该目标。
				if utils.MatchAnyOfRegexp(string(bruteResult), `(?i)lockout`, `(?i)locked`) {
					result.AccountLocked = true
					result.ExtraInfo = bruteResult
					return result
				}
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

		// 提示符与拒绝特征（锁定/限流）一并等待，读到即返回。
		bannerWait := []string{`(?i)login`, `(?i)user`, `(?i)password`, `(?i)lockout`, `(?i)locked`,
			`(?i)exceed`, `(?i)no more connections`, `(?i)disabled`, `(?i)not enabled`, `(?i)expired`}
		bannerAndFinished := telnetReadUntil(conn, defaultTimeout, bannerWait...)
		if res := classifyTelnetRefusal(result, bannerAndFinished); res != nil {
			return res
		}
		u := strings.TrimSpace(string(bannerAndFinished))
		if !utils.MatchAnyOfRegexp(u, `(?i)login`, `(?i)user`) {
			// 没有匹配到 login 或者 user，看是不是匹配到 password
			if utils.MatchAnyOfRegexp(u, `(?i)password`) {
				finalResult := doPassword()
				finalResult.OnlyNeedPassword = true
				return finalResult
			}
			// 部分真实设备（Shodan 语料：Telnet Server 2.00 等）banner
			// 后不出提示符，需要客户端先敲一次回车。
			conn.Write([]byte("\n"))
			second := telnetReadUntil(conn, defaultTimeout, bannerWait...)
			if res := classifyTelnetRefusal(result, second); res != nil {
				return res
			}
			bannerAndFinished = append(bannerAndFinished, second...)
			u = strings.TrimSpace(string(bannerAndFinished))
			if !utils.MatchAnyOfRegexp(u, `(?i)login`, `(?i)user`) {
				if utils.MatchAnyOfRegexp(u, `(?i)password`) {
					finalResult := doPassword()
					finalResult.OnlyNeedPassword = true
					return finalResult
				}
				return result
			}
		}

		conn.Write([]byte(i.Username + "\n"))
		return doPassword()
	},
}

// telnetReadUntil 流式读取，双出口：
//  1. 内容匹配任一模式（提示符/判定词/拒绝特征）→ 立即返回；
//  2. 已读到数据且连续两个静默窗口（共 ~600ms）无新增 → banner 已
//     发完且无等待词，返回已读内容（旧实现等满超时，真实无提示符
//     设备单阶段 ~10s）。
//
// EOF/错误/总超时同样返回已读内容。
func telnetReadUntil(conn net.Conn, timeout time.Duration, patterns ...string) []byte {
	var buf bytes.Buffer
	ddl := time.Now().Add(timeout)
	ch := make([]byte, 1)
	quiet := 0
	for time.Now().Before(ddl) {
		_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		n, err := conn.Read(ch)
		if n > 0 {
			buf.Write(ch[:n])
			quiet = 0
			content := buf.String()
			if utils.MatchAnyOfRegexp(content, patterns...) {
				break
			}
			continue
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				quiet++
				if buf.Len() > 0 && quiet >= 2 {
					break // 数据已读完且无匹配词：稳定
				}
				if buf.Len() == 0 && quiet >= 4 {
					break // 1.2s 无任何字节：哑连接，快速放弃
				}
				continue
			}
			break // EOF / 连接错误
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	return buf.Bytes()
}

// classifyTelnetRefusal 识别真实 telnet 设备的明确拒绝特征
// （Shodan 100 样本语料，37% 无标准提示符）：
//   - 爆破锁定：Protection of brute force attack!! Lockout remaining:
//     TELNET[ppp0] N seconds（华为系等）→ AccountLocked，调度器短路目标
//   - 连接资源受限：connections exceed 5 / maximum number of telnet
//     sessions / no more connections → Finished，目标暂不可用
//
// 命中返回已标记的 result，未命中返回 nil（继续正常流程）。
func classifyTelnetRefusal(result *BruteItemResult, raw []byte) *BruteItemResult {
	s := string(raw)
	if utils.MatchAnyOfRegexp(s,
		`(?i)lockout remaining`, `(?i)protection of brute force`,
		`(?i)locked out`, `(?i)account.{0,10}locked`, `(?i)too many.{0,20}(fail|attempt|login)`) {
		result.AccountLocked = true
		result.ExtraInfo = raw
		return result
	}
	if utils.MatchAnyOfRegexp(s,
		`(?i)connections? exceed`, `(?i)maximum number of.{0,30}session`,
		`(?i)no more connections`, `(?i)too many (connections|users|sessions)`,
		`(?i)telnet.*(disabled|not enabled)`) {
		result.Finished = true
		result.ExtraInfo = raw
		return result
	}
	return nil
}
