package bruteutils

import (
	"fmt"
	"net"
	"strings"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/lowhttp"

	"gopkg.in/mgo.v2"
)

// https://github.com/k8gege/LadonGo/
var mongoAuth = &DefaultServiceAuthInfo{
	ServiceName:      "mongodb",
	DefaultPorts:     "27017",
	DefaultUsernames: append([]string{"root", "admin", "mongodb"}, CommonUsernames...),
	DefaultPasswords: CommonPasswords,
	UnAuthVerify: func(i *BruteItem) *BruteItemResult {
		result := i.Result()
		i.Target = appendDefaultPort(i.Target, 27017)

		conn, err := DefaultDailer.Dial("tcp", i.Target)
		if err != nil {
			res := i.Result()
			res.Finished = true
			return res
		}
		conn.Close()

		host, port, _ := utils.ParseStringToHostPort(i.Target)
		bytes := lowhttp.FetchBannerFromHostPort(utils.TimeoutContextSeconds(5), host, port, 4096, true, false, false)

		// 指纹识别验证
		if !utils.IContains(string(bytes), "It looks like you are trying to access MongoDB over HTTP on the native driver port.") {
			result.Finished = true
			return result
		}

		if r, err := MongodbUnauth(host, port); err != nil {
			log.Errorf("check mongodb unauth failed: %s", err)
		} else {
			if r {
				result.Ok = true
				result.Username = ""
				result.Password = ""
				return result
			}
		}
		return result
	},
	BrutePass: func(i *BruteItem) *BruteItemResult {
		r := i.Result()
		username := i.Username
		password := i.Password
		host, port, _ := utils.ParseStringToHostPort(appendDefaultPort(i.Target, 27017))
		session, err := mgo.DialWithTimeout("mongodb://"+username+":"+password+"@"+host+":"+fmt.Sprint(port)+"/"+"admin", time.Second*3)
		if err != nil {
			autoSetFinishedByConnectionError(err, r)
			return r
		}
		defer session.Close()

		err = session.Ping()
		if err != nil {
			autoSetFinishedByConnectionError(err, r)
			return r
		}
		if session.Run("serverStatus", nil) == nil {
			r.Ok = true
			return r
		}
		return r
	},
}

func MongodbUnauth(host string, port int) (flag bool, err error) {
	timeoutSeconds := 10

	flag = false
	senddata := []byte{58, 0, 0, 0, 167, 65, 0, 0, 0, 0, 0, 0, 212, 7, 0, 0, 0, 0, 0, 0, 97, 100, 109, 105, 110, 46, 36, 99, 109, 100, 0, 0, 0, 0, 0, 255, 255, 255, 255, 19, 0, 0, 0, 16, 105, 115, 109, 97, 115, 116, 101, 114, 0, 1, 0, 0, 0, 0}
	getlogdata := []byte{72, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 212, 7, 0, 0, 0, 0, 0, 0, 97, 100, 109, 105, 110, 46, 36, 99, 109, 100, 0, 0, 0, 0, 0, 1, 0, 0, 0, 33, 0, 0, 0, 2, 103, 101, 116, 76, 111, 103, 0, 16, 0, 0, 0, 115, 116, 97, 114, 116, 117, 112, 87, 97, 114, 110, 105, 110, 103, 115, 0, 0}
	realhost := fmt.Sprintf("%s:%v", host, port)
	conn, err := net.DialTimeout("tcp", realhost, time.Duration(timeoutSeconds)*time.Second)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()
	if err != nil {
		return flag, err
	}
	err = conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutSeconds) * time.Second))
	if err != nil {
		return flag, err
	}
	_, err = conn.Write(senddata)
	if err != nil {
		return flag, err
	}
	buf := make([]byte, 1024)
	count, err := conn.Read(buf)
	if err != nil {
		return flag, err
	}
	text := string(buf[0:count])
	if strings.Contains(text, "ismaster") {
		_, err = conn.Write(getlogdata)
		if err != nil {
			return flag, err
		}
		count, err := conn.Read(buf)
		if err != nil {
			return flag, err
		}
		text := string(buf[0:count])
		if strings.Contains(text, "totalLinesWritten") {
			return true, nil
		}
	}
	return flag, err
}
