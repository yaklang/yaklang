package bruteutils

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"sync"
	"time"
)

const CommonTimeoutDuration = 8 * time.Second

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

		conn, err := utils.GetAutoProxyConnEx(i.Target, nil, CommonTimeoutDuration)
		if err != nil {
			log.Debugf("telnet:\\\\%v conn failed: %s", i.Target, err)
			res := i.Result()
			res.Finished = true
			return res
		}
		defer conn.Close()

		raw := utils.StableReaderEx(conn, CommonTimeoutDuration, 1024)
		if raw == nil {
			res := i.Result()
			res.Finished = true
			return res
		}

		conn.Write([]byte("?\n"))
		raw = utils.StableReaderEx(conn, CommonTimeoutDuration, 4096)
		if raw == nil {
			return i.Result()
		}

		if utils.MatchAllOfRegexp(string(raw), "(?)route", "aaa", "ip") ||
			utils.MatchAllOfSubString(string(raw), "UNAUTHORIZED ACCESS TO THIS DEVICE") ||
			utils.MatchAnyOfSubString(string(raw), `prompt for`) ||
			utils.MatchAllOfSubString(string(raw), "We Monitor Our Traffic") ||
			utils.MatchAllOfSubString(string(raw), "THDCR001SW23>") {
			var r = i.Result()
			r.Ok = true
			r.Username = ""
			r.Password = ""
			r.ExtraInfo = raw
			return r
		} else {
			log.Infof("===============%v================", i.Target)
			spew.Dump(raw)
			log.Info("===========================================")
		}

		return i.Result()
	},
	BrutePass: func(item *BruteItem) *BruteItemResult {
		if item.Password == "" && item.Username == "" {
			log.Info("empty username and password")
		}
		log.Debugf("telnet client start to handle: %s", item.String())
		defer log.Debugf("telnet finished to handle: %s", item.String())

		result := item.Result()
		var target = fixToTarget(item.Target, 23)

		defer func() {
			if err := recover(); err != nil {
				log.Error("telnet panic: %s", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		var mutex *sync.Mutex
		val, ok := telnetHostlock.Load(item.Target)
		if ok {
			mutex = val.(*sync.Mutex)
		} else {
			mutex = new(sync.Mutex)
			telnetHostlock.Store(item.Target, mutex)
		}
		mutex.Lock()
		defer mutex.Unlock()

		var conn, err = utils.GetAutoProxyConnEx(target, nil, CommonTimeoutDuration)
		if err != nil {
			log.Error("get auto proxy conn ex failed: %s", err)
			if utils.MatchAnyOfRegexp(err.Error(), `(?i)timeout`) {
				return result
			}
			return result
		}

		defer conn.Close()

		var doPassword = func() *BruteItemResult {
			var passRaw = utils.StableReaderEx(conn, CommonTimeoutDuration, 1024)
			if utils.MatchAnyOfRegexp(string(passRaw), `(?i)password`, `(?i)verification code:`) {
				conn.Write([]byte(item.Password + "\n"))
				var bruteResult = utils.StableReaderEx(conn, CommonTimeoutDuration, 1024)
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

		var bannerAndFinished = utils.StableReaderEx(conn, CommonTimeoutDuration, 1024)
		var u = strings.TrimSpace(string(bannerAndFinished))
		if !utils.MatchAnyOfRegexp(u, `(?i)login`, `(?i)user`) {
			// 没有匹配到 login 或者 user，看是不是匹配到 password
			if utils.MatchAnyOfRegexp(u, `(?i)password`) {
				var finalResult = doPassword()
				finalResult.OnlyNeedPassword = true
				return finalResult
			}
			return result
		}

		conn.Write([]byte(item.Username + "\n"))
		return doPassword()
	},
}
