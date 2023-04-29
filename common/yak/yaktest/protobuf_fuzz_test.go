package yaktest

import (
	"testing"
)

func TestProtobuf_Fuzz(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "Protobuf fuzz 测试",
			Src: `
println("---------------START TEST 1------------------")
result, err = fuzz.ProtobufHex("080110012001405f4a5f7b226d657373616765223a22476f6c616e6720576562736f636b6574204d6573736167653a20323032322d30392d30362032323a35333a32392e363336303732202b3038303020435354206d3d2b343632362e333935343136323130227d0a5001").FuzzEveryIndex(fn(index, typ, data) {
	printf("index: %d type: %s data: %s\n", index, typ, data)
	if typ == "varint" {
		return 123
	} elif typ == "string" {
		return "fuzz"
	}
	return 
})
die(err)
println("---------------------------------")
for _, r = range result {
	v = fuzz.ProtobufBytes(r)
	printf("%s\n", v)
	println("---------------------------------")
}
println("---------------END TEST 1------------------")
`,
		},
		{
			Name: "Protobuf fuzz group测试",
			Src: `
println("---------------START TEST 2------------------")
result, err = fuzz.ProtobufHex("0b10b90ab301eb14e3950210b90a18b90ae49502ec14b4010c").FuzzEveryIndex(fn(index, type, data) {
	printf("index: %d type: %s data: %s\n", index, type, data)
	return [123, 456]
})
die(err)
println("---------------------------------")
for _, r = range result {
	v = fuzz.ProtobufBytes(r)
	printf("%s\n", v)
	println("---------------------------------")
}
println("---------------END TEST 2------------------")
`,
		},
		{
			Name: "Protobuf fuzz 错误测试",
			Src: `
println("---------------START TEST 3------------------")
result, err = fuzz.ProtobufHex("xxxxxx").FuzzEveryIndex(fn(index, type, data) {
	return
})
if err != nil {
	printf("[hex] err is no nil, pass: %s\n", err)
} else {
	panic(err)
}
println("---------------------------------")
result, err = fuzz.ProtobufJSON("xxxxxx").FuzzEveryIndex(fn(index, type, data) {
	return
})
println("---------------------------------")
if err != nil {
	printf("[json] err is no nil, pass: %s\n", err)
} else {
	panic(err)
}
println("---------------------------------")
result, err = fuzz.ProtobufYAML("xxxxxx").FuzzEveryIndex(fn(index, type, data) {
	return
})
if err != nil {
	printf("[yaml] err is no nil, pass: %s\n", err)
} else {
	panic(err)
}
println("---------------END TEST 3------------------")
`,
		},
		{
			Name: "Protobuf fuzz 结构体转换测试",
			Src: `
println("---------------START TEST 4------------------")
v = fuzz.ProtobufHex("2206038e029ea705")
println("---------------------------------")
println(v.ToHex())
println("---------------------------------")
println(v.ToYAML())
println("---------------------------------")
println(v.ToJSON())
println("---------------------------------")
println(fuzz.ProtobufHex(v.ToHex()))
println("---------------------------------")
println(fuzz.ProtobufYAML(v.ToYAML()))
println("---------------------------------")
println(fuzz.ProtobufJSON(v.ToJSON()))
println("---------------------------------")
println("---------------END TEST 4------------------")
`,
		},
	}

	Run("Protobuf fuzz 测试", t, cases...)
}
