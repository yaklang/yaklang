package yaklib

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	if r == nil || r.rawClient == nil {
		return "", utils.Error("no client set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.TimeoutSeconds)*time.Second)
	defer cancel()

	data := r.rawClient.Get(ctx, utils.InterfaceToString(i))
	if data.Err() != nil {
		return "", utils.Errorf("redis `get(%v)` failed: %s", i, data.Err())
	}
	return data.String(), nil
}

func (r *redisClient) GetEx(i interface{}, ttlSeconds int) (string, error) {
	if r == nil || r.rawClient == nil {
		return "", utils.Error("no client set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.TimeoutSeconds)*time.Second)
	defer cancel()

	data := r.rawClient.GetEx(ctx, utils.InterfaceToString(i), time.Duration(ttlSeconds)*time.Second)
	if data.Err() != nil {
		return "", utils.Errorf("redis `get(%v)` failed: %s", i, data.Err())
	}
	return data.String(), nil
}

func (r *redisClient) Set(k interface{}, value interface{}) error {
	if r == nil || r.rawClient == nil {
		return utils.Error("no client set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.TimeoutSeconds)*time.Second)
	defer cancel()

	status := r.rawClient.Set(ctx, utils.InterfaceToString(k), utils.InterfaceToString(value), 0)
	return status.Err()
}

func (r *redisClient) SetWithTTL(k interface{}, value interface{}, ttlSeconds int) error {
	if r == nil || r.rawClient == nil {
		return utils.Error("no client set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.TimeoutSeconds)*time.Second)
	defer cancel()

	status := r.rawClient.Set(ctx, utils.InterfaceToString(k), utils.InterfaceToString(value), time.Duration(ttlSeconds)*time.Second)
	return status.Err()
}

func (r *redisClient) Publish(channel string, msg string) error {
	if r == nil || r.rawClient == nil {
		return utils.Error("no client set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.TimeoutSeconds)*time.Second)
	defer cancel()

	status := r.rawClient.Publish(ctx, "test", "test")
	return status.Err()
}

func (r *redisClient) Do(items ...interface{}) {
	if r == nil || r.rawClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.TimeoutSeconds)*time.Second)
	defer cancel()

	status := r.rawClient.Do(ctx, items...)
	if status.Err() != nil {
		log.Errorf("redis do ... failed: %s", status.Err())
	}
}

func (r *redisClient) Subscribe(channel string, cb func(msg *redis.Message)) (fe error) {
	if r == nil || r.rawClient == nil {
		return utils.Error("no client set")
	}

	defer func() {
		if err := recover(); err != nil {
			fe = errors.Errorf("subcribe[%v] panic: %v", channel, err)
			log.Error(fe)
			return
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.TimeoutSeconds)*time.Second)
	defer cancel()

	subc := r.rawClient.Subscribe(ctx, channel)
	defer subc.Close()

	for {
		msg, err := subc.ReceiveMessage(context.Background())
		if err != nil {
			return errors.Wrap(err, "subpub receive msg failed")
		}

		if msg != nil && cb != nil {
			cb(msg)
		}
	}
}

func (r *redisClient) Close() error {
	if r == nil || r.rawClient == nil {
		return utils.Error("no client set")
	}

	return r.rawClient.Close()
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
	redisConfig := &redis.Options{
		Addr:         utils.HostPort(config.Host, config.Port),
		Username:     config.Username,
		Password:     config.Password,
		MaxRetries:   config.MaxRetries,
		DialTimeout:  timeout,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		// TLSConfig:    &tls.Config{InsecureSkipVerify: true},
	}
	client := redis.NewClient(redisConfig)
	return &redisClient{rawClient: client, config: config}
}
