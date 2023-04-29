package lowhttp

import "testing"

func TestMergeUrlFromHTTPRequest(t *testing.T) {
	println("basic")
	a := MergeUrlFromHTTPRequest([]byte(`GET /target HTTP/1.1
Host: www.baidu.com

`), "index.php", false)
	if a != "http://www.baidu.com/target/index.php" {
		println(a)
		panic(11)
	}

	println("basic abs")
	a = MergeUrlFromHTTPRequest([]byte(`GET /target HTTP/1.1
Host: www.baidu.com

`), "/index.php", false)
	if a != "http://www.baidu.com/index.php" {
		println(a)
		panic(11)
	}

	println("basic full url")
	a = MergeUrlFromHTTPRequest([]byte(`GET /target HTTP/1.1
Host: www.baidu.com

`), "https://www.example.com/badiu.com", false)
	if a != "https://www.example.com/badiu.com" {
		println(a)
		panic(11)
	}

	println("basic query url")
	a = MergeUrlFromHTTPRequest([]byte(`GET /target HTTP/1.1
Host: www.baidu.com

`), "login.php?a=123", true)
	if a != "https://www.baidu.com/target/login.php?a=123" {
		println(a)
		panic(11)
	}
}
