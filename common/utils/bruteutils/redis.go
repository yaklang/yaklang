package bruteutils

import (
	"net"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/redis"
)

func RedisAuth(target, password string, needAuth bool) (bool, error) {
	conn, err := defaultDialer.DialContext(utils.TimeoutContext(defaultTimeout), "tcp", target)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	rdb := redis.NewClient(conn, defaultTimeout)
	if needAuth {
		err := rdb.Auth(password)
		if err != nil {
			return false, err
		}
	}

	randomKey := utils.RandStringBytes(10)

	err = rdb.Set(randomKey, randomKey+randomKey)
	if err != nil {
		return false, err
	}

	b, err := rdb.Get(randomKey)
	if err != nil {
		return false, err
	}

	if string(b) == randomKey+randomKey {
		return true, nil
	}
	return false, nil
}

var redisAuth = &DefaultServiceAuthInfo{
	ServiceName:      "redis",
	DefaultPorts:     "6379",
	DefaultUsernames: append([]string{"redis"}, CommonUsernames...),
	// DefaultPasswords: CommonPasswords,
	DefaultPasswords: []string{"test"},
	UnAuthVerify: func(item *BruteItem) *BruteItemResult {
		item.Target = appendDefaultPort(item.Target, 6379)
		ok, err := RedisAuth(item.Target, "", false)
		res := item.Result()
		if err != nil {
			if _, ok := err.(net.Error); ok {
				res.Finished = true
				return res
			}
		}

		if ok {
			res.Ok = true
			res.Username = "-"
			res.OnlyNeedPassword = true
		}
		return res
	},

	BrutePass: func(item *BruteItem) *BruteItemResult {
		item.Target = appendDefaultPort(item.Target, 6379)
		ok, err := RedisAuth(item.Target, item.Password, true)
		res := item.Result()
		if err != nil {
			if _, ok := err.(net.Error); ok {
				res.Finished = true
				return res
			}
		}

		if ok {
			res.Ok = true
			res.Username = "-"
			res.OnlyNeedPassword = true
		}
		return res
	},
}
