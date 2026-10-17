package synscanx

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
)

// Scan is the single SYN/UDP scan entry used by the Yak synscan library and
// the synscan CLIs. The older synscan.Scanner plus hybridscan path is gone.
func Scan(ctx context.Context, targets, ports string, opts ...SynxConfigOption) (chan *synscan.SynScanResult, error) {
	config := NewDefaultConfig()
	if ctx != nil {
		config.Ctx = ctx
	}
	for _, opt := range opts {
		if opt != nil {
			opt(config)
		}
	}
	return ScanWithConfig(targets, ports, config)
}

// ScanWithConfig runs one scan with a config that already has its options applied.
func ScanWithConfig(targets, ports string, config *SynxConfig) (chan *synscan.SynScanResult, error) {
	if config == nil {
		config = NewDefaultConfig()
	}
	if config.Ctx == nil {
		config.Ctx = context.Background()
	}
	if config.netInterface != "" {
		sample := chooseRouteSample(targets)
		if sample == "" {
			return nil, errors.New("empty target")
		}
		return scanOne(sample, targets, ports, config)
	}
	groups := partitionScanTargets(targets)
	if len(groups) == 0 {
		return nil, errors.New("empty target")
	}
	if len(groups) == 1 {
		return scanOne(groups[0].sample, strings.Join(groups[0].hosts, ","), ports, config)
	}
	return scanRouteGroups(groups, ports, config)
}

func scanOne(sample, targets, ports string, config *SynxConfig) (chan *synscan.SynScanResult, error) {
	scanner, err := NewScannerx(config.Ctx, sample, config)
	if err != nil {
		return nil, err
	}
	targetCh, err := scanner.SubmitTarget(targets, ports)
	if err != nil {
		return nil, err
	}
	resultCh, err := scanner.Scan(targetCh)
	if err != nil {
		log.Errorf("scan failed: %s", err)
		return nil, err
	}
	return resultCh, nil
}

type routeGroup struct {
	sample string
	hosts  []string
}

// partitionScanTargets keeps loopback on the session that also opens the
// loopback device, and splits a modest target list when those addresses leave
// through different interfaces. A large CIDR stays on one route.
func partitionScanTargets(targets string) []routeGroup {
	hosts := utils.ParseStringToHosts(targets)
	var loopback, others []string
	skippedV6 := 0
	for _, host := range hosts {
		switch {
		case utils.IsIPv6(host):
			skippedV6++
		case utils.IsLoopback(host):
			loopback = append(loopback, host)
		default:
			others = append(others, host)
		}
	}
	if skippedV6 > 0 {
		log.Infof("synscanx: skipped %d ipv6 targets; probes are ipv4-only", skippedV6)
	}
	if len(others) == 0 {
		if len(loopback) == 0 {
			return nil
		}
		return []routeGroup{{sample: loopback[0], hosts: loopback}}
	}
	if len(others) > 64 {
		all := append(append([]string{}, others...), loopback...)
		return []routeGroup{{sample: others[0], hosts: all}}
	}
	order := make([]string, 0, 4)
	buckets := make(map[string]*routeGroup)
	var failed []string
	for _, host := range others {
		iface, _, src, err := getRoute(host)
		if err != nil || iface == nil {
			failed = append(failed, host)
			continue
		}
		srcKey := ""
		if src != nil {
			srcKey = src.String()
		}
		key := iface.Name + "|" + srcKey
		group := buckets[key]
		if group == nil {
			group = &routeGroup{sample: host}
			buckets[key] = group
			order = append(order, key)
		}
		group.hosts = append(group.hosts, host)
	}
	groups := make([]routeGroup, 0, len(order)+1)
	for _, key := range order {
		groups = append(groups, *buckets[key])
	}
	if len(failed) > 0 {
		groups = append(groups, routeGroup{sample: failed[0], hosts: failed})
	}
	if len(loopback) > 0 {
		if len(groups) == 0 {
			groups = append(groups, routeGroup{sample: loopback[0], hosts: loopback})
		} else {
			groups[0].hosts = append(groups[0].hosts, loopback...)
		}
	}
	return groups
}

func scanRouteGroups(groups []routeGroup, ports string, config *SynxConfig) (chan *synscan.SynScanResult, error) {
	merged := make(chan *synscan.SynScanResult, 1024)
	go func() {
		defer close(merged)
		succeeded := 0
		for i, group := range groups {
			if config.Ctx.Err() != nil {
				return
			}
			cfg := *config
			cfg.appendOutput = i > 0 && config.outputFile != ""
			ch, err := scanOne(group.sample, strings.Join(group.hosts, ","), ports, &cfg)
			if err != nil {
				log.Errorf("synscanx: route group %s failed: %v", group.sample, err)
				continue
			}
			succeeded++
			for result := range ch {
				select {
				case <-config.Ctx.Done():
					for range ch {
					}
					return
				case merged <- result:
				}
			}
		}
		if succeeded == 0 {
			log.Errorf("synscanx: every route group failed")
		}
	}()
	return merged, nil
}

// chooseRouteSample picks a non-loopback address when the target list has one,
// so the route lookup does not bind the scan to the loopback interface.
func chooseRouteSample(targets string) string {
	targetList := utils.ParseStringToHosts(targets)
	if len(targetList) == 0 {
		return ""
	}
	for _, target := range targetList {
		if !utils.IsLoopback(target) {
			return target
		}
	}
	return targetList[0]
}
