package spel

import (
	"github.com/yaklang/yaklang/common/utils"
	spelparser "github.com/yaklang/yaklang/common/yak/java/spel/parser"
	"testing"
)

const EXAMPLE = `
'Hello World'
123
123.45
true
null
1 + 2
10 - 5
2 * 3
10 / 2
10 % 3
2 ^ 3
(1 + 2) * 3
1 == 1
1 != 2
2 < 3
3 <= 3
4 > 3
4 >= 4
true and true
true or false
!true
(true and false) or true
2 > 1 ? 'yes' : 'no'
null ?: 'default'
T(java.lang.String)
T(java.lang.Math).random()
T(java.lang.Math).PI
T(java.lang.Math).max(10, 20)
'hello'.toUpperCase()
'hello'.length()
'hello'.concat(' world')
'HELLO'.toLowerCase()
'5.00' matches '^\\d+\\.\\d{2}$'
'test@email.com' matches '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,6}$'
{1,2,3}
{1,2,3}.size()
{1,2,3}[0]
{1,2,3}.get(0)
{1,2,3}.?[#this>1]
{1,2,3}.!['Number' + #this]
{1,2,3}.^[#this>1]
{1,2,3}.$[#this>1]
{'key1':'value1','key2':'value2'}
{'key1':'value1'}['key1']
new int[]{1,2,3}
new int[3]
new String[3]{'a','b','c'}
#variable
#root
#this
#user?.name
#user?.getAddress()?.street
{'a','b','c'}.?[#this != 'b'].![#this + '1']
{1,2,3,4}.?[#this % 2 == 0].![#this * 2]
'hello' + ' ' + 'world'
'hello world'.substring(0,5)
'hello world'.indexOf('o')
'test' instanceof T(String)
new java.util.Date()
T(java.time.LocalDate).now()
T(java.lang.Math).abs(-10)
T(java.lang.Math).round(3.14)
T(java.lang.Math).max(10, 20)
#number > 0 ? 'positive' : (#number < 0 ? 'negative' : 'zero')
{1,2,3,4,5}.?[#this > 2].![(#this * 2) + ' times']
{'a':1,'b':2,'c':3}.?[value>1]
@myBean.property
@myBean.doSomething()
#{systemProperties['user.home']}
#{systemEnvironment['PATH']}
T(java.lang.Math).random() > 0.5 ? {1,2,3}.?[#this > 1] : {4,5,6}.?[#this < 5]
{1,2,3,4,5}.![#this * 2].sum()
{1,2,3,4,5}.average()
#customFunction('arg1', 'arg2')
'hello world'.replaceAll('\\s+', '-')
#nullValue ?: 'default value'
#nullValue != null ? #nullValue : 'default'
T(Integer).parseInt('123')
T(Double).parseDouble('123.45')
2 & 3
2 | 4
~2
2 << 2
new java.util.ArrayList()
new java.util.HashMap()
String.format('%d + %d = %d', 2, 3, 2 + 3)
#person?.address?.city?.name
{1,2,3}.![T(String).valueOf(#this)]
'abc' matches '[a-z]+'
T(java.lang.System).currentTimeMillis()
{1,2,3} contains 2
{1,2,3} instanceof T(java.util.List)
#{'key1': 'value1', 'key2': 'value2'}.keySet()
#{'key1': 'value1', 'key2': 'value2'}.values()
'test'?.bytes?.length
T(java.util.Arrays).asList('a','b','c')
{1,2,3,4,5}.?[#this > 2 and #this < 5]
new String('hello').getBytes().length
T(java.util.UUID).randomUUID().toString()
{1,2,3}.stream().sum()
'hello'.bytes.![T(String).format('%02x', #this)]
{1,2,3}.stream().average().get()
'Hello'.getBytes('UTF-8').length
T(java.time.LocalDateTime).now().plusDays(1)
{1,2,3}.?[#this > 1].size()
`

func TestGetAST(t *testing.T) {
	for line := range utils.ParseLines(EXAMPLE) {
		a, err := GetAST(line)
		if err != nil {
			t.Errorf("GetAST() failed: %v", err)
		}
		scriptIns := a.Script()
		expr := scriptIns.(*spelparser.ScriptContext).SpelExpr()
		if expr == nil {
			t.Errorf("GetAST() failed: expected *spelparser.SpelParser, got: %T", expr)
		}
	}
}
