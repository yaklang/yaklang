package scannode

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"yaklang/common/cybertunnel"
	"yaklang/common/log"
)

type IpEcho struct {
	ExternalIp string `json:"external_ip"`
}

func (n *ScanNode) GetIpecho(serverIp string, serverPort string) {
	n.node.ExternalIp = "error"
	url := fmt.Sprintf("http://%s:%s/api/node/ipecho?node_id=%s", serverIp, serverPort, n.node.NodeId)
	method := "GET"
	client := &http.Client{}
	req, err := http.NewRequest(method, url, nil)

	if err != nil {
		log.Warnf("connect to ip echo service error: %v", err)
		log.Infof("use external ip echo service instead...")
		ip, err := cybertunnel.FetchExternalIP()
		if err != nil {
			log.Warnf("fetch external ip failed")
			return
		}
		n.node.ExternalIp = string(ip)
		return
	}
	req.Header.Add("User-Agent", "")
	req.Header.Add("Authorization", "")

	res, err := client.Do(req)
	if err != nil {
		log.Warnf("connect to ip echo service error: %v", err)
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Warnf("connect to ip echo service error: %v", err)
		return
	}
	//log.Infof("ip echo service return: %v", body)
	var ipEchoJson IpEcho
	if err := json.Unmarshal(body, &ipEchoJson); err == nil {
		//log.Infof("node ip: %v", ipEchoJson.ExternalIp)
		n.node.ExternalIp = ipEchoJson.ExternalIp
		return
	} else {
		log.Warnf("error ip: %v", err)
		return
	}
}
