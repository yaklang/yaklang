package bruteutils

import (
	"context"
	"github.com/go-redis/redis/v8"
	"yaklang/common/log"
	"yaklang/common/utils"
	"strings"
	"time"
)

var redisAuth = &DefaultServiceAuthInfo{
	ServiceName:      "redis",
	DefaultPorts:     "6379",
	DefaultUsernames: append([]string{"redis"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		i.Target = appendDefaultPort(i.Target, 6379)
		conn, err := DefaultDailer.Dial("tcp", i.Target)
		if err != nil {
			res := i.Result()
			res.Finished = true
			res.OnlyNeedPassword = true
			return res
		}
		conn.Close()

		// 107.187.110.241/24
		rdb := redis.NewClient(&redis.Options{Addr: i.Target})
		rkey := utils.RandStringBytes(10)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rdb.Set(ctx, rkey, rkey+rkey, 10*time.Second)
		var data = rdb.Get(ctx, rkey)
		if result, _ := data.Result(); result == rkey+rkey {
			res := i.Result()
			res.Ok = true
			res.Username = "-"
			res.OnlyNeedPassword = true
			res.Finished = true
			return res
		}
		r := i.Result()
		r.OnlyNeedPassword = true
		return r
	},
	BrutePass: func(item *BruteItem) *BruteItemResult {
		item.Target = appendDefaultPort(item.Target, 6379)

		result := item.Result()
		result.OnlyNeedPassword = true
		result.Username = "-"

		rdb := redis.NewClient(&redis.Options{
			Addr:     item.Target,
			Password: item.Password,
		})
		k := utils.RandStringBytes(12)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rdb.Set(ctx, k, k+k, 15*time.Second)
		var data = rdb.Get(ctx, k)
		if result, _ := data.Result(); result == k+k {
			res := item.Result()
			res.Ok = true
			res.Username = "-"
			res.OnlyNeedPassword = true
			res.Finished = true
			return res
		}

		err := data.Err()
		switch true {
		case strings.Contains(err.Error(), "connect: connection refused"):
			result.Finished = true
			return result
		case utils.MatchAllOfSubString(err.Error(), `ERR AUTH`, `called without any password configured for the default user`):
			result.Finished = true
			return result
		}
		if err != nil {

			log.Errorf("execute redis set %s failed: %s", k, err)
			return result
		}
		result.Ok = false
		return result
	},
}
