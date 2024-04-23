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

func redisOpt_Host(h string) redisConfigOpt {
	return func(i *redisConfig) {
		i.Host = h
	}
}

func redisOpt_Port(h int) redisConfigOpt {
	return func(i *redisConfig) {
		i.Port = h
	}
}

func redisOpt_Addr(a string) redisConfigOpt {
	return func(i *redisConfig) {
		i.Host, i.Port, _ = utils.ParseStringToHostPort(a)
	}
}

func redisOpt_Username(a string) redisConfigOpt {
	// TODO: redis ACL auth support
	return func(i *redisConfig) {
		i.Username = a
	}
}

func redisOpt_Password(a string) redisConfigOpt {
	return func(i *redisConfig) {
		i.Password = a
	}
}

func redisOpt_Retry(a int) redisConfigOpt {
	return func(i *redisConfig) {
		i.MaxRetries = a
	}
}

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
