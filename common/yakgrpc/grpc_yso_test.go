package yakgrpc

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"github.com/yaklang/yaklang/common/yak"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"github.com/yaklang/yaklang/common/yso"
)

func TestGRPCMUSTPASS_GeneratePayload(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ysoVerboses, err := client.GetAllYsoGadgetOptions(ctx, &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	//测试获取所有的yso选项(ysoVerboses, t)
	assert.Equal(t, len(ysoVerboses.GetOptions()), len(yso.AllGadgets)-1)
	for _, option := range ysoVerboses.GetOptions() {
		assert.Equal(t, yso.AllGadgets[yso.GadgetType(option.GetName())].Name, option.GetName())
		classOptions, err := client.GetAllYsoClassOptions(ctx, &ypb.YsoOptionsRequerstWithVerbose{Gadget: option.GetName()})
		if err != nil {
			t.Fatal(err)
		}
		options := classOptions.GetOptions()
		for _, classOption := range options {
			paramsOption, err := client.GetAllYsoClassGeneraterOptions(ctx, &ypb.YsoOptionsRequerstWithVerbose{Gadget: option.GetName(), Class: classOption.GetName()})
			if err != nil {
				t.Fatal(err)
			}
			options := paramsOption.GetOptions()
			for _, option := range options {
				switch option.Type {
				case string(String):
					option.Value = utils.GetRandomIPAddress()
				case string(StringPort):
					option.Value = fmt.Sprint(rand.Int() % 65535)
				case string(StringBool):
					option.Value = fmt.Sprint(rand.Int()%2 == 0)
				case string(Base64Bytes):
					option.Value = "yv66vgAAADQAXgkAMwA0CAA1CgAEADYHADcIADgIADkJABsAOggAHQgAOwoAPAA9CgA8AD4HAD8KAAwAQAoAHABBCQAbAEIIAEMKABsARAgARQgAHwkAGwBGCABHCQAbAEgHAEkKABcAQQoAFwBKCgAXAEsHAEwHAE0BAANjbWQBABJMamF2YS9sYW5nL1N0cmluZzsBAAhpc1N0YXRpYwEAA3llcwEABGZsYWcBAAVzdGFydAEAAygpVgEABENvZGUBAA9MaW5lTnVtYmVyVGFibGUBAA1TdGFja01hcFRhYmxlBwBOBwA/AQAGPGluaXQ+BwBMAQAJdHJhbnNmb3JtAQByKExjb20vc3VuL29yZy9hcGFjaGUveGFsYW4vaW50ZXJuYWwveHNsdGMvRE9NO1tMY29tL3N1bi9vcmcvYXBhY2hlL3htbC9pbnRlcm5hbC9zZXJpYWxpemVyL1NlcmlhbGl6YXRpb25IYW5kbGVyOylWAQAKRXhjZXB0aW9ucwcATwEApihMY29tL3N1bi9vcmcvYXBhY2hlL3hhbGFuL2ludGVybmFsL3hzbHRjL0RPTTtMY29tL3N1bi9vcmcvYXBhY2hlL3htbC9pbnRlcm5hbC9kdG0vRFRNQXhpc0l0ZXJhdG9yO0xjb20vc3VuL29yZy9hcGFjaGUveG1sL2ludGVybmFsL3NlcmlhbGl6ZXIvU2VyaWFsaXphdGlvbkhhbmRsZXI7KVYBAAg8Y2xpbml0PgEAClNvdXJjZUZpbGUBABBSdW50aW1lRXhlYy5qYXZhBwBQDABRAB4BAAEvDABSAFMBABBqYXZhL2xhbmcvU3RyaW5nAQAHL2Jpbi9zaAEAAi1jDAAdAB4BAAIvQwcAVAwAVQBWDABXAFgBABNqYXZhL2lvL0lPRXhjZXB0aW9uDABZACMMACkAIwwAIQAeAQALaXNTdGF0aWNZZXMMACIAIwEAAmlkDAAfAB4BAANZZXMMACAAHgEAF2phdmEvbGFuZy9TdHJpbmdCdWlsZGVyDABaAFsMAFwAXQEACENuSmlqU3ZwAQBAY29tL3N1bi9vcmcvYXBhY2hlL3hhbGFuL2ludGVybmFsL3hzbHRjL3J1bnRpbWUvQWJzdHJhY3RUcmFuc2xldAEAE1tMamF2YS9sYW5nL1N0cmluZzsBADljb20vc3VuL29yZy9hcGFjaGUveGFsYW4vaW50ZXJuYWwveHNsdGMvVHJhbnNsZXRFeGNlcHRpb24BAAxqYXZhL2lvL0ZpbGUBAAlzZXBhcmF0b3IBAAZlcXVhbHMBABUoTGphdmEvbGFuZy9PYmplY3Q7KVoBABFqYXZhL2xhbmcvUnVudGltZQEACmdldFJ1bnRpbWUBABUoKUxqYXZhL2xhbmcvUnVudGltZTsBAARleGVjAQAoKFtMamF2YS9sYW5nL1N0cmluZzspTGphdmEvbGFuZy9Qcm9jZXNzOwEAD3ByaW50U3RhY2tUcmFjZQEABmFwcGVuZAEALShMamF2YS9sYW5nL1N0cmluZzspTGphdmEvbGFuZy9TdHJpbmdCdWlsZGVyOwEACHRvU3RyaW5nAQAUKClMamF2YS9sYW5nL1N0cmluZzsAIQAbABwAAAAEAAoAHQAeAAAACgAfAB4AAAAKACAAHgAAAAoAIQAeAAAABQAJACIAIwABACQAAACZAAQAAgAAAEmyAAESArYAA5kAGwa9AARZAxIFU1kEEgZTWQWyAAdTS6cAGAa9AARZAxIIU1kEEglTWQWyAAdTS7gACiq2AAtXpwAITCu2AA2xAAEAOABAAEMADAACACUAAAAiAAgAAAAWAAsAFwAjABkAOAAdAEAAIABDAB4ARAAfAEgAIQAmAAAADgAEI/wAFAcAJ0oHACgEAAEAKQAjAAEAJAAAAEkAAgABAAAAEyq3AA6yAA8SELYAA5oABrgAEbEAAAACACUAAAASAAQAAAAnAAQAKAAPACkAEgAqACYAAAAMAAH/ABIAAQcAKgAAAAEAKwAsAAIAJAAAABkAAAADAAAAAbEAAAABACUAAAAGAAEAAAAtAC0AAAAEAAEALgABACsALwACACQAAAAZAAAABAAAAAGxAAAAAQAlAAAABgABAAAAMAAtAAAABAABAC4ACAAwACMAAQAkAAAAcAACAAAAAAA3EhKzAAcSE7MAFBIVswAWuwAXWbcAGLIAFLYAGbIAFrYAGbYAGrMAD7IADxIQtgADmQAGuAARsQAAAAIAJQAAAB4ABwAAABAABQARAAoAEgAPACMAKAAkADMAJQA2ACYAJgAAAAMAATYAAQAxAAAAAgAy"
				}
			}
			rsp, err := client.GenerateYsoBytes(ctx, &ypb.YsoOptionsRequerstWithVerbose{
				Gadget:  option.GetName(),
				Class:   classOption.GetName(),
				Options: options,
			})
			if err != nil {
				t.Fatal(fmt.Sprintf("GenerateYsoBytes error: %v, gadget: %s, class: %s", err, option.GetName(), classOption.GetName()))
			}
			if len(rsp.Bytes) == 0 {
				t.Fatal(fmt.Sprintf("rsp.Bytes is empty, gadget: %s, class: %s", option.GetName(), classOption.GetName()))
			}
		}
	}
}
func TestGRPCMUSTPASS_GenerateYakCode(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cb := func(req *ypb.YsoOptionsRequerstWithVerbose) {
		rsp, err := client.GenerateYsoCode(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		e, err := yak.Execute(rsp.Code)
		if err != nil {
			t.Fatal(err)
		}
		v, ok := e.GetVar("gadgetBytes")
		if !ok || v == nil {
			t.Fatalf("gadgetBytes is nil, gadget: %s, class: %s,code: %s", req.Gadget, req.Class, rsp.Code)
		}
	}

	ysoVerboses, err := client.GetAllYsoGadgetOptions(ctx, &ypb.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	//测试获取所有的yso选项(ysoVerboses, t)
	assert.Equal(t, len(ysoVerboses.GetOptions()), len(yso.AllGadgets)-1)
	for _, option := range ysoVerboses.GetOptions() {
		assert.Equal(t, yso.AllGadgets[yso.GadgetType(option.GetName())].Name, option.GetName())
		classOptions, err := client.GetAllYsoClassOptions(ctx, &ypb.YsoOptionsRequerstWithVerbose{Gadget: option.GetName()})
		if err != nil {
			t.Fatal(err)
		}
		options := classOptions.GetOptions()
		for _, classOption := range options {
			paramsOption, err := client.GetAllYsoClassGeneraterOptions(ctx, &ypb.YsoOptionsRequerstWithVerbose{Gadget: option.GetName(), Class: classOption.GetName()})
			if err != nil {
				t.Fatal(err)
			}
			options := paramsOption.GetOptions()
			for _, option := range options {
				if option.Key == string(JavaClassGeneraterOption_DirtyData) {
					option.Value = "100"
					continue
				}
				switch option.Type {
				case string(String):
					option.Value = utils.GetRandomIPAddress()
				case string(StringPort):
					option.Value = fmt.Sprint(rand.Int() % 65535)
				case string(StringBool):
					option.Value = fmt.Sprint(rand.Int()%2 == 0)
				case string(Base64Bytes):
					option.Value = "yv66vgAAADQAXgkAMwA0CAA1CgAEADYHADcIADgIADkJABsAOggAHQgAOwoAPAA9CgA8AD4HAD8KAAwAQAoAHABBCQAbAEIIAEMKABsARAgARQgAHwkAGwBGCABHCQAbAEgHAEkKABcAQQoAFwBKCgAXAEsHAEwHAE0BAANjbWQBABJMamF2YS9sYW5nL1N0cmluZzsBAAhpc1N0YXRpYwEAA3llcwEABGZsYWcBAAVzdGFydAEAAygpVgEABENvZGUBAA9MaW5lTnVtYmVyVGFibGUBAA1TdGFja01hcFRhYmxlBwBOBwA/AQAGPGluaXQ+BwBMAQAJdHJhbnNmb3JtAQByKExjb20vc3VuL29yZy9hcGFjaGUveGFsYW4vaW50ZXJuYWwveHNsdGMvRE9NO1tMY29tL3N1bi9vcmcvYXBhY2hlL3htbC9pbnRlcm5hbC9zZXJpYWxpemVyL1NlcmlhbGl6YXRpb25IYW5kbGVyOylWAQAKRXhjZXB0aW9ucwcATwEApihMY29tL3N1bi9vcmcvYXBhY2hlL3hhbGFuL2ludGVybmFsL3hzbHRjL0RPTTtMY29tL3N1bi9vcmcvYXBhY2hlL3htbC9pbnRlcm5hbC9kdG0vRFRNQXhpc0l0ZXJhdG9yO0xjb20vc3VuL29yZy9hcGFjaGUveG1sL2ludGVybmFsL3NlcmlhbGl6ZXIvU2VyaWFsaXphdGlvbkhhbmRsZXI7KVYBAAg8Y2xpbml0PgEAClNvdXJjZUZpbGUBABBSdW50aW1lRXhlYy5qYXZhBwBQDABRAB4BAAEvDABSAFMBABBqYXZhL2xhbmcvU3RyaW5nAQAHL2Jpbi9zaAEAAi1jDAAdAB4BAAIvQwcAVAwAVQBWDABXAFgBABNqYXZhL2lvL0lPRXhjZXB0aW9uDABZACMMACkAIwwAIQAeAQALaXNTdGF0aWNZZXMMACIAIwEAAmlkDAAfAB4BAANZZXMMACAAHgEAF2phdmEvbGFuZy9TdHJpbmdCdWlsZGVyDABaAFsMAFwAXQEACENuSmlqU3ZwAQBAY29tL3N1bi9vcmcvYXBhY2hlL3hhbGFuL2ludGVybmFsL3hzbHRjL3J1bnRpbWUvQWJzdHJhY3RUcmFuc2xldAEAE1tMamF2YS9sYW5nL1N0cmluZzsBADljb20vc3VuL29yZy9hcGFjaGUveGFsYW4vaW50ZXJuYWwveHNsdGMvVHJhbnNsZXRFeGNlcHRpb24BAAxqYXZhL2lvL0ZpbGUBAAlzZXBhcmF0b3IBAAZlcXVhbHMBABUoTGphdmEvbGFuZy9PYmplY3Q7KVoBABFqYXZhL2xhbmcvUnVudGltZQEACmdldFJ1bnRpbWUBABUoKUxqYXZhL2xhbmcvUnVudGltZTsBAARleGVjAQAoKFtMamF2YS9sYW5nL1N0cmluZzspTGphdmEvbGFuZy9Qcm9jZXNzOwEAD3ByaW50U3RhY2tUcmFjZQEABmFwcGVuZAEALShMamF2YS9sYW5nL1N0cmluZzspTGphdmEvbGFuZy9TdHJpbmdCdWlsZGVyOwEACHRvU3RyaW5nAQAUKClMamF2YS9sYW5nL1N0cmluZzsAIQAbABwAAAAEAAoAHQAeAAAACgAfAB4AAAAKACAAHgAAAAoAIQAeAAAABQAJACIAIwABACQAAACZAAQAAgAAAEmyAAESArYAA5kAGwa9AARZAxIFU1kEEgZTWQWyAAdTS6cAGAa9AARZAxIIU1kEEglTWQWyAAdTS7gACiq2AAtXpwAITCu2AA2xAAEAOABAAEMADAACACUAAAAiAAgAAAAWAAsAFwAjABkAOAAdAEAAIABDAB4ARAAfAEgAIQAmAAAADgAEI/wAFAcAJ0oHACgEAAEAKQAjAAEAJAAAAEkAAgABAAAAEyq3AA6yAA8SELYAA5oABrgAEbEAAAACACUAAAASAAQAAAAnAAQAKAAPACkAEgAqACYAAAAMAAH/ABIAAQcAKgAAAAEAKwAsAAIAJAAAABkAAAADAAAAAbEAAAABACUAAAAGAAEAAAAtAC0AAAAEAAEALgABACsALwACACQAAAAZAAAABAAAAAGxAAAAAQAlAAAABgABAAAAMAAtAAAABAABAC4ACAAwACMAAQAkAAAAcAACAAAAAAA3EhKzAAcSE7MAFBIVswAWuwAXWbcAGLIAFLYAGbIAFrYAGbYAGrMAD7IADxIQtgADmQAGuAARsQAAAAIAJQAAAB4ABwAAABAABQARAAoAEgAPACMAKAAkADMAJQA2ACYAJgAAAAMAATYAAQAxAAAAAgAy"
				}
			}
			cb(&ypb.YsoOptionsRequerstWithVerbose{
				Gadget:  option.GetName(),
				Class:   classOption.GetName(),
				Options: options,
			})
		}
	}
}
