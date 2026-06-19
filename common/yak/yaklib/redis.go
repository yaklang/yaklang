package yaklib

import (
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/redis"
)

type redisConfig struct {
	Host           string
	Port           int
	Password       string
	Username       string
	TimeoutSeconds int
	MaxRetries     int
}

type redisConfigOpt func(i *redisConfig)

// host 是一个 Redis 客户端配置选项，用于设置 Redis 服务器主机地址
// 参数:
//   - h: Redis 服务器主机地址
//
// 返回值:
//   - 一个 Redis 客户端配置选项，作为可变参数传入 redis.New
//
// Example:
// ```
// // 指定 Redis 主机与端口创建客户端，此处仅作示意
// client = redis.New(redis.host("127.0.0.1"), redis.port(6379))
// defer client.Close()
// ```
func redisOpt_Host(h string) redisConfigOpt {
	return func(i *redisConfig) {
		i.Host = h
	}
}

// port 是一个 Redis 客户端配置选项，用于设置 Redis 服务器端口
// 参数:
//   - h: Redis 服务器端口，默认 6379
//
// 返回值:
//   - 一个 Redis 客户端配置选项，作为可变参数传入 redis.New
//
// Example:
// ```
// // 指定 Redis 端口创建客户端，此处仅作示意
// client = redis.New(redis.host("127.0.0.1"), redis.port(6380))
// defer client.Close()
// ```
func redisOpt_Port(h int) redisConfigOpt {
	return func(i *redisConfig) {
		i.Port = h
	}
}

// addr 是一个 Redis 客户端配置选项，用于以 host:port 形式同时设置主机与端口
// 参数:
//   - a: Redis 服务器地址，格式为 host:port
//
// 返回值:
//   - 一个 Redis 客户端配置选项，作为可变参数传入 redis.New
//
// Example:
// ```
// // 以 host:port 形式创建 Redis 客户端，此处仅作示意
// client = redis.New(redis.addr("127.0.0.1:6379"))
// defer client.Close()
// ```
func redisOpt_Addr(a string) redisConfigOpt {
	return func(i *redisConfig) {
		i.Host, i.Port, _ = utils.ParseStringToHostPort(a)
	}
}

// username 是一个 Redis 客户端配置选项，用于设置认证用户名（Redis 6.0+ ACL）
// 参数:
//   - a: 认证用户名
//
// 返回值:
//   - 一个 Redis 客户端配置选项，作为可变参数传入 redis.New
//
// Example:
// ```
// // 指定用户名与密码创建 Redis 客户端，此处仅作示意
// client = redis.New(redis.addr("127.0.0.1:6379"), redis.username("default"), redis.password("123456"))
// defer client.Close()
// ```
func redisOpt_Username(a string) redisConfigOpt {
	// TODO: redis ACL auth support
	return func(i *redisConfig) {
		i.Username = a
	}
}

// password 是一个 Redis 客户端配置选项，用于设置认证密码
// 参数:
//   - a: 认证密码
//
// 返回值:
//   - 一个 Redis 客户端配置选项，作为可变参数传入 redis.New
//
// Example:
// ```
// // 指定密码创建 Redis 客户端，此处仅作示意
// client = redis.New(redis.addr("127.0.0.1:6379"), redis.password("123456"))
// defer client.Close()
// ```
func redisOpt_Password(a string) redisConfigOpt {
	return func(i *redisConfig) {
		i.Password = a
	}
}

// retry 是一个 Redis 客户端配置选项，用于设置连接的最大重试次数
// 参数:
//   - a: 最大重试次数
//
// 返回值:
//   - 一个 Redis 客户端配置选项，作为可变参数传入 redis.New
//
// Example:
// ```
// // 设置连接重试次数创建 Redis 客户端，此处仅作示意
// client = redis.New(redis.addr("127.0.0.1:6379"), redis.retry(5))
// defer client.Close()
// ```
func redisOpt_Retry(a int) redisConfigOpt {
	return func(i *redisConfig) {
		i.MaxRetries = a
	}
}

// timeoutSeconds 是一个 Redis 客户端配置选项，用于设置连接与读写超时（单位：秒）
// 参数:
//   - d: 超时时间，单位为秒
//
// 返回值:
//   - 一个 Redis 客户端配置选项，作为可变参数传入 redis.New
//
// Example:
// ```
// // 设置超时创建 Redis 客户端，此处仅作示意
// client = redis.New(redis.addr("127.0.0.1:6379"), redis.timeoutSeconds(5))
// defer client.Close()
// ```
func redisOpt_TimeoutSeconds(d int) redisConfigOpt {
	return func(i *redisConfig) {
		i.TimeoutSeconds = d
	}
}

var RedisExports = map[string]interface{}{
	"New":            newRedis,
	"host":           redisOpt_Host,
	"port":           redisOpt_Port,
	"addr":           redisOpt_Addr,
	"username":       redisOpt_Username,
	"password":       redisOpt_Password,
	"timeoutSeconds": redisOpt_TimeoutSeconds,
	"retry":          redisOpt_Retry,
}

type redisClient struct {
	config    *redisConfig
	rawClient *redis.Client
}

func (r *redisClient) Get(i interface{}) (string, error) {
	b, err := r.rawClient.Get(utils.InterfaceToString(i))
	return string(b), err
}

func (r *redisClient) GetEx(i interface{}, ttlSeconds int) (string, error) {
	b, err := r.rawClient.GetEx(utils.InterfaceToString(i), time.Duration(time.Second)*time.Duration(ttlSeconds))
	return string(b), err
}

func (r *redisClient) Set(k interface{}, value interface{}) error {
	return r.rawClient.Set(utils.InterfaceToString(k), utils.InterfaceToString(value))
}

func (r *redisClient) SetWithTTL(k interface{}, value interface{}, ttlSeconds int) error {
	return r.rawClient.SetEx(utils.InterfaceToString(k), utils.InterfaceToString(value), time.Duration(time.Second)*time.Duration(ttlSeconds))
}

func (r *redisClient) Publish(channel string, msg string) error {
	return r.rawClient.Publish(channel, msg)
}

func (r *redisClient) Subscribe(channel string, cb func(msg *redis.Message)) (closeFunc func()) {
	subscribeCh := make(chan string)
	messageCh := make(chan redis.Message)
	go func() {
		subscribeCh <- channel
		for {
			select {
			case msg := <-messageCh:
				if cb != nil {
					cb(&msg)
				}
			case _, ok := <-subscribeCh:
				if !ok {
					close(messageCh)
					return
				}
			}
		}
	}()

	go func() {
		r.rawClient.Subscribe(subscribeCh, nil, nil, nil, messageCh)
	}()
	return func() {
		close(subscribeCh)
	}
}

func (r *redisClient) Close() error {
	return r.rawClient.Close()
}

func (r *redisClient) Do(items ...interface{}) {
	if len(items) < 1 {
		return
	}
	sItems := lo.Map(items, func(i any, _ int) string {
		return utils.InterfaceToString(i)
	})
	r.rawClient.Do(sItems[0], sItems[1:]...)
}

// New 创建一个 Redis 客户端，可通过配置选项指定地址、认证、超时等参数
// 参数:
//   - r: 可选配置，例如 redis.addr、redis.host、redis.port、redis.password、redis.timeoutSeconds
//
// 返回值:
//   - Redis 客户端对象，可调用 Get/Set/Publish/Subscribe 等方法
//
// Example:
// ```
// // 创建 Redis 客户端并读写键值，依赖 Redis 服务，此处仅作示意
// client = redis.New(redis.addr("127.0.0.1:6379"), redis.timeoutSeconds(5))
// defer client.Close()
// client.Set("key", "value")~
// val = client.Get("key")~
// println(val)
// ```
func newRedis(r ...redisConfigOpt) *redisClient {
	config := &redisConfig{
		Host:           "127.0.0.1",
		Port:           6379,
		Username:       "",
		Password:       "",
		TimeoutSeconds: 10,
		MaxRetries:     3,
	}

	for _, opt := range r {
		opt(config)
	}
	if config.Port <= 0 {
		config.Port = 6379
	}
	if config.TimeoutSeconds <= 0 {
		config.TimeoutSeconds = 10
	}

	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	conn, err := netx.DialX(utils.HostPort(config.Host, config.Port), netx.DialX_WithTimeout(timeout), netx.DialX_WithTimeoutRetry(3))
	if err != nil {
		log.Errorf("redis dial connection failed: %v", err)
	}

	client := redis.NewClient(conn, timeout)
	return &redisClient{rawClient: client, config: config}
}
