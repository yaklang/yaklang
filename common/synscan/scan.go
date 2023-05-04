package synscan

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

func (s *Scanner) OnSubmitTask(i func(addr string, port int)) {
	s.onSubmitTaskCallback = i
}

func (s *Scanner) callOnSubmitTask(addr string, port int) {
	if s == nil {
		return
	}

	if s.onSubmitTaskCallback == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("on submit task callback failed: %s", err)
		}
	}()

	s.onSubmitTaskCallback(addr, port)
}

func (s *Scanner) scanPublic(publicHosts []string, ports []int, random bool) error {
	// 获取网关的 mac 地址作为目的 Mac
	// 当前网卡为 mac 源
	type pair struct {
		host string
		port int
	}
	var publicPairs []*pair
	for _, host := range publicHosts {
		for _, port := range ports {
			publicPairs = append(publicPairs, &pair{host, port})
		}
	}

	if random {
		rand.Shuffle(len(publicPairs), func(i, j int) {
			publicPairs[i], publicPairs[j] = publicPairs[j], publicPairs[i]
		})
	}

	swg := utils.NewSizedWaitGroup(1000)
	defer swg.Wait()

	for _, i := range publicPairs {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}

		// 设置回调函数
		s.callOnSubmitTask(i.host, i.port)

		i := i
		dstTarget := utils.FixForParseIP(i.host)
		if dstIp := net.ParseIP(dstTarget); dstIp != nil {
			swg.Add()
			go func() {
				defer swg.Done()

				log.Debugf("create syn packet for %v", utils.HostPort(dstIp.String(), i.port))
				layers, loopback, err := s.createSynTCP(dstIp, i.port, nil, s.defaultGatewayIp.String())
				if err != nil {
					log.Warnf("cannot create syn-tcp packet for %s:%v err: %v", dstIp.String(), i.port, err)
					return
				}
				s.inject(loopback, layers...)
			}()

		} else {
			err := swg.AddWithContext(s.ctx)
			if err != nil {
				return err
			}
			go func() {
				defer swg.Done()
				addrs := utils.DomainToIP(dstTarget, 3*time.Second)
				if len(addrs) > 0 {
					for _, ip := range addrs {
						if dstIp := net.ParseIP(ip); dstIp != nil {
							layers, loopback, err := s.createSynTCP(dstIp, i.port, nil, s.defaultGatewayIp.String())
							if err != nil {
								log.Warnf("cannot create syn-tcp packet for %s:%v: %v", dstIp.String(), i.port, err)
								continue
							}
							s.inject(loopback, layers...)
						}
					}
				} else {
					log.Warnf("cannot query dns for %v", dstTarget)
				}
			}()
		}
	}
	return nil
}

func (s *Scanner) scanPrivate(privateHosts []string, ports []int, random bool) error {
	log.Infof("private net scan need use arp to locate mac addr")

	ctx := utils.TimeoutContextSeconds(5)
	results, err := utils.ArpIPAddressesWithContext(ctx, s.iface.Name, strings.Join(privateHosts, ","))
	if err != nil {
		log.Errorf("create arp results from privateHosts failed: %s", err)
		return err
	}

	// 打乱端口
	if random {
		rand.Shuffle(len(ports), func(i, j int) {
			ports[i], ports[j] = ports[j], ports[i]
		})
	}

	// 控制一点并发
	packetSwg := utils.NewSizedWaitGroup(80)
	defer packetSwg.Wait()

	// 进行统计
	var count, total int64
	for targetIP, hwAddr := range results {
		hwAddr := hwAddr
		targetIP := targetIP

		dstIP := net.ParseIP(utils.FixForParseIP(targetIP))
		if dstIP == nil {
			continue
		}

		for _, port := range ports {
			// 设置回调函数
			s.callOnSubmitTask(targetIP, port)
			port := port
			packetSwg.Add()
			go func() {
				defer packetSwg.Done()
				layers, loopback, err := s.createSynTCP(dstIP, port, hwAddr, "")
				if err != nil {
					log.Warnf("cannot create syn-tcp packet for %s err: %s", utils.HostPort(dstIP.String(), port), err)
					return
				}

				log.Debugf("start to inject %v with loopback: %v", hwAddr.String(), loopback)
				err = s.inject(loopback, layers...)
				if err != nil {
					log.Errorf("inject syn-tcp packet error: %s", err)
				}
				atomic.AddInt64(&count, 1)
				atomic.AddInt64(&total, 1)
				if count >= 5000 {
					count = 0
					log.Infof("syn scanner sent 5000 packets... total: %v", total)
				}
			}()
		}
	}

	return nil
}

func (s *Scanner) scan(host string, port string, random bool, noWait bool) error {
	hosts := utils.ParseStringToHosts(host)
	ports := utils.ParseStringToPorts(port)

	addrs, err := s.iface.Addrs()
	if err != nil {
		return utils.Errorf("iface: %s has no local addrs", s.iface.Name)
	}
	var privateHosts []string
	var publicHosts []string
	var localhost []string
	for _, host := range hosts {
		if utils.IsLoopback(host) {
			localhost = append(localhost, host)
			continue
		}

		// 判断是不是当前网卡内网的地址？如果是，就添加到内网扫描中
		// 内网扫描需要先去找 MAC 地址
		setPrivate := false
		for _, addr := range addrs {
			ifNet, ok := addr.(*net.IPNet)
			targetHost := net.ParseIP(host)
			if ok && targetHost != nil && ifNet.Contains(targetHost) {
				privateHosts = append(privateHosts, host)
				setPrivate = true
				break
			}
		}

		// 公网扫描，一般来说网管地址就是目的 MAC，不需要额外处理
		if !setPrivate {
			publicHosts = append(publicHosts, host)
		}
	}

	if localhost != nil {
		log.Infof("start to scan localhost: %v", localhost)
		err := s.scanPublic(localhost, ports, random)
		if err != nil {
			log.Errorf("scan localhost failed: %s", err)
		}
	}

	if privateHosts != nil {
		log.Infof("start to scan private hosts: %v", len(privateHosts))
		err = s.scanPrivate(privateHosts, ports, random)
		if err != nil {
			log.Errorf("scan private failed: %s", err)
		}
	}

	if publicHosts != nil {
		//log.Infof("start to scan public hosts: %v", len(publicHosts))
		err = s.scanPublic(publicHosts, ports, random)
		if err != nil {
			return err
		}
	}

	if !noWait {
		s._waitChanEmpty()
	}

	return nil
}

func (s *Scanner) WaitChannelEmpty() {
	s._waitChanEmpty()
}

func (s *Scanner) _waitChanEmpty() {
	log.Infof("start to wait all packets are sent")
	for {
		haveResult := true
		select {
		case p := <-s.localHandlerWriteChan:
			s.localHandlerWriteChan <- p
		case p := <-s.handlerWriteChan:
			s.handlerWriteChan <- p
		default:
			haveResult = false
		}
		time.Sleep(200 * time.Millisecond)
		if !haveResult {
			return
		}
	}
}

func (s *Scanner) RandomScan(host string, port string, noWait bool) error {
	return s.scan(host, port, true, noWait)
}

func (s *Scanner) Scan(host string, port string, noWait bool) error {
	return s.scan(host, port, false, noWait)
}
