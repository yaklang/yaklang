package utils

import (
	"fmt"
	"strings"
)

func ParseStringToUrlsWith3W(sub ...string) []string {
	urls := ParseStringToUrls(sub...)

	var t []string
	for _, u := range urls {
		t = append(t, u)

		host, port, err := ParseStringToHostPort(u)
		if err != nil {
			continue
		}

		if host == "" {
			continue
		}

		rawPath := ExtractRawPath(u)

		if !strings.HasPrefix(host, "www.") {
			if IsIPv4(host) {
				continue
			}

			if !strings.Contains(host, ".") {
				continue
			}

			newDomain := fmt.Sprintf("www.%v", host)
			if strings.HasPrefix(u, "http://") {
				switch port {
				case 80:
					t = append(t, fmt.Sprintf("http://%v", newDomain)+rawPath)
				default:
					t = append(t, fmt.Sprintf("http://%v:%v", newDomain, port)+rawPath)
				}
			} else if strings.HasPrefix(u, "https://") {
				switch port {
				case 443:
					t = append(t, fmt.Sprintf("https://%v", newDomain)+rawPath)
				default:
					t = append(t, fmt.Sprintf("https://%v:%v", newDomain, port)+rawPath)
				}
			}
		}
	}

	return t
}
