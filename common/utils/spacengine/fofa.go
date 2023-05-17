package spacengine

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/fofa"
	"github.com/yaklang/yaklang/common/utils/suspect"
	"strings"
)

func FofaQuery(email string, fofaKey string, filter string, maxPage, maxRecord int) (chan *NetSpaceEngineResult, error) {
	// build fofa client
	client := fofa.NewFofaClient([]byte(email), []byte(fofaKey))
	userInfo, err := client.UserInfo()
	if err != nil {
		return nil, err
	}
	_ = userInfo

	ch := make(chan *NetSpaceEngineResult)
	go func() {
		defer close(ch)

		var nextFinished bool
		var count int
		var page = 0

		_ = count
		for range make([]int, maxPage) {
			page++
			if nextFinished {
				break
			}
			match, err := client.QueryAsJSON(
				uint(page),
				[]byte(filter),
			)
			if err != nil {
				log.Error(err)
				return
			}

			var res = make(map[string]interface{})
			err = json.Unmarshal(match, &res)
			if err != nil {
				log.Error(err)
				return
			}
			//spew.Dump(res)
			// map[string]interface {}) (len=6) {
			// (string) (len=5) "error": (bool) false,
			// (string) (len=5) "query": (string) (len=14) "title=\"国网\"",
			// (string) (len=4) "page": (float64) 1,
			// (string) (len=4) "size": (float64) 14829,
			// (string) (len=7) "results": ([]interface {}) (len=100 cap=128) {
			//  ([]interface {}) (len=7 cap=8) {
			//   (string) "",
			//   (string) (len=18) "218.5.160.209:8088",
			//   (string) (len=13) "218.5.160.209",
			//   (string) (len=4) "8088",
			//   (string) (len=47) "帝国网站管理系统 - Powered by EmpireCMS",
			//   (string) (len=2) "CN",
			//   (string) ""
			//  },
			if r, ok := res["results"]; ok {
				results, tok := r.([]interface{})
				if !tok {
					continue
				}

				for _, data := range results {
					strArray, tok := data.([]interface{})
					if !tok {
						continue
					}
					if len(strArray) < 7 {
						continue
					}
					ip := fmt.Sprintf("%v", strArray[2])
					port := fmt.Sprintf("%v", strArray[3])
					log.Infof("fofa fetch: %s",
						utils.HostPort(ip, port),
					)

					var finalHost = ip
					var domains = []string{fmt.Sprint(strArray[0])}
					urlIns := fmt.Sprint(strArray[1])
					if urlIns != "" {
						host, _, _ := utils.ParseStringToHostPort(urlIns)
						if host != "" {
							finalHost = host
							domains = append(domains, host)
						} else {
							domains = append(domains, urlIns)
						}
					}

					var confirmHttps = false
					if strings.HasPrefix(strings.ToLower(urlIns), "https://") {
						confirmHttps = true
					}

					if !suspect.IsFullURL(urlIns) {
						urlIns = ""
					}

					ch <- &NetSpaceEngineResult{
						Addr:            utils.HostPort(ip, port),
						FromEngine:      "fofa",
						Latitude:        0,
						Longitude:       0,
						Url:             urlIns,
						ConfirmHttps:    confirmHttps,
						HtmlTitle:       fmt.Sprintf("%v", strArray[4]),
						Domains:         strings.Join(domains, "|"),
						Province:        fmt.Sprintf("%v", strArray[5]),
						City:            fmt.Sprintf("%v", strArray[6]),
						Asn:             "",
						Location:        fmt.Sprintf("%v", strArray[6]),
						ServiceProvider: "",
						FromFilter:      filter,
						Host:            finalHost,
					}

					count++
					if maxRecord > 0 && count >= maxRecord {
						nextFinished = true
						break
					}
				}
			}
			//for _, d := range match {
			//	ip := d.IP
			//	port := d.Port
			//	log.Infof("shodan fetch: %s",
			//		utils.HostPort(ip, port),
			//	)
			//	err := CreateMonitorWebsite(
			//		db, utils.HostPort(ip, port), "shodan",
			//		0, 0, d.Title, d.Domain, d.Country, d.City, "",
			//		d.City, "",
			//		filter,
			//	)
			//	if err != nil {
			//		log.Error(err)
			//	}
			//
			//	count++
			//	if maxRecord > 0 && count >= maxRecord {
			//		nextFinished = true
			//	}
			//}
		}

	}()
	return ch, nil
}
