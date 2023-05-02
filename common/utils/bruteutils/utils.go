package bruteutils

import (
	"fmt"
	"regexp"
	"yaklang.io/yaklang/common/mutate"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
)

func keywordToRegexp(k string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)` + regexp.QuoteMeta(k))
}

func appendDefaultPort(i string, defaultPort int) string {
	host, port, _ := utils.ParseStringToHostPort(i)
	if port <= 0 {
		i = fmt.Sprintf("%v:%v", i, defaultPort)
	} else {
		i = utils.HostPort(host, port)
	}
	return i
}

func packetToBrute(
	packet string,
	data map[string][]string,
	timeout float64,
	isTls bool,
) ([]byte, [][]byte, error) {
	res, _ := mutate.QuickMutate(packet, nil, mutate.MutateWithExtraParams(data))
	if len(res) <= 0 {
		return nil, nil, utils.Error("mutate packet error... BUG!")
	}
	return lowhttp.SendPacketQuick(isTls, []byte(res[0]), timeout)
}

func GeneratePasswordByUser(user []string, pass []string) []string {
	var results []string
	for _, r := range pass {
		for _, b := range user {
			arr, err := mutate.QuickMutate(r, nil, mutate.MutateWithExtraParams(
				map[string][]string{
					"user": {b},
				}))
			if err != nil {
				continue
			}
			results = append(results, arr...)
		}
	}
	return results
}

// http://k8gege.org/p/16172.html
var CommonUsernames = []string{
	"admin", "root", "test", "op", "www", "data",
	"guest",
}

var CommonPasswords = []string{
	"root",
	"123456",
	"admin",
	"!@",
	"wubao",
	"password",
	"12345",
	"1234",
	"p@ssw0rd",
	"123",
	"1",
	"jiamima",
	"test",
	"root123",
	"!",
	"!q@w",
	"!qaz@wsx",
	"idc!@",
	"admin!@",
	"",
	"alpine",
	"qwerty",
	"12345678",
	"111111",
	"123456789",
	"1q2w3e4r",
	"123123",
	"default",
	"1234567",
	"qwe123",
	"1qaz2wsx",
	"1234567890",
	"abcd1234",
	"000000",
	"user",
	"toor",
	"qwer1234",
	"1q2w3e",
	"asdf1234",
	"redhat",
	"1234qwer",
	"cisco",
	"12qwaszx",
	"test123",
	"1q2w3e4r5t",
	"admin123",
	"changeme",
	"1qazxsw2",
	"123qweasd",
	"q1w2e3r4",
	"letmein",
	"server",
	"root1234",
	"master",
	"abc123",
	"rootroot",
	"a",
	"system",
	"pass",
	"1qaz2wsx3edc",
	"p@$$w0rd",
	"112233",
	"welcome",
	"!QAZ2wsx",
	"linux",
	"123321",
	"manager",
	"1qazXSW@",
	"q1w2e3r4t5",
	"oracle",
	"asd123",
	"admin123456",
	"ubnt",
	"123qwe",
	"qazwsxedc",
	"administrator",
	"superuser",
	"zaq12wsx",
	"121212",
	"654321",
	"ubuntu",
	"0000",
	"zxcvbnm",
	"root@123",
	"1111",
	"vmware",
	"q1w2e3",
	"qwerty123",
	"cisco123",
	"11111111",
	"pa55w0rd",
	"asdfgh",
	"11111",
	"123abc",
	"asdf",
	"centos",
	"888888",
	"54321",
	"password123",
	"123456789",
	"a123456",
	"123456",
	"a123456789",
	"1234567890",
	"woaini1314",
	"qq123456",
	"abc123456",
	"123456a",
	"123456789a",
	"147258369",
	"zxcvbnm",
	"987654321",
	"12345678910",
	"abc123",
	"qq123456789",
	"123456789.",
	"7708801314520",
	"woaini",
	"5201314520",
	"q123456",
	"123456abc",
	"1233211234567",
	"123123123",
	"123456.",
	"0123456789",
	"asd123456",
	"aa123456",
	"135792468",
	"q123456789",
	"abcd123456",
	"12345678900",
	"woaini520",
	"woaini123",
	"zxcvbnm123",
	"1111111111111111",
	"w123456",
	"aini1314",
	"abc123456789",
	"111111",
	"woaini521",
	"qwertyuiop",
	"1314520520",
	"1234567891",
	"qwe123456",
	"asd123",
	"000000",
	"1472583690",
	"1357924680",
	"789456123",
	"123456789abc",
	"z123456",
	"1234567899",
	"aaa123456",
	"abcd1234",
	"www123456",
	"123456789q",
	"123abc",
	"qwe123",
	"w123456789",
	"7894561230",
	"123456qq",
	"zxc123456",
	"123456789qq",
	"1111111111",
	"111111111",
	"0000000000000000",
	"1234567891234567",
	"qazwsxedc",
	"qwerty",
	"123456..",
	"zxc123",
	"asdfghjkl",
	"0000000000",
	"1234554321",
	"123456q",
	"123456aa",
	"9876543210",
	"110120119",
	"qaz123456",
	"qq5201314",
	"123698745",
	"5201314",
	"000000000",
	"as123456",
	"123123",
	"5841314520",
	"z123456789",
	"52013145201314",
	"a123123",
	"caonima",
	"a5201314",
	"wang123456",
	"abcd123",
	"123456789..",
	"woaini1314520",
	"123456asd",
	"aa123456789",
	"741852963",
	"a12345678",
}
