package aibalance

import (
	"context"
	"net"
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"gopkg.in/yaml.v3"
)

type Balancer struct {
	config   *ServerConfig
	listener net.Listener
	mutex    sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewBalancerFromRawConfig(raw []byte, files ...string) (*Balancer, error) {
	var configFile = "[::memory::]"
	if len(files) > 0 {
		configFile = files[0]
	}
	var ymlConfig YamlConfig
	if err := yaml.Unmarshal(raw, &ymlConfig); err != nil {
		return nil, utils.Wrapf(err, "Unmarshal config file %s error", configFile)
	}

	serverConfig, err := ymlConfig.ToServerConfig()
	if err != nil {
		return nil, utils.Wrapf(err, "cannot convert yaml config file to server %s", configFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &Balancer{
		config: serverConfig,
		ctx:    ctx,
		cancel: cancel,
	}
	return b, nil
}

func NewBalancer(configFile string) (*Balancer, error) {
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return nil, utils.Errorf("Read config file %s error: %v", configFile, err)
	}
	return NewBalancerFromRawConfig(raw)
}

func (b *Balancer) RunWithPort(port int) error {
	if port <= 0 {
		return utils.Errorf("invalid port %d", port)
	}
	return b.run(utils.HostPort("0.0.0.0", port))
}

func (b *Balancer) RunWithAddr(addr string) error {
	if addr == "" {
		return utils.Errorf("invalid address %s", addr)
	}
	return b.run(addr)
}

func (b *Balancer) Run() error {
	return b.run(utils.HostPort("127.0.0.1", 80))
}

func (b *Balancer) run(addr string) error {
	b.mutex.Lock()
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		b.mutex.Unlock()
		return err
	}
	b.listener = lis
	b.mutex.Unlock()

	go func() {
		<-b.ctx.Done()
		log.Infof("balancer context is done, closing listener...")
		if b.listener != nil {
			b.listener.Close()
		}
	}()

	for {
		conn, err := lis.Accept()
		if err != nil {
			if b.ctx.Err() != nil {
				return nil // 正常关闭
			}
			return err
		}
		go func() {
			defer conn.Close()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("panic recover %v", utils.ErrorStack(err))
				}
			}()
			b.config.Serve(conn)
		}()
	}
}

// Close 关闭 balancer 并释放资源
func (b *Balancer) Close() error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.cancel != nil {
		b.cancel()
	}

	if b.listener != nil {
		return b.listener.Close()
	}

	return nil
}
