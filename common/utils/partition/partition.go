package partition

import (
	"fmt"
	"yaklang/common/log"
	"yaklang/common/utils"
	"regexp"
	"strings"
)

const RegexRaw = `(?i)\{\{%s:(.*?)\}\}`

type Tag struct {
	Name string
	R    *regexp.Regexp
}

var (
	cTagR = func(tag string) *regexp.Regexp {
		rRaw := fmt.Sprintf(RegexRaw, tag)
		r, err := regexp.Compile(rRaw)
		if err != nil {
			panic("cannot compile build-in tag for partition")
		}

		return r
	}

	// IPv4 Tag
	TAG_IPv4   = "IPv4"
	TAG_IPv4_R = cTagR(TAG_IPv4)

	// Port Tag
	TAG_Port   = "PORT"
	TAG_Port_R = cTagR(TAG_Port)

	// Dict Tag
	TAG_Dict   = "DICT"
	TAG_Dict_R = cTagR(TAG_Dict)

	IPv4Tag = &Tag{
		Name: TAG_IPv4,
		R:    TAG_IPv4_R,
	}

	PortTag = &Tag{
		Name: TAG_Port,
		R:    TAG_Port_R,
	}

	DictTag = &Tag{
		Name: TAG_Dict,
		R:    TAG_Dict_R,
	}
)

var (
	availableTags = map[*Tag]func(string, *regexp.Regexp) chan string{
		// ipv4 mutate
		IPv4Tag: func(s string, r *regexp.Regexp) chan string {
			outC := make(chan string)

			go func() {
				defer close(outC)

				results := r.FindAllStringSubmatch(s, -1)
				if len(results) <= 0 {
					outC <- s
					return
				}

				for _, group := range results {
					origin := group[0]
					generator := group[1]
					log.Infof("parsing TAG: %s apply: %s", origin, generator)

					for _, line := range utils.ParseStringToHosts(generator) {
						outC <- strings.Replace(s, origin, line, -1)
					}
				}
			}()

			return outC
		},

		// port mutate
		PortTag: func(s string, r *regexp.Regexp) chan string {
			outC := make(chan string)

			go func() {
				defer close(outC)

				results := r.FindAllStringSubmatch(s, -1)
				if len(results) <= 0 {
					outC <- s
					return
				}

				for _, group := range results {
					origin := group[0]
					generator := group[1]

					for _, port := range utils.ParseStringToPorts(generator) {
						outC <- strings.Replace(s, origin, fmt.Sprint(port), -1)
					}
				}
			}()

			return outC
		},
	}
)

func NeedSeperating(raw string) bool {
	return strings.Contains(raw, "{{PORT") || strings.Contains(raw, "{{IPv4") || strings.Contains(raw, "{{DICT")
}

func NeedDictSeperating(raw string) bool {
	return strings.Contains(raw, "{{DICT")
}

func NeedIPorPortSeperating(raw string) bool {
	return strings.Contains(raw, "{{PORT") || strings.Contains(raw, "{{IPv4")
}

func SeperateIPv4nPort(raw string) chan string {
	outC := make(chan string)
	go func() {
		defer close(outC)

		// handle target
		ipv4Handler := availableTags[IPv4Tag]
		portHandler := availableTags[PortTag]
		for result := range ipv4Handler(raw, IPv4Tag.R) {
			for finalRaw := range portHandler(result, PortTag.R) {
				outC <- finalRaw
			}
		}
	}()

	return outC
}
