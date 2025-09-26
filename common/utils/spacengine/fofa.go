package spacengine

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/spacengine/base"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/spacengine/fofa"
	"github.com/yaklang/yaklang/common/utils/suspect"
)

func FofaQuery(email string, fofaKey string, filter string, maxPage, pageSize, maxRecord int, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	return FofaQueryWithConfig(email, fofaKey, filter, maxPage, pageSize, maxRecord, nil, domains...)
}

func FofaQueryWithConfig(email string, fofaKey string, filter string, maxPage, pageSize, maxRecord int, config *base.QueryConfig, domains ...string) (chan *base.NetSpaceEngineResult, error) {
	if config == nil {
		config = &base.QueryConfig{}
	}
	var client *fofa.FofaClient
	if len(domains) > 0 && domains[0] != "" {
		client = fofa.NewClientEx(email, fofaKey, domains[0])
	} else {
		client = fofa.NewClient(email, fofaKey)
	}
	_, err := client.UserProfile()
	if err != nil {
		return nil, err
	}

	ch := make(chan *base.NetSpaceEngineResult)
	go func() {
		defer close(ch)

		var nextFinished bool
		var count int

		_ = count
		for page := 1; page <= maxPage; page++ {
			if nextFinished {
				break
			}
			result, err := client.Query(
				page,
				pageSize,
				filter,
			)
			if err != nil {
				log.Error(err)
				return
			}
			rResults := result.Get("results").Array()

			// 如果当前页没有数据，停止翻页
			if len(rResults) == 0 {
				nextFinished = true
				break
			}

			for _, r := range rResults {
				if nextFinished {
					break
				}
				rData := r.Array()
				if len(rData) < 7 {
					continue
				}

				ip := rData[2].String()
				port := rData[4].String()
				log.Infof("fofa fetch: %s",
					utils.HostPort(ip, port),
				)

				finalHost := ip
				domains := []string{}
				urlIns := rData[0].String()
				if urlIns != "" {
					host, _, _ := utils.ParseStringToHostPort(urlIns)
					if host != "" {
						finalHost = host
						domains = append(domains, host)
					} else {
						domains = append(domains, urlIns)
					}
				}

				confirmHttps := false
				if strings.HasPrefix(strings.ToLower(urlIns), "https://") {
					confirmHttps = true
				}

				if !suspect.IsFullURL(urlIns) {
					urlIns = ""
				}

				country := rData[5].String()
				city := rData[6].String()
				location := country
				if city != "" {
					location = fmt.Sprintf("%s %s", country, city)
				}

				ch <- &base.NetSpaceEngineResult{
					Addr:            utils.HostPort(ip, port),
					FromEngine:      "fofa",
					Latitude:        0,
					Longitude:       0,
					Url:             urlIns,
					ConfirmHttps:    confirmHttps,
					HtmlTitle:       rData[1].String(),
					Domains:         strings.Join(domains, "|"),
					City:            city,
					Asn:             "",
					Location:        location,
					ServiceProvider: "",
					FromFilter:      filter,
					Host:            finalHost,
				}

				count++
				// 检查是否达到最大记录数限制
				if maxRecord > 0 && count >= maxRecord {
					nextFinished = true
					break
				}
			}

			// 如果当前页返回的数据少于pageSize，说明没有更多数据了
			if len(rResults) < pageSize {
				nextFinished = true
			}

			// 在翻页之间应用随机延迟
			if !nextFinished && page < maxPage {
				base.ApplyRandomDelay(config.RandomDelayRange)
			}
		}
	}()
	return ch, nil
}
