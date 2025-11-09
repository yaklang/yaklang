package netstackvm

import (
	"encoding/json"
	"net"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// Record 记录路由管理的记录
type Record struct {
	IPAddr  string `json:"ip_addr"`  // IP地址
	TunName string `json:"tun_name"` // 隧道名称
}

// SystemRouteManager 系统路由管理器，单例模式
// 只管理通过AddIPRoute添加的路由记录，不涉及系统默认路由的获取
type SystemRouteManager struct {
	records map[string]*Record // key: ip_addr, value: record
	mux     sync.RWMutex
}

// 路由管理器数据库存储的键名
const routeManagerKeyPrefix = "system-route-manager"
const routeManagerKeyMD5 = "e10adc3949ba59abbe56e057f20f883e" // MD5("yak-route-manager")
const routeManagerDBKey = routeManagerKeyPrefix + "-" + routeManagerKeyMD5

var (
	systemRouteManagerInstance *SystemRouteManager
	systemRouteManagerOnce     sync.Once
)

// GetSystemRouteManager 获取系统路由管理器的单例实例
func GetSystemRouteManager() *SystemRouteManager {
	systemRouteManagerOnce.Do(func() {
		systemRouteManagerInstance = &SystemRouteManager{
			records: make(map[string]*Record),
		}
		// 启动时从数据库加载记录
		systemRouteManagerInstance.loadFromDB()
	})
	return systemRouteManagerInstance
}

// AddIPRoute 添加IP路由
// ipAddrs 可以是 []string 或单个 string，在 Yaklang 中使用更方便
func (m *SystemRouteManager) AddIPRoute(ipAddrs interface{}, tunName string) error {
	var ipList []string

	// 处理参数，支持 []string 或单个 string
	switch v := ipAddrs.(type) {
	case []string:
		ipList = v
	case string:
		ipList = []string{v}
	case []any:
		for _, element := range v {
			ipList = append(ipList, utils.InterfaceToString(element))
		}
	default:
		return utils.Errorf("invalid ipAddrs type: %T, expected []string or string or []any", ipAddrs)
	}

	if len(ipList) == 0 {
		return utils.Error("no IP addresses provided")
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	// 过滤出需要添加的IP（不存在的）
	var toAdd []string
	for _, ipAddr := range ipList {
		// 验证IP地址格式
		if net.ParseIP(ipAddr) == nil {
			log.Errorf("invalid IP address: %s", ipAddr)
			continue
		}

		// 检查是否已经存在
		if _, exists := m.records[ipAddr]; exists {
			log.Warnf("route for IP %s already exists", ipAddr)
			continue
		}

		toAdd = append(toAdd, ipAddr)
	}

	if len(toAdd) == 0 {
		log.Info("no new routes to add")
		return nil
	}

	// 批量添加路由
	success, failed := netutil.BatchAddSpecificIPRouteToNetInterface(toAdd, tunName)
	if len(failed) > 0 {
		log.Errorf("failed to add some routes: %v", failed)
	}

	// 添加成功的记录到内存
	for _, ipAddr := range success {
		record := &Record{
			IPAddr:  ipAddr,
			TunName: tunName,
		}
		m.records[ipAddr] = record
		log.Infof("successfully added route for IP %s to interface %s", ipAddr, tunName)
	}

	// 保存到数据库
	m.saveToDB()

	if len(success) == 0 {
		return utils.Errorf("failed to add any routes")
	}

	return nil
}

// DeleteIPRoute 删除IP路由
// ipAddrs 可以是 []string 或单个 string，在 Yaklang 中使用更方便
func (m *SystemRouteManager) DeleteIPRoute(ipAddrs interface{}) error {
	var ipList []string

	// 处理参数，支持 []string 或单个 string
	switch v := ipAddrs.(type) {
	case []string:
		ipList = v
	case string:
		ipList = []string{v}
	case []any:
		for _, element := range v {
			ipList = append(ipList, utils.InterfaceToString(element))
		}
	default:
		return utils.Errorf("invalid ipAddrs type: %T, expected []string or string or []any", ipAddrs)
	}

	if len(ipList) == 0 {
		return utils.Error("no IP addresses provided")
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	// 过滤出存在的IP
	var toDelete []string
	for _, ipAddr := range ipList {
		if _, exists := m.records[ipAddr]; !exists {
			log.Warnf("route for IP %s does not exist", ipAddr)
			continue
		}
		toDelete = append(toDelete, ipAddr)
	}

	if len(toDelete) == 0 {
		log.Info("no routes to delete")
		return nil
	}

	// 批量删除路由
	success, failed := netutil.BatchDeleteSpecificIPRoute(toDelete)
	if len(failed) > 0 {
		log.Errorf("failed to delete some routes: %v", failed)
	}

	// 从内存记录中移除成功的
	for _, ipAddr := range success {
		record := m.records[ipAddr]
		delete(m.records, ipAddr)
		log.Infof("successfully deleted route for IP %s from interface %s", ipAddr, record.TunName)
	}

	// 保存到数据库
	m.saveToDB()

	if len(success) == 0 {
		return utils.Errorf("failed to delete any routes")
	}

	return nil
}

// DeleteRoutesForInterface 删除指定接口的所有路由
func (m *SystemRouteManager) DeleteRoutesForInterface(interfaceName string) error {
	if interfaceName == "" {
		return utils.Error("interface name cannot be empty")
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	// 找到该接口的所有路由
	var toDelete []string
	for ipAddr, record := range m.records {
		if record.TunName == interfaceName {
			toDelete = append(toDelete, ipAddr)
		}
	}

	if len(toDelete) == 0 {
		log.Infof("no routes found for interface %s", interfaceName)
		return nil
	}

	// 批量删除路由
	success, failed := netutil.BatchDeleteSpecificIPRoute(toDelete)
	if len(failed) > 0 {
		log.Errorf("failed to delete some routes for interface %s: %v", interfaceName, failed)
	}

	// 从内存记录中移除成功的
	for _, ipAddr := range success {
		delete(m.records, ipAddr)
		log.Infof("successfully deleted route for IP %s from interface %s", ipAddr, interfaceName)
	}

	// 保存到数据库
	m.saveToDB()

	if len(success) == 0 {
		return utils.Errorf("failed to delete any routes for interface %s", interfaceName)
	}

	return nil
}

// GetExistedManagedSystemTableRoute 获取已存在的管理路由记录
func (m *SystemRouteManager) GetExistedManagedSystemTableRoute() []*Record {
	m.mux.RLock()
	defer m.mux.RUnlock()

	records := make([]*Record, 0, len(m.records))
	for _, record := range m.records {
		records = append(records, &Record{
			IPAddr:  record.IPAddr,
			TunName: record.TunName,
		})
	}

	return records
}

// saveToDB 保存记录到数据库
func (m *SystemRouteManager) saveToDB() {
	records := make([]*Record, 0, len(m.records))
	for _, record := range m.records {
		records = append(records, record)
	}

	data, err := json.Marshal(records)
	if err != nil {
		log.Errorf("failed to marshal route records: %v", err)
		return
	}

	yakit.Set(routeManagerDBKey, string(data))
	log.Debugf("saved %d route records to database", len(records))
}

// loadFromDB 从数据库加载记录
func (m *SystemRouteManager) loadFromDB() {
	data := yakit.Get(routeManagerDBKey)
	if data == "" {
		log.Debugf("no route records found in database")
		return
	}

	var records []*Record
	err := json.Unmarshal([]byte(data), &records)
	if err != nil {
		log.Errorf("failed to unmarshal route records: %v", err)
		return
	}

	for _, record := range records {
		m.records[record.IPAddr] = record
	}

	log.Infof("loaded %d route records from database", len(records))
}
