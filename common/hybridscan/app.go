package hybridscan

import (
	"context"
	"github.com/pkg/errors"
	"net"
	"sync"
	"yaklang.io/yaklang/common/fp"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/synscan"
)

type HyperScanCenter struct {
	config *Config
	ctx    context.Context
	cancel context.CancelFunc

	synScanner     *synscan.Scanner
	fpScanPool     *fp.Pool
	fpTargetStream chan *fp.PoolTask

	// 指纹回调函数的处理
	fpResultHandlerMutex *sync.Mutex
	fpResultHandlers     map[string]fp.PoolCallback

	// 开放端口回调
	openPortHandlerMutex *sync.Mutex
	openPortHandlers     map[string]func(ip net.IP, port int)
}

func NewHyperScanCenter(ctx context.Context, config *Config) (*HyperScanCenter, error) {
	nCtx, cancel := context.WithCancel(ctx)

	center := &HyperScanCenter{
		config: config,
		cancel: cancel,
		ctx:    nCtx,

		fpResultHandlerMutex: new(sync.Mutex),
		fpResultHandlers:     make(map[string]fp.PoolCallback),
		openPortHandlerMutex: new(sync.Mutex),
		openPortHandlers:     make(map[string]func(ip net.IP, port int)),
	}

	if config.SynScanConfig == nil {
		return nil, errors.New("empty syn scan config")
	}

	sscan, err := synscan.NewScanner(nCtx, config.SynScanConfig)
	if err != nil {
		return nil, errors.Errorf("create syn scanner failed: %s", err)
	}
	center.synScanner = sscan

	center.fpTargetStream = make(chan *fp.PoolTask, config.FingerprintMatchQueueBuffer)
	center.fpScanPool, err = fp.NewExecutingPool(nCtx, 30, center.fpTargetStream, config.FingerprintMatcherConfig)
	if err != nil {
		return nil, errors.Errorf("create fp executing pool failed: %s", err)
	}
	center.fpScanPool.AddCallback(center.onMatcherResult)
	err = center.synScanner.RegisterSynAckHandler("daemon-tag", center.onOpenPort)
	if err != nil {
		cancel()
		return nil, errors.Errorf("register open port handler failed: %s", err)
	}
	go func() {
		err := center.fpScanPool.Run()
		if err != nil {
			log.Info("fp scan pool run failed: %s", err)
		}
	}()

	return center, nil
}

func (c *HyperScanCenter) SetSynScanRateLimit(ms float64, count int) {
	if c.synScanner == nil {
		log.Warnf("cannot set rate-limit %v(ms)/%v(gap)", ms, count)
		return
	}
	c.synScanner.SetRateLimit(ms, count)
}

func (c *HyperScanCenter) Close() {
	defer c.cancel()

	if c.synScanner != nil {
		c.synScanner.Close()
	}

	c.fpScanPool.Close()
	close(c.fpTargetStream)
}

func (c *HyperScanCenter) GetFingerprintScanPool() *fp.Pool {
	return c.fpScanPool
}

func (c *HyperScanCenter) GetSYNScanner() *synscan.Scanner {
	return c.synScanner
}
