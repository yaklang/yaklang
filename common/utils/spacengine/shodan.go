package spacengine

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/go-shodan"
)

var defaultHttpClient = utils.NewDefaultHTTPClient()

type ShodanUser struct {
	Member      bool        `json:"member"`
	Credits     int         `json:"credits"`
	DisplayName interface{} `json:"display_name"`
	Created     string      `json:"created"`
}

func ShodanUserProfile(key string) (*ShodanUser, error) {
	profileApi := "https://api.shodan.io/account/profile?key="
	profileApi = fmt.Sprintf("%s%s", profileApi, key)
	rsp, err := defaultHttpClient.Get(profileApi)
	if err != nil {
		return nil, err
	}
	if rsp.StatusCode != 200 {
		return nil, utils.Errorf("[%v]: invalid status code", rsp.StatusCode)
	}
	var user ShodanUser
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		return nil, err
	}
	defer rsp.Body.Close()
	err = json.Unmarshal(body, &user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
func interfaceArrayToString(rets []interface{}) string {
	var r []string
	for _, i := range rets {
		r = append(r, fmt.Sprintf("%v", i))
	}
	return strings.Join(r, ",")
}

func ServiceProviderToChineseName(i string) string {
	switch true {
	case strings.Contains(i, "Tencent cloud computing"):
		return "腾讯云"
	case strings.Contains(i, "Amazon.com"):
		return "亚马逊"
	case strings.Contains(i, "Amazon"):
		return "亚马逊"
	case strings.Contains(i, "Alibaba"):
		return "阿里巴巴"
	case strings.Contains(i, "Hangzhou Alibaba Advertising Co.,Ltd."):
		return "阿里云"
	case strings.Contains(i, "China Telecom"):
		return "中国电信"
	case strings.Contains(i, "Google Cloud"):
		return "谷歌云"
	case strings.Contains(i, "Microsoft Corporation"):
		return "微软"
	default:
		return i
	}
}

func ShodanQuery(key string, filter string, maxPage, maxRecord int) (chan *NetSpaceEngineResult, error) {
	client := shodan.New(key)
	info, err := client.APIInfo()
	if err != nil {
		return nil, utils.Errorf("get shodan info failed: %s", err)
	}
	_ = info

	ch := make(chan *NetSpaceEngineResult)
	go func() {
		defer close(ch)

		var nextFinished bool
		var count int
		page := 0
		for range make([]int, maxPage) {
			page++
			if nextFinished {
				break
			}

			if utils.IsIPv4(filter) || utils.IsIPv6(filter) {
				hostResult, err := client.Host(filter, map[string][]string{})
				if err != nil {
					log.Errorf("query host for op failed: %s", err)
					return
				}
				if hostResult == nil {
					log.Errorf("emtpy result for %s", filter)
					return
				}
				for _, port := range hostResult.Ports {
					if nextFinished {
						break
					}
					tmpR := &NetSpaceEngineResult{
						Addr:            utils.HostPort(hostResult.IPStr, port),
						FromEngine:      "shodan",
						Latitude:        hostResult.Latitude,
						Longitude:       hostResult.Longitude,
						Domains:         strings.Join(hostResult.Hostnames, ","),
						Province:        hostResult.CountryName,
						City:            hostResult.City,
						Asn:             hostResult.Asn,
						Location:        hostResult.CountryName,
						ServiceProvider: hostResult.Isp,
						FromFilter:      filter,
					}
					ch <- tmpR
				}
				return
			}

			match, err := client.HostSearch(filter, nil, map[string][]string{
				"page": {fmt.Sprintf(`%v`, page)},
				//"limit": {"20"},
			})
			if err != nil {
				log.Errorf("shodan.HostSearch[%v] failed: %s", filter, err)
				return
			}

			for _, d := range match.Matches {
				ip, port := utils.Uint32ToIPv4(uint32(d.IP)), d.Port
				log.Infof("shodan fetch: %s",
					utils.HostPort(ip.String(), port),
				)

				serviceProvider := ""
				if d.Isp == d.Org {
					serviceProvider = d.Isp
				} else {
					serviceProvider = fmt.Sprintf("%v[%v]", d.Isp, d.Org)
				}
				serviceProvider = ServiceProviderToChineseName(serviceProvider)

				var fps []string
				if d.Os != nil {
					fps = append(fps, fmt.Sprint(d.Os))
				}
				fps = utils.RemoveRepeatStringSlice(fps)
				provider := &NetSpaceEngineResult{
					Addr:            utils.HostPort(ip.String(), port),
					FromEngine:      "shodan",
					Latitude:        d.Location.Latitude,
					Longitude:       d.Location.Longitude,
					HtmlTitle:       d.HTTP.Title,
					Domains:         interfaceArrayToString(d.Domains),
					ConfirmHttps:    false,
					Host:            interfaceArrayToString(d.Hostnames),
					City:            fmt.Sprint(d.Location.City),
					Asn:             d.Asn,
					Location:        strings.Join([]string{d.Location.CountryName, fmt.Sprint(d.Location.City)}, "/"),
					ServiceProvider: serviceProvider,
					FromFilter:      filter,
					Fingerprints:    strings.Join(fps, "/"),
					Banner:          utils.ParseStringToVisible(d.Data),
				}
				ch <- provider

				count++
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
				}
			}
		}
	}()
	return ch, nil
}
