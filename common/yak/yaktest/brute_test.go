package yaktest

import "testing"

func TestBrute(t *testing.T) {
	cases := []YakTestCase{
		{Name: "brute basic", Src: `dump(brute.GetAvailableBruteTypes())
bruter, err := brute.New("ssh", 
	brute.concurrentTarget(256), brute.debug(true),
	brute.userList("root", "root123"),
	brute.passList("password", "admin123"),
	brute.bruteHandler(fn(i){
		result = i.Result()
		if result.Username == "root" && result.Password == "admin123" {
			result.Ok = true
			return result
		}
		return result
	}),
)
die(err)

res, err := bruter.Start("localhost:22")
die(err)

for res := range res {
	if res.Ok {
		dump(res)
	}
}
`},
	}
	Run("brute basic test", t, cases...)
}

func TestBruteRedis(t *testing.T) {
	cases := []YakTestCase{
		{Name: "brute redis", Src: `dump(brute.GetAvailableBruteTypes())
bruter, err := brute.New("redis", 
	brute.concurrentTarget(256), brute.debug(true),
	brute.userList("root", "root123"),
	brute.passList("password", "admin123"),
)
die(err)

res, err := bruter.Start("localhost")
die(err)

for res := range res {
	if res.Ok {
		dump(res)
	}
}
`},
	}
	Run("brute basic test", t, cases...)
}

func TestBruteTomcat(t *testing.T) {
	cases := []YakTestCase{
		{Name: "brute tomcat", Src: `dump(brute.GetAvailableBruteTypes())
bruter, err := brute.New("tomcat", 
	brute.concurrentTarget(256), brute.debug(true),
	brute.userList("root", "root123"),
	brute.passList("password", "admin123"),
)
die(err)

res, err := bruter.Start("https://***8.net/manager/html")
die(err)

for res := range res {
	dump(res)
	if res.Ok {
		dump(res)
	}
}
`},
	}
	Run("brute basic test", t, cases...)
}

func TestBrute_MYSQL(t *testing.T) {
	cases := []YakTestCase{
		{Name: "brute tomcat", Src: `dump(brute.GetAvailableBruteTypes())
bruter, err := brute.New("mysql", 
	brute.concurrentTarget(256), brute.debug(true),
	brute.userList("root", "root123", "root"),
	brute.passList("password", "admin123", "123456"),
)
die(err)

res, err := bruter.Start("127.0.0.1:3306")
die(err)

for res := range res {
	dump(res)
	if res.Ok {
		dump(res)
	}
}
`},
	}
	Run("brute basic test", t, cases...)
}
