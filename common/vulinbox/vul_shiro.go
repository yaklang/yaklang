package vulinbox

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

var keyList = []string{
	"kPH+bIxk5D2deZiIxcaaaA==",
	"4AvVhmFLUs0KTA3Kprsdag==",
	"Z3VucwAAAAAAAAAAAAAAAA==",
	"fCq+/xW488hMTCD+cmJ3aQ==",
	"0AvVhmFLUs0KTA3Kprsdag==",
	"1AvVhdsgUs0FSA3SDFAdag==",
	"1QWLxg+NYmxraMoxAXu/Iw==",
	"25BsmdYwjnfcWmnhAciDDg==",
	"2AvVhdsgUs0FSA3SDFAdag==",
	"3AvVhmFLUs0KTA3Kprsdag==",
	"3JvYhmBLUs0ETA5Kprsdag==",
	"r0e3c16IdVkouZgk1TKVMg==",
	"5aaC5qKm5oqA5pyvAAAAAA==",
	"5AvVhmFLUs0KTA3Kprsdag==",
	"6AvVhmFLUs0KTA3Kprsdag==",
	"6NfXkC7YVCV5DASIrEm1Rg==",
	"6ZmI6I2j5Y+R5aSn5ZOlAA==",
	"cmVtZW1iZXJNZQAAAAAAAA==",
	"7AvVhmFLUs0KTA3Kprsdag==",
	"8AvVhmFLUs0KTA3Kprsdag==",
	"8BvVhmFLUs0KTA3Kprsdag==",
	"9AvVhmFLUs0KTA3Kprsdag==",
	"OUHYQzxQ/W9e/UjiAGu6rg==",
	"a3dvbmcAAAAAAAAAAAAAAA==",
	"aU1pcmFjbGVpTWlyYWNsZQ==",
	"bWljcm9zAAAAAAAAAAAAAA==",
	"bWluZS1hc3NldC1rZXk6QQ==",
	"bXRvbnMAAAAAAAAAAAAAAA==",
	"ZUdsaGJuSmxibVI2ZHc9PQ==",
	"wGiHplamyXlVB11UXWol8g==",
	"U3ByaW5nQmxhZGUAAAAAAA==",
	"MTIzNDU2Nzg5MGFiY2RlZg==",
	"L7RioUULEFhRyxM7a2R/Yg==",
	"a2VlcE9uR29pbmdBbmRGaQ==",
	"WcfHGU25gNnTxTlmJMeSpw==",
	"OY//C4rhfwNxCQAQCrQQ1Q==",
	"5J7bIJIV0LQSN3c9LPitBQ==",
	"f/SY5TIve5WWzT4aQlABJA==",
	"bya2HkYo57u6fWh5theAWw==",
	"WuB+y2gcHRnY2Lg9+Aqmqg==",
	"kPv59vyqzj00x11LXJZTjJ2UHW48jzHN",
	"3qDVdLawoIr1xFd6ietnwg==",
	"ZWvohmPdUsAWT3=KpPqda",
	"YI1+nBV//m7ELrIyDHm6DQ==",
	"6Zm+6I2j5Y+R5aS+5ZOlAA==",
	"2A2V+RFLUs+eTA3Kpr+dag==",
	"6ZmI6I2j3Y+R1aSn5BOlAA==",
	"SkZpbmFsQmxhZGUAAAAAAA==",
	"2cVtiE83c4lIrELJwKGJUw==",
	"fsHspZw/92PrS3XrPW+vxw==",
	"XTx6CKLo/SdSgub+OPHSrw==",
	"sHdIjUN6tzhl8xZMG3ULCQ==",
	"O4pdf+7e+mZe8NyxMTPJmQ==",
	"HWrBltGvEZc14h9VpMvZWw==",
	"rPNqM6uKFCyaL10AK51UkQ==",
	"Y1JxNSPXVwMkyvES/kJGeQ==",
	"lT2UvDUmQwewm6mMoiw4Ig==",
	"MPdCMZ9urzEA50JDlDYYDg==",
	"xVmmoltfpb8tTceuT5R7Bw==",
	"c+3hFGPjbgzGdrC+MHgoRQ==",
	"ClLk69oNcA3m+s0jIMIkpg==",
	"Bf7MfkNR0axGGptozrebag==",
	"1tC/xrDYs8ey+sa3emtiYw==",
	"ZmFsYWRvLnh5ei5zaGlybw==",
	"cGhyYWNrY3RmREUhfiMkZA==",
	"IduElDUpDDXE677ZkhhKnQ==",
	"yeAAo1E8BOeAYfBlm4NG9Q==",
	"cGljYXMAAAAAAAAAAAAAAA==",
	"2itfW92XazYRi5ltW0M2yA==",
	"XgGkgqGqYrix9lI6vxcrRw==",
	"ertVhmFLUs0KTA3Kprsdag==",
	"5AvVhmFLUS0ATA4Kprsdag==",
	"s0KTA3mFLUprK4AvVhsdag==",
	"hBlzKg78ajaZuTE0VLzDDg==",
	"9FvVhtFLUs0KnA3Kprsdyg==",
	"d2ViUmVtZW1iZXJNZUtleQ==",
	"yNeUgSzL/CfiWw1GALg6Ag==",
	"NGk/3cQ6F5/UNPRh8LpMIg==",
	"4BvVhmFLUs0KTA3Kprsdag==",
	"MzVeSkYyWTI2OFVLZjRzZg==",
	"empodDEyMwAAAAAAAAAAAA==",
	"A7UzJgh1+EWj5oBFi+mSgw==",
	"YTM0NZomIzI2OTsmIzM0NTueYQ==",
	"c2hpcm9fYmF0aXMzMgAAAA==",
	"i45FVt72K2kLgvFrJtoZRw==",
	"U3BAbW5nQmxhZGUAAAAAAA==",
	"ZnJlc2h6Y24xMjM0NTY3OA==",
	"Jt3C93kMR9D5e8QzwfsiMw==",
	"MTIzNDU2NzgxMjM0NTY3OA==",
	"vXP33AonIp9bFwGl7aT7rA==",
	"V2hhdCBUaGUgSGVsbAAAAA==",
	"Z3h6eWd4enklMjElMjElMjE=",
	"Q01TX0JGTFlLRVlfMjAxOQ==",
	"ZAvph3dsQs0FSL3SDFAdag==",
	"Is9zJ3pzNh2cgTHB4ua3+Q==",
	"NsZXjXVklWPZwOfkvk6kUA==",
	"GAevYnznvgNCURavBhCr1w==",
	"66v1O8keKNV3TTcGPK1wzg==",
	"SDKOLKn2J1j/2BHjeZwAoQ==",
}

var gadgetList = []string{
	"CB183NoCC",
	"CB192NoCC",
	"CCK1",
	"CCK2",
}

var randKey []byte

var randGadget string

func init() {
	rand.NewSource(time.Now().UnixNano())
	key := keyList[rand.Intn(len(keyList))]
	//key := keyList[0]
	randGadget = gadgetList[rand.Intn(len(gadgetList))]
	//randGadget = gadgetList[3]
	log.Infof("Use RandKey: %s , Use GadGet :%s ", key, randGadget)
	randKey, _ = codec.DecodeBase64(key)
}

func (s *VulinServer) registerMockVulShiro() {
	var router = s.router
	shiroGroup := router.Name("ShiroVuls Simulation").Subrouter()

	shiroRoutes := []*VulInfo{
		{
			DefaultQuery: "",
			Path:         "/shiro/cbc",
			Title:        "Shiro CBC 默认KEY(<1.4.2)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				failNow := func(writer http.ResponseWriter, request *http.Request, err error) {
					cookie := http.Cookie{
						Name:     "rememberMe",
						Value:    "deleteMe",                         // 设置 cookie 的值
						Expires:  time.Now().Add(7 * 24 * time.Hour), // 设置过期时间
						HttpOnly: false,                              // 仅限 HTTP 访问，不允许 JavaScript 访问
					}
					http.SetCookie(writer, &cookie)
					writer.WriteHeader(200)
					if err != nil {
						writer.Write([]byte(err.Error()))
					}
					return
				}
				successNow := func(writer http.ResponseWriter, request *http.Request) {
					writer.WriteHeader(200)
					return
				}
				rememberMe, err := request.Cookie("rememberMe")
				if err != nil { // 请求没有cookie 那就设置一个
					failNow(writer, request, err)
					return
				}
				cookieVal, _ := codec.DecodeBase64(rememberMe.Value)
				var iv []byte
				if len(cookieVal) > len(randKey) {
					iv = cookieVal[:16]
					cookieVal = cookieVal[16:]
				} else { // 第一次探测请求
					failNow(writer, request, err)
					return
				}

				payload, err := codec.AESCBCDecrypt(randKey, cookieVal, iv)
				if err != nil || payload == nil { // key不对返回deleteMe
					failNow(writer, request, err)
					return
				}
				payload = codec.PKCS7UnPadding(payload)

				checkGadget(payload, failNow, successNow, writer, request)
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "",
			Path:         "/shiro/gcm",
			Title:        "Shiro GCM 默认KEY(>=1.4.2)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				failNow := func(writer http.ResponseWriter, request *http.Request, err error) {
					cookie := http.Cookie{
						Name:     "rememberMe",
						Value:    "deleteMe",                         // 设置 cookie 的值
						Expires:  time.Now().Add(7 * 24 * time.Hour), // 设置过期时间
						HttpOnly: false,                              // 仅限 HTTP 访问，不允许 JavaScript 访问
					}
					http.SetCookie(writer, &cookie)
					writer.WriteHeader(200)
					if err != nil {
						writer.Write([]byte(err.Error()))
					}
					return
				}
				successNow := func(writer http.ResponseWriter, request *http.Request) {
					writer.WriteHeader(200)
					return
				}
				rememberMe, err := request.Cookie("rememberMe")
				if err != nil { // 请求没有cookie 那就设置一个
					failNow(writer, request, err)
					return
				}
				cookieVal, _ := codec.DecodeBase64(rememberMe.Value)

				payload, err := codec.AESGCMDecrypt(randKey, cookieVal, nil)
				if err != nil || payload == nil { // key不对返回deleteMe
					failNow(writer, request, err)
					return
				}

				checkGadget(payload, failNow, successNow, writer, request)

				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
	}

	for _, v := range shiroRoutes {
		addRouteWithVulInfo(shiroGroup, v)
	}

}

func checkGadget(payload []byte, failNow func(writer http.ResponseWriter, request *http.Request, err error), successNow func(writer http.ResponseWriter, request *http.Request), writer http.ResponseWriter, request *http.Request) {
	javaSerializables, err := yserx.ParseJavaSerialized(payload)
	if err != nil {
		failNow(writer, request, err)
		return
	}
	raw, err := yserx.ToJson(javaSerializables)

	var javaObjectMap []map[string]interface{}

	err = json.Unmarshal(raw, &javaObjectMap)
	if err != nil {
		failNow(writer, request, err)
	}

	if strings.Contains(string(raw), "org.apache.shiro.subject.SimplePrincipalCollection") {
		successNow(writer, request)
		return
	}
	var serialVersionUID int64
	var expectedClassName string
	switch randGadget {
	case "CB183NoCC":
		// Commons-beanutils:1.8.0 serialVersionUID -3490850999041592962
		serialVersionUID = -3490850999041592962
		expectedClassName = "org.apache.commons.beanutils.BeanComparator"

	case "CB192NoCC":
		serialVersionUID = -2044202215314119608
		expectedClassName = "org.apache.commons.beanutils.BeanComparator"

	case "CCK1":
		// Commons-collections:3.1 serialVersionUID -8653385846894047688
		serialVersionUID = -8453869361373831205
		expectedClassName = "org.apache.commons.collections.keyvalue.TiedMapEntry"

	case "CCK2":
		serialVersionUID = -8453869361373831205

		expectedClassName = "org.apache.commons.collections4.keyvalue.TiedMapEntry"
	default:
		// Handle unexpected gadget type.
		failNow(writer, request, utils.Error("Unexpected gadget type"))
		return
	}

	for _, item := range javaObjectMap {
		getUid := findSerialVersionUid(item, expectedClassName)
		if len(getUid) == 0 {
			failNow(writer, request, utils.Errorf("not found %s", expectedClassName))
			return
		}
		uid, err := codec.DecodeBase64(getUid)
		if err != nil || len(uid) == 0 {
			failNow(writer, request, err)
			return
		}
		getserialVersionUID := int64(binary.BigEndian.Uint64(uid))
		if getserialVersionUID != serialVersionUID {
			err = utils.Errorf("serialVersionUID %d not match %d", getserialVersionUID, serialVersionUID)
			failNow(writer, request, err)
			return
		}

		javaClasss := findByteCodes(item)

		for _, class := range javaClasss {
			obj, err := javaclassparser.ParseFromBase64(class)
			if err != nil {
				failNow(writer, request, err)
				return
			}
			flag := obj.FindConstStringFromPool("EchoHeader")

			if flag == nil {
				continue
			}
			javaJson, err := obj.Json()
			if err != nil {
				failNow(writer, request, err)
				return
			}
			v := findSetHeaderValue(javaJson)
			if len(v) == 0 {
				failNow(writer, request, err)
				return
			}
			if strings.Contains(v, "|") {
				vs := strings.Split(v, "|")
				writer.Header().Set(vs[0], vs[1])
				writer.Header().Set("Gadget", randGadget)
				failNow(writer, request, nil)
				return
			}

		}
	}
}

func findClassName(obj interface{}) {
	switch concreteVal := obj.(type) {
	case map[string]interface{}:
		for key, value := range concreteVal {
			if key == "class_name" {
				fmt.Println(value)
			}
			findClassName(value)
		}
	case []interface{}:
		for _, item := range concreteVal {
			findClassName(item)
		}
	}
}

func findSerialVersionUid(obj interface{}, className string) string {
	switch concreteVal := obj.(type) {
	case map[string]interface{}:
		if concreteVal["class_name"] == className {
			if serialVersion, ok := concreteVal["serial_version"].(string); ok {
				return serialVersion
			}
		}
		for _, value := range concreteVal {
			if result := findSerialVersionUid(value, className); result != "" {
				return result
			}
		}
	case []interface{}:
		for _, item := range concreteVal {
			if result := findSerialVersionUid(item, className); result != "" {
				return result
			}
		}
	}
	return ""
}

func findByteCodes(obj interface{}) []string {
	var bytesList []string
	switch concreteVal := obj.(type) {
	case map[string]interface{}:
		if bytesCode, ok := concreteVal["bytescode"].(bool); ok && bytesCode {
			if bytes, ok := concreteVal["bytes"].(string); ok {
				bytesList = append(bytesList, bytes)
			}
		}
		for _, value := range concreteVal {
			bytesList = append(bytesList, findByteCodes(value)...)
		}
	case []interface{}:
		for _, item := range concreteVal {
			bytesList = append(bytesList, findByteCodes(item)...)
		}
	}
	return bytesList
}

func findSetHeaderValue(j string) string {
	var result map[string]interface{}
	err := json.Unmarshal([]byte(j), &result)
	if err != nil {
		return ""
	}

	constantPool := result["ConstantPool"].([]interface{})

	var targetString string

	for i, v := range constantPool {
		pool := v.(map[string]interface{})
		if pool["NameAndTypeIndexVerbose"] == "setHeader" && i+1 < len(constantPool) {
			nextPool := constantPool[i+1].(map[string]interface{})
			targetString = nextPool["StringIndexVerbose"].(string)
			return targetString
		}
	}

	return ""
}
