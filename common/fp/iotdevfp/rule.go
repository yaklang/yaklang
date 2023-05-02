package iotdevfp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"github.com/yaklang/yaklang/common/utils"
)

type IotDevRule struct {
	// app
	AppClass         string
	AppVersion       string // regexp
	AppVersionRegexp *regexp.Regexp
	AppName          string // vendor + product

	// device
	DeviceClass       string
	DeviceModel       string
	DeviceModelRegexp *regexp.Regexp
	DeviceVendor      string

	//
	Flag       string // regxp
	FlagRegexp *regexp.Regexp
	IsDevice   bool

	Depends []string
	Implies map[string]string
}

/*
   {
       "dev_class": "router",
       "dev_model": "JHR-N825R",
       "dev_vendor": "JCG",
       "flag": "JHR-N825R",
       "is_dev": true
   },

    {
        "dev_class": "switch",
        "dev_model": "(fs|fsm)\\d+\\w*",
        "dev_vendor": "netgear",
        "flag": "(fs|fsm)\\d+\\w*",
        "is_dev": true
    },

*/

var extraRuleRaw = `[
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tongwei",
        "flag": "IDCS_LOGIN_NBSP",
        "is_dev": true
    },
	{
		"dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "geovision",
		"flag": "ng-bind.*?oLan.login",
        "is_dev": true
	}
]`

var disabledRaw = `[
    {
        "dev_class": "router",
        "dev_model": "honor Pro 2", 
        "dev_vendor": "huawei",
        "flag": "unsafe-inline",
        "is_dev": true
    }
]`

var ruleRaw = `[
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "couchbase:couchbase_server",
        "flag": "kafka-bigquery",
        "is_dev": false
    },
     {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "php:php",
        "flag": "X-Powered-By\\s*:.*php",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "php:php",
        "flag": "Set-Cookie\\s*:.*PHPSSIONID",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "jsp:jsp",
        "flag": "Set-Cookie\\s*:.*JSESSIONID",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "microsoft:asp",
        "flag": "Set-Cookie\\s*:.*ASPSESSION",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "depends": ["microsoft:.net_framework"],
        "app_version": "",
        "app_name": "microsoft:aspx",
        "flag": "Set-Cookie\\s*:.*ASP.NET_SessionId",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "app_version": "\\d+\\.\\d+\\.\\d+",
        "app_name": "microsoft:.net_framework",
        "flag": "X-AspNet-Version\\s*:\\s*\\d+\\.\\d+\\.\\d+",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "microsoft:aspx",
        "flag": "<input[^>]+name=\\\"__VIEWSTATE",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "microsoft:aspx",
        "flag": "<a[^>]*?href=('|\")[^http][^>]*?\\.aspx(\\?|\\#|\\1)",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "microsoft:asp",
        "flag": "<a[^>]*?href=('|\")[^http][^>]*?\\.asp(\\?|\\#|\\1)",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "php:php",
        "flag": "<a[^>]*?href=('|\")[^http][^>]*?\\.php(\\?|\\#|\\1)",
        "is_dev": false
    },
    {
        "app_class": "web_langeuage",
        "app_version": "",
        "app_name": "jsp:jsp",
        "flag": "<a[^>]*?href=('|\")[^http][^>]*?\\.jsp(\\?|\\#|\\1)",
        "is_dev": false
    },
    {
        "dev_class": "firewall",
        "dev_model": "SANGFOR终端检测响应平台",
        "dev_vendor": "sangfor",
        "flag": "linux_edr_installer",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "TopApp-LB 负载均衡系统",
        "dev_vendor": "topapp-lb",
        "flag": "TopApp-LB",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "JHR-N825R",
        "dev_vendor": "JCG",
        "flag": "JHR-N825R",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "srx240h2",
        "dev_vendor": "Juniper",
        "flag": "SRX240H2",
        "is_dev": true
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "aws:kinesis",
        "flag": "Kinesis Autoscaling",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name":  "bigquery:bigquery",
        "flag": "Matillion ETL for BigQuery",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "metabase:metabase",
        "flag": "metabaseBootstrap",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "aws:redshift",
        "flag": "redshift",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "clusters-summary:clusters-summary",
        "flag": "Clusters Summary",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "alibaba:jstorm",
        "flag": "\\<title\\>JStorm\\<\/title\\>",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "alibaba:jstorm",
        "flag": "\\<title\\>JStorm\\<\/title\\>",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name":  "ibase4j:ibase4j",
        "flag": "content \\=\\“ iBase4J\\”",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "alibaba:aliyunmqs",
        "flag": "AliyunMQS",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "huawei:fusioninsight_hd",
        "flag": "FusionInsight",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:oozie",
        "flag": "Oozie Web Console",
        "is_dev": false
    },

    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:jackrabbit",
        "flag": "Apache Jackrabbit JCR Server",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:helix",
        "flag": "Helix",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "alluxio:alluxio",
        "flag": "Cassandra Web View",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "tableau:tableau_server",
        "flag": "Tableau Server",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:storm",
        "flag": "\\<title\\>Storm UI\\<\/title\\>",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:superset",
        "flag": "<title>Superset</title>",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:zeppelin",
        "flag": "Welcome to Zeppelin",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:griffin",
        "flag": "Griffin",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:presto",
        "flag": "Apache presto",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:hive",
        "flag": "Apache Hive",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:kylin",
        "flag": "Kylin",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name":  "apache:atlas",
        "flag": "Atlas",
        "is_dev": false
    },        
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:kafka",
        "flag": "Kafka",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:flume",
        "flag": "Flume",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:ranger",
        "flag": "Ranger",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:zookeeper",
        "flag": "Apache Zookeeper",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name":  "apache:sqoop",
        "flag": "Sqoop",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:flink",
        "flag": "Apache Flink",
        "is_dev": false
    },
    {
        "depends": ["apache:hadoop"],
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:hbase",
        "flag": "HBase",
        "is_dev": false
    },
    {
        "dev_class": "router-safety",
        "dev_model": "",
        "dev_vendor": "Imperva SecureSphere",
        "flag": "SecureSphere",
        "is_dev": true
    },
    {
        "flag": "basic\\s*realm=\"cprn-nlw\"",
        "dev_model": "nlw",
        "dev_vendor": "hitachi",
        "dev_class": "nlw",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "dahua",
        "flag": "digest\\s*realm=\"login\\s*to",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "dahua",
        "flag": "server:\\s*dahua",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "flag": "digest\\s*realm=\"hikvision",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "aszeno",
        "flag": "aszeno\\s*rtsp",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "hipcam",
        "flag": "server:\\s*hipcam\\s*realserver",
        "is_dev": true
    },
     {
        "dev_class": "dvs",
        "dev_model": "dvs2000",
        "dev_vendor": "domex",
        "flag": "DVS2000 by Domex",
        "is_dev": true
    },
    {
        "dev_class": "dvs",
        "dev_model": "hd-dvs-xxx",
        "dev_vendor": "hikvision",
        "flag": "Server: DVS-Webs",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tas-tech",
        "flag": "Server: TAS-Tech",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "ip116_plus",
        "dev_vendor": "chuango",
        "flag": "Chuango login",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "shihuaanxin",
        "flag": "Digest realm=\"HIipCamera",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "yanzhong",
        "flag": "RTSP/1.0 405.*SET_PARAMETER,USER_CMD_SET",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "qualvision",
        "flag": "qv\\s*rtsp",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "qualvision",
        "flag": "server:\\s*qualvision",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "dahua",
        "flag": "dahuartsp",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tvt",
        "favicon": {
            "hash": 492290497
        },
        "is_dev": true
    },
    {
        "dev_class": "",
        "dev_model": "",
        "dev_vendor": "axis",
        "favicon": {
            "hash": -1616143106
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "ecor",
        "flag": "realm=\"ecor\\d*-\\s*\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tangqiao",
        "flag": "<script>alert('sorry,you need to use ie brower!')</script>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "canon",
        "flag": "server:\\s*vb",
        "is_dev": true
    },
    {
        "dev_class": "vms",
        "dev_model": "ivms_8700",
        "dev_vendor": "hikvison",
        "flag": "ivms.*8700",
        "is_dev": true
    },
    {
        "dev_class": "vms",
        "dev_model": "dvms 6.5f",
        "dev_vendor": "onssi",
        "flag": "<title>netdvms\\s*6.5f",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "onssi",
        "flag": "<title>onssi",
        "is_dev": true
    },
    {
        "dev_class": "nas",
        "dev_model": "prosight-smb 6.5c",
        "dev_vendor": "onssi",
        "flag": "<title>prosight-smb\\s*6\\.5c",
        "is_dev": true
    },
    {
        "dev_class": "nas",
        "dev_model": "rc-i 7.0b",
        "dev_vendor": "onssi",
        "flag": "<title>rc-i\\s*7\\.0b</title>",
        "is_dev": true
    },
    {
        "dev_class": "dvr",
        "dev_model": "netdvrv3",
        "dev_vendor": "netdvr",
        "flag": "<title>netdvrv3",
        "is_dev": true
    },
    {
        "dev_class": "dvr",
        "dev_model": "",
        "dev_vendor": "",
        "flag": "mini_httpd/1\\.19 19dec2003",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "videoiq",
        "flag": "<title>videoiq\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tvt",
        "flag": "\\d\\.\\d\\.\\d\\s\\s*server\\s*\\(basler\\s*ipcam\\)",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "mobotix",
        "flag": "basic realm=\"mobotix camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "siemens",
        "flag": "realm=\"compact\\s*siemens\\s*ip\\s*camera\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "siemens",
        "flag": "<title>siemens\\s*ip-camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "honeywell",
        "flag": "<title>honeywell\\s*ip\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "sony",
        "flag": "sony\\s*network\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "aja",
        "flag": "<title>aja\\s*\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "acti",
        "flag": "acti\\s*corporation\\s*all\\s*rights\\s*reserved  ",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "acti",
        "flag": "<title>web\\sconfigurator</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "acti",
        "flag": "url=/cgi-bin/videoconfiguration.cgi",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "acti",
        "flag": "vicworl_sessid=",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "dreamsoft",
        "flag": "<title>web\\sconfigurator</title>",
        "is_dev": true
    },
    {
        "dev_class": "vcs",
        "dev_model": "",
        "dev_vendor": "avcon",
        "flag": "<title>avcon6",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "axis",
        "flag": "<title>avhs&nbsp;</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "axis\\s*\\d+\\s*network\\s*camera",
        "dev_vendor": "axis",
        "flag": "axis\\s*\\d+\\s*network\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "vhs",
        "dev_model": "",
        "dev_vendor": "axis",
        "flag": "axis\\s*video\\s*hosting\\s*system",
        "is_dev": true
    },
    {
        "dev_class": "tvrs",
        "dev_model": "",
        "dev_vendor": "dreambox",
        "flag": "basic\\s*realm=\"dreambox\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "panasonic",
        "flag": "basic\\s*realm=\"panasonic\\s*network\\s*device\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "wv-\\w\\d+\\w*",
        "dev_vendor": "panasonic",
        "flag": "<title>wv-\\w\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dg-\\w\\d+\\w*",
        "dev_vendor": "panasonic",
        "flag": "<title>dg-\\w\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "wj-\\w\\d+\\w*",
        "dev_vendor": "panasonic",
        "flag": "<title>wj-\\w\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "panasonic",
        "flag": "src=\"cgitagmenu?page=top&language=",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "d-link",
        "flag": "d-link\\s*internet\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "d-link",
        "flag": "basic\\s*realm=\"dcs-\\s*\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "trendnet",
        "flag": "digest\\s*realm=\"tv-ip\\s*\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "trendnet",
        "flag": "basic\\s*realm=\"netcam\"",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "tv-nvr\\s*\\w*",
        "dev_vendor": "trendnet",
        "flag": "basic\\s*realm=\"tv-nvr\\s*w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "bluenet",
        "flag": "<title>bluenet\\s*video\\s*viewer\\s*version\\s*\\d\\.\\d\\.\\d</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "direct-packet",
        "flag": "basic\\s*realm=\"dprweb\\s*server\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "direct-packet",
        "flag": "dpwebserver",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "evocam",
        "flag": "<title>live\\s*video\\s*stream",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "evocam",
        "flag": "<title>evocam",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "evocam",
        "flag": "<title>southcreekvillage,tx weather</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "everfocus",
        "flag": "digest\\s*realm=\"everfocus\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "everfocus",
        "flag": "http\\s*server/everfocus",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "heitel",
        "flag": "heitel\\s*gmbh\\s*web\\s*serve",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "i3micro vrg",
        "dev_vendor": "i3micro",
        "flag": "digest\\s*realm=\"i3micro\\s*vrg\",",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "intellinet",
        "flag": "intellinet\\s*network\\s*solutions",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "iqinvision",
        "flag": "iqinvision",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "iqinvision",
        "flag": "iq\\s*\\s*\\s*\\s*live\\s*images",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "nuuo",
        "flag": "nuuo\\s*web\\s*remote",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "nuuo",
        "flag": "<title>web\\s*remote\\s*client</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "stardot",
        "flag": "netcam.*live image",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "stardot",
        "flag": "express.*live image",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "reecam",
        "flag": "reecam\\s*ip\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "bosch",
        "flag": "bosch\\s*security",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "bosch",
        "flag": "vcs-videojet-webserver",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "linksys",
        "flag": "'\\+tm01\\+'",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "linksys",
        "flag": "<title>linksys.*camera</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "linksys",
        "flag": "<title>Linksys Internet Camera</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "flexidome",
        "flag": "flexidome\\s*ip\\s*outdoor",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "bosch",
        "flag": "dinion\\s*ip\\s*bullet",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tp-link",
        "flag": "<title>ip\\s*camera\\s*viewer",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "foscam",
        "flag": "<title>ipcam\\s*client",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "juanvision",
        "flag": "jaws/\\d*\\.*\\d*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "webcamxp",
        "flag": "webcamxp",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "orite",
        "flag": "<TITLE>Orite\\s*\\w+\\d+",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "kedacom",
        "flag": "kedacom-hs",
        "is_dev": true
    },
     {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "kedacom",
        "flag": "nvr\\s*station\\s*web",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "kedacom",
        "flag": "<title>nvr_station\\s*web",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "unsecurity",
        "flag": "<title\\s*data-text=\"text\\.videomanagesystem\">",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "rvi-camera",
        "flag": "<a\\s*href=\"xdview.exe\">",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "vimicro",
        "flag": "vilar\\s*ipcamera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "huawei",
        "flag": "<title>HUAWEI IPC",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "huawei",
        "flag": "Server: HWServer",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "HWSERVER-*\\w+",
        "dev_vendor": "huawei",
        "flag": "Server \\(HWSERVER-*\\w+",
        "is_dev": true
    },
     {
        "dev_class": "media server",
        "dev_model": "",
        "dev_vendor": "cisco",
        "flag": "Server: Cisco MediaSense Media Server",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "PVC-300",
        "dev_vendor": "cisco",
        "flag": "Basic realm=\"PVC-300\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "vimicro",
        "flag": "visiondigi\\s*rtsp\\s*server",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "polycom",
        "flag": "<title>Polycom Login",
        "is_dev": true
    },
      {
        "dev_class": "ip_cam",
        "dev_model": "ZXV10",
        "dev_vendor": "zte",
        "flag": "Server: ZXV10 PUSS",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "netwave",
        "flag": "netwave\\s*ip\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "juanvision",
        "favicon": {
            "hash": 90066852
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "abus",
        "favicon": {
            "hash": -313860026
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "unsecurity",
        "favicon": {
            "hash": -1240222446
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "unisight",
        "flag": "basic\\s*realm=\"unisight\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "yunshitong",
        "flag": "remote\\s*mgmt\\s*system",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "znv",
        "flag": "zxnvm",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "znv",
        "flag": "hdipcam",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "znv",
        "flag": ">network\\s*video\\s*surveillance\\s*system",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tiandy",
        "flag": "<title>net\\s*video\\s*browser",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tiandy",
        "flag": "<title>omny\\s*ip\\s*\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "lg",
        "flag": "lg\\s*smart\\s*ip\\s*device",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tiandy",
        "flag": "tiandy\\s*rtsp\\s*server",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "axis",
        "flag": "axis.*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "infinova",
        "flag": "infinova-webs",
        "is_dev": true
    },
    {
        "dev_class": "dvr",
        "dev_model": "",
        "dev_vendor": "cpplus",
        "flag": "cpplus\\s*dvr\\s*–\\s*web\\s*view",
        "is_dev": true
    },
    {
        "dev_class": "dvr",
        "dev_model": "",
        "dev_vendor": "",
        "flag": "DVR WebClient",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "intelbras",
        "flag": "intelbras",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "kedacom",
        "flag": "<title>.*viewshot.*</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "webguard",
        "flag": "<title>webguard.*</title>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "novus",
        "flag": "novus\\s*rtsp\\s*server",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "webeye",
        "flag": "<title>webeye",
        "is_dev": true
    },
    {
        "dev_class": "dvr",
        "dev_model": "",
        "dev_vendor": "webdvr",
        "flag": "<title>.*webdvr",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "vivotek",
        "flag": "network-camera\\s*ftp\\s*server\\s*\\(.*\\)\\s*ready.",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "vivotek",
        "flag": "wireless\\s*network\\s* camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "vivotek",
        "flag": "basic\\s*realm=\"network\\s*camera\\s*with\\s*pan/tilt\"",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "vivotek",
        "flag": "Vivotek Network Camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "visiongs",
        "flag": "visiongs",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "visec",
        "flag": "visec",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "veo-observer",
        "flag": "veo\\s*observer",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "toshiba",
        "flag": "toshiba\\s*network\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "march",
        "flag": "march\\s*networks\\s*command\\s*client",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "nvr-0208",
        "dev_vendor": "levelone",
        "flag": "basic\\s*realm=\"nvr-0208",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tvt",
        "flag": "tvt\\s*rtsp\\s*serve",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tvt",
        "flag": "dvr\\s*components\\s*download",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tvt",
        "flag": "cross\\s*web\\s*server",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tvt",
        "flag": "webcam",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "zavio",
        "flag": "basic\\s*realm=\"\\s*\\s*\\s*\\s*bullet\\s*camera\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "eagle eye",
        "flag": "eagle\\s*eye\\s*networks",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "abelcam",
        "flag": "abelcam",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "ptz",
        "dev_vendor": "abelcam",
        "flag": "abelcam.*ptz ",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "agasio",
        "flag": "wificam",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dm-ap240",
        "dev_vendor": "dongyoung",
        "flag": "dm-ap240\\s*webserver ",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dm-ap240t",
        "dev_vendor": "dongyoung",
        "flag": "dm-ap240t\\s*webserver ",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "lilin",
        "flag": "basic\\s*realm=\"merit\\s*lilin",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "tv723",
        "dev_vendor": "abus",
        "flag": "basic\\s*realm=\"tv7230",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "aic250",
        "dev_vendor": "airlink",
        "flag": "basic\\s*realm=aic250",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "skyipcam",
        "dev_vendor": "airlink",
        "flag": "basic\\s*realm=\"skyipcam",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "wl-2600cam",
        "dev_vendor": "airlink",
        "flag": "basic\\s*realm=\"wl-2600cam",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "2205",
        "dev_vendor": "allnet",
        "flag": "basic\\s*realm=\"all2205\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "2272",
        "dev_vendor": "allnet",
        "flag": "basic\\s*realm=\"all2272\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "brickcom.*",
        "dev_vendor": "brickcom",
        "flag": "basic\\s*realm=\"brickcom.*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "cs-\\s*",
        "dev_vendor": "planex",
        "flag": "basic\\s*realm=\"cs-\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "cg-\\s*",
        "dev_vendor": "planex",
        "flag": "basic\\s*realm=\"cg-\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "blueiris",
        "flag": "blueiris",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "hb-dvr/\\d+\\.\\d+",
        "dev_vendor": "hbgk",
        "flag": "hb-dvr/\\d+\\.\\d+",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "hbgk",
        "flag": "hanbang\\s*web\\s*service",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "flag": "nvr\\s*webserver",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "flag": "dnvrs-webs",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "d\\d*[a-z]{0,1}-\\s*-.*-camera",
        "dev_vendor": "zavio",
        "flag": "\\s*realm=\"d\\d*[a-z]{0,1}-\\s*-.*-camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "b\\d*[a-z]{0,1}-\\s*-.*-camera",
        "dev_vendor": "zavio",
        "flag": "\\s*realm=\"b\\d*[a-z]{0,1}-\\s*-.*-camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "d\\d*\\s*biuro",
        "dev_vendor": "zavio",
        "flag": "\\s*realm=\"d\\d*\\s*biuro",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "d\\d*[a-z]{0,1}-\\s*-.*-dome",
        "dev_vendor": "zavio",
        "flag": "\\s*realm=\"d\\d*[a-z]{0,1}-\\s*-.*-dome",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "d\\d*[a-z]{0,1}.*entree",
        "dev_vendor": "zavio",
        "flag": "\\s*realm=\"d\\d*[a-z]{0,1}.*entree",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "levelone",
        "flag": "(levelone\\s){0,1}wcs-\\s*(\\s*.*camera){0,1}",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "(levelone\\s){0,1}fcs-\\s*(\\s*.*camera){0,1}",
        "dev_vendor": "levelone",
        "flag": "(levelone\\s){0,1}fcs-\\s*(\\s*.*camera){0,1}",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "networkcamera-[a-z]\\s*-{0,1}\\s*",
        "dev_vendor": "networkcamera",
        "flag": "networkcamera-[a-z]\\s*-{0,1}\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "nfc\\d*-\\w*",
        "dev_vendor": "intellinet",
        "flag": "basic\\s*realm=\"nfc\\d*-\\w*\\s*.*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "p\\w*\\s",
        "dev_vendor": "panoramic",
        "flag": "basic\\s*realm=\"p\\w*\\s*\\w*.*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "s\\w*-\\w*",
        "dev_vendor": "sony",
        "flag": "basic\\s*realm=\"sony.*camera\\s*s\\w*-\\w*\"",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "TL-\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "tp-link",
        "flag": "title>TL-\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "tl-ipcxxx",
        "dev_vendor": "tp-link",
        "flag": "digest\\s*realm=\"tp-link\\s*ip-camera\"",
        "is_dev": true
    },
      {
        "dev_class": "vms",
        "dev_model": "ivms",
        "dev_vendor": "hikvision",
        "flag": "cms/login?.+%2Fweb%2Fgateway%2Fhome.action",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "tl-ipcxxx",
        "dev_vendor": "mercury",
        "flag": "MERCURY RTSP Server",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "mc-ipcxxx",
        "dev_vendor": "tp-link",
        "flag": "TP-LINK RTSP Server",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "tl-ipcxxx",
        "dev_vendor": "tp-link",
        "flag": "digest\\s*realm=\"tp-link\\s*ip-camera\"",
        "is_dev": true
    },
      {
        "dev_class": "ip_cam",
        "dev_model": "mc-ipcxxx",
        "dev_vendor": "mercury",
        "flag": "digest\\s*realm=\"MERCURY\\s*ip-camera\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "tv[-]{0,1}ip\\w*(-|_){0,1}\\w{0,3}",
        "dev_vendor": "trendnet",
        "flag": "tv[-]{0,1}ip\\w*(-|_){0,1}\\w{0,3}",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "huawei",
        "flag": "huawei\\s*ipc",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "xm030",
        "flag": "netsurveillance\\s*web",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "xm030",
        "flag": "http://xmsecu.com:8080/ocx/NewActive.exe",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "axis \\w* ",
        "dev_vendor": "axis",
        "flag": "axis \\w* video server",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "\\w*-\\w*",
        "dev_vendor": "airca",
        "flag": "basic\\s*realm=\"aircam\\s*\\w*-\\w*\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "belkin",
        "flag": "camera\\s*web\\s*server",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "indigo-security/\\d*\\.*\\d*",
        "dev_vendor": "indigo",
        "flag": "indigo-security/\\d*\\.*\\d*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "ubnt\\s*streaming\\s*server\\s*v\\d*\\.*\\d*",
        "dev_vendor": "ubnt",
        "flag": "ubnt\\s*streaming\\s*server\\s*v\\d*\\.*\\d*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "v\\d*r\\d*",
        "dev_vendor": "grandstream",
        "flag": "grandstream\\s*rtsp\\s*server\\s*v\\d*r\\d*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "flussonic",
        "flag": "flussonic",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "\\d*-\\s*",
        "dev_vendor": "ecor",
        "flag": "realm=\"ecor\\d*-\\s*\"",
        "is_dev": true
    },
    {
        "dev_class": "",
        "dev_model": "",
        "dev_vendor": "",
        "flag": "mini_httpd/1\\.19 19dec2003",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "\\d\\.\\d\\.\\d",
        "dev_vendor": "tvt",
        "flag": "\\d\\.\\d\\.\\d\\s*\\s*server\\s*\\(basler\\s*ipcam\\)",
        "is_dev": true
    },
    {
        "dev_class": "vcs",
        "dev_model": "avcon6",
        "dev_vendor": "avcon",
        "flag": "<title>avcon6",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "axis\\s*\\d+\\s*",
        "dev_vendor": "axis",
        "flag": "axis\\s*\\d+\\s*network\\s*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "wv-\\s*\\s*\\w*",
        "dev_vendor": "panasonic",
        "flag": "<title>wv-\\s*\\s*\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dg-\\s*\\s*\\w*",
        "dev_vendor": "panasonic",
        "flag": "<title>dg-\\s*\\s*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "wj-\\s*\\s*\\w*",
        "dev_vendor": "panasonic",
        "flag": "<title>wj-\\s*\\s*\\s*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dcs-\\w*(-\\w*|\\+){0,1}",
        "dev_vendor": "d-link",
        "flag": "basic\\s*realm=\"dcs-\\w*(-\\w*|\\+){0,1}\"",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "dcs-\\w*(-\\w*|\\+){0,1}",
        "dev_vendor": "d-link",
        "flag": "digest\\s*realm=\"dcs-\\w*(-\\w*|\\+){0,1}\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "tv-ip\\s*",
        "dev_vendor": "trendnet",
        "flag": "digest\\s*realm=\"tv-ip\\s*\"",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "tv-nvr\\w*",
        "dev_vendor": "trendnet",
        "flag": "realm=\"tv-nvr\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "\\d\\.\\d\\.\\d",
        "dev_vendor": "bluenet",
        "flag": "<title>bluenet\\s*video\\s*viewer\\s*version\\s*\\d\\.\\d\\.\\d</title>",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "i3micro vrg",
        "dev_vendor": "i3micro",
        "flag": "digest\\s*realm=\"i3micro\\s*vrg\",",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "jaws/\\d*\\.*\\d*",
        "dev_vendor": "juanvision",
        "flag": "jaws/\\d*\\.*\\d*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "omny\\s*ip\\s*\\s*",
        "dev_vendor": "tiandy",
        "flag": "<title>omny\\s*ip\\s*\\s*",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "TD-NVR-XXX",
        "dev_vendor": "tiandy",
        "flag": "Tiandy Co.,Ltd All Rights Reserved",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "axis.*camera",
        "dev_vendor": "axis",
        "flag": "axis.*camera",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "axis",
        "flag": "<title>AXIS \\w*\\d+\\w*-*\\w* Network Camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "\\s*\\s*\\s*\\s*bullet\\s*camera",
        "dev_vendor": "zavio",
        "flag": "basic\\s*realm=\"\\s*bullet\\s*camera\"",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "geovision",
        "flag": "Server: GeoVision IPCam",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "geovision",
        "flag": "<TITLE>GeoVision Inc",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "y-cam",
        "flag": "Server: Y-cam Cube",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "zavio",
        "favicon": {
            "hash": 623744943
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "intellinet",
        "favicon": {
            "hash": 405527018
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "abelcam",
        "favicon": {
            "hash": 11685462
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "favicon": {
            "hash": 999357577
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "xm030",
        "favicon": {
            "hash": 469671045
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "favicon": {
            "hash": 999357577
        },
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "xm030",
        "flag": "netsurveillance\\s*web",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "v\\d*r\\d*",
        "dev_vendor": "grandstream",
        "flag": "grandstream\\s*rtsp\\s*server\\s*v\\d*r\\d*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "\\d*\\.*\\d*\\.*\\d*\\.*\\d*",
        "dev_vendor": "helix",
        "flag": "helix.*\\d*\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "icu-\\d*\\w*",
        "dev_vendor": "icu",
        "flag": "icu-\\d*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "autodome\\s*\\d*",
        "dev_vendor": "bosch",
        "flag": "autodome\\s*\\d*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dinion hd 1080p",
        "dev_vendor": "dinion",
        "flag": "dinion\\s*hd\\s*1080p",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dynamic 7000",
        "dev_vendor": "dinion",
        "flag": "dinion\\s*ip\\s*dynamic\\s*7000",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "starlight 7000",
        "dev_vendor": "dinion",
        "flag": "dinion\\s*ip\\s*starlight\\s*7000",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "starlight 7000",
        "dev_vendor": "dinion",
        "flag": "dinion\\s*ip\\s*ultra\\s*8000",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "lorex",
        "flag": "href=\"/lorex_webplugin.exe",
        "is_dev": true
    },
    {
        "dev_class": "dvr",
        "dev_model": "",
        "dev_vendor": "axnet",
        "flag": "<title>dvr\\s*activex\\s*viewer",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "dss",
        "dev_vendor": "dahua",
        "flag": "<title>dss",
        "is_dev": true
    },
    {
        "dev_class": "vms",
        "dev_model": "phoenix",
        "dev_vendor": "kedacom",
        "flag": "<!--//@yu 20",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "yinhe",
        "flag": "<div id=\"psclientdiv\"></div>",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "ipc\\w*-*\\w*-*\\w*",
        "dev_vendor": "huawei",
        "flag": "digest\\s*realm=\"huawei\\s*ipc\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "avm\\w*",
        "dev_vendor": "avtech",
        "flag": "ip\\s*camera.*avm\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "ipc-(hf|pf|hdbw|pdbw|hum)\\w*",
        "dev_vendor": "dahua",
        "flag": "ipc-(hf|pf|hdbw|pdbw|hum)\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "iqeye\\w*",
        "dev_vendor": "iqinvision",
        "flag": "iqeye\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "snapshotcamera",
        "dev_vendor": "360 vision",
        "flag": "whsnapshotcamera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "360 vision",
        "flag": "360\\s*vision",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dm365",
        "dev_vendor": "360 vision",
        "flag": "dm365ipnc",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "av787",
        "dev_vendor": "avtech",
        "flag": "av-tech\\s*av787",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "ndc-\\w*",
        "dev_vendor": "bosch",
        "flag": "ndc-\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dcs-\\w*-*\\w*",
        "dev_vendor": "d-link",
        "flag": "dcs-\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "axis-\\w*-*\\w*",
        "dev_vendor": "axis",
        "flag": "axis-\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "avigilon-\\w*-*\\w*",
        "dev_vendor": "avigilon",
        "flag": "avigilon-\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "avigilon",
        "flag": "avigilononvif",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "camera",
        "flag": "basic\\s*realm=camera\\s*name\\s*:.*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "dsh-\\w*_\\w*",
        "dev_vendor": "d-link",
        "flag": "digest\\s*realm=\"dsh-\\w*_\\w*\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "networkcamera",
        "flag": "basic\\s*realm=\"networkcamera\\s*\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "wireless.*camera",
        "dev_vendor": "tp-link",
        "flag": "wireless.*camera",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "tv-ip\\w*_*\\w*",
        "dev_vendor": "trendnet",
        "flag": "tv-ip\\w*_*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "iq\\s*",
        "dev_vendor": "iqinvision",
        "flag": "iq\\s*\\s*\\s*\\s*live\\s*images",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "brickcom\\s*\\w*-*\\w*-*\\w*",
        "dev_vendor": "brickcom",
        "flag": "basic\\s*realm=\"brickcom\\s*\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "(er|am|icg)\\d+\\w*",
        "dev_vendor": "h3c",
        "flag": "<title>(er|am|icg)\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "h3c",
        "flag": "h3c-miniware-webs",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "linksys\\s*\\w{1,6}-*([0-9]{1,4})*-*\\w*-*\\w*",
        "dev_vendor": "linksys",
        "flag": "basic\\s*realm=\"linksys\\s*\\w{1,6}-*([0-9]{1,4})*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "netgear\\s*\\w+-*\\d*\\w*\\d*",
        "dev_vendor": "netgear",
        "flag": "basic\\s*realm=\"netgear\\s*\\w+-*\\d*\\w*\\d*",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "netcore\\s*\\w+-*\\d*\\w*\\d*",
        "dev_vendor": "netcore",
        "flag": "basic\\s*realm=\"netcore\\s*\\w+-*\\d*\\w*\\d*",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "300mbps.*\\w+-\\w+",
        "dev_vendor": "tp-link",
        "flag": "300mbps.*\\w+-\\w+",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "qiaoan",
        "flag": "GoAhead-Webs/2.5.0.*apcam/adm",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "dsl-\\w+_*\\w*",
        "dev_vendor": "d-link",
        "flag": "basic\\s*realm=\"dsl-\\w+_*\\w*",
        "is_dev": true
    },
     {
        "dev_class": "router",
        "dev_model": "WR\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "youhuatech",
        "flag": "Basic realm=\"WR\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "bipac\\s*\\w+-*\\w+",
        "dev_vendor": "bipac",
        "flag": "basic\\s*realm=\"bipac\\s*\\w+-*\\w+",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "d-link",
        "flag": "<title>d-link.*wireless\\s*router",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "\\w+-\\w*-*\\w*",
        "dev_vendor": "ruckus",
        "flag": "color:#ff6300.*>.*</h2>",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "ruckus",
        "flag": "ruckus wireless",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "huawei",
        "flag": "copyright\\s*&copy;\\s*huawei\\s*technologies",
        "is_dev": true
    },
    {
        "dev_class": "wimax cpe",
        "dev_model": "",
        "dev_vendor": "zyxel",
        "flag": "wimax\\s*cpe\\s*configuration",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "jcg",
        "flag": "var\\s\\w*url\\s*=\\s*\"http://www.jcgcn.com",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "mikrotik",
        "flag": "<img src=\"mikrotik_logo.png\"",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "mikrotik \\d+\\.*\\d*\\.*\\d*",
        "dev_vendor": "mikrotik",
        "flag": "ftp\\s*server\\s*\\(mikrotik \\d+\\.*\\d*\\.*\\d*\\) ",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "mikrotik",
        "flag": "mikrotik\\s*httpproxy",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "ruijie",
        "flag": "support\\.ruijie\\.com\\.cn",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "huawei b683",
        "dev_vendor": "huawei",
        "flag": "<title>huawei\\s*b683",
        "is_dev": true
    },
    {
        "dev_class": "wimax cpe",
        "dev_model": "\\w+\\d+\\w*",
        "dev_vendor": "zyxel",
        "flag": "textarea_content_word>welcome\\s*to\\s*\\w+\\d+\\w*\\s*configuration\\s*interface.",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "\\w*\\d+\\s*",
        "dev_vendor": "zte",
        "flag": "welcome\\s*to\\s*.*\\szte corporation",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "\\w*\\d+\\s*",
        "dev_vendor": "cisco",
        "flag": "cisco.*\\s*\\w*\\d+\\s*\\s*software",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "cisco",
        "flag": "level_\\d+.*access",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "cisco",
        "flag": "cisco\\s*router",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "cisco",
        "flag": "cisco-ios",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "\\w*\\d+\\w*\\-*\\w*",
        "dev_vendor": "juniper",
        "flag": "juniper.*\\s*\\w*\\d+\\w*\\-*\\w*s*.*router",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "juniper",
        "flag": "juniper.*router",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "(\\w-)*\\w*\\d+\\w*",
        "dev_vendor": "zyxel",
        "flag": "<title>.*zyxel\\s*(\\w-)*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "apd",
        "favicon": {
            "hash": 347462278
        },
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "ruckus",
        "favicon": {
            "hash": -2069844696
        },
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "mikrotik",
        "favicon": {
            "hash": 1924358485
        },
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "ruijie",
        "favicon": {
            "hash": 772273815
        },
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "(\\w-)*\\w*\\d+\\w*",
        "dev_vendor": "linksys",
        "flag": "basic\\s*realm=\"linksys\\s*(\\w-)*\\w*\\d+\\w*\"",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "(es|gs|mgs|mes)-\\w*\\d+\\w*",
        "dev_vendor": "zyxel",
        "flag": "basic\\s*realm=\"(es|gs|mgs)-\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "lanswitch\\s*-\\s*v\\d+r\\d+",
        "dev_vendor": "huawei",
        "flag": "lanswitch\\s*-\\s*v\\d+r\\d+\\s*httpserver",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "j02-2-5u-s7703",
        "dev_vendor": "huawei",
        "flag": "hk-tko-j02-2-5u-s7703-zhanqun",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "hp\\s*\\w*\\d+\\w*",
        "dev_vendor": "hp",
        "flag": "basic\\s*realm=\"hp\\s*\\w*\\d+\\w*\"",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "",
        "dev_vendor": "hp",
        "flag": "HP Comware switch",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "3com\\s*switch.*port",
        "dev_vendor": "3com switch",
        "flag": "3com\\s*switch.*port",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "allied\\s*telesis\\s*\\w*-\\w*\\d+\\w*",
        "dev_vendor": "allied telesis",
        "flag": "\\s*realm=\"allied\\s*telesis\\s*\\w*-\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "(des|dcrs|def|dgs|dhs)(\\s|-)\\w*\\d+\\w*(\\s|-)*\\w*(\\s|-)*\\w*",
        "dev_vendor": "d-link",
        "flag": "(des|dcrs|def|dgs|dhs)(\\s|-)\\w*\\d+\\w*(\\s|-|\\+)*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "ecs\\d+(\\s|-)\\w*",
        "dev_vendor": "edge-core",
        "flag": "ecs\\d+(\\s|-)\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "engenius\\s*\\w*\\d+\\w*",
        "dev_vendor": "engenius",
        "flag": "engenius\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "\\w*\\s*\\w*\\d+",
        "dev_vendor": "enterasys",
        "flag": "enterasys.*\\s*\\w*\\s*\\w*\\d+\\s*platinum",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "\\w*\\s*\\w*\\d+(\\s|-)*\\w*\\d+(\\s|-)*\\w*",
        "dev_vendor": "brocade",
        "flag": "brocade.*inc.*\\s*\\w*\\s*\\w*\\d+(\\s|-)*\\w*\\d+(\\s|-)*\\w*,",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "\\w*-\\w*\\d+\\w*-\\w*",
        "dev_vendor": "cisco",
        "flag": "cisco.*\\s*\\w*-\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "ethernet.*switch\\s*\\w*\\d+\\w*",
        "dev_vendor": "compufox",
        "flag": "ethernet.*switch\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "extremexos\\s*\\(.*\\)",
        "dev_vendor": "extreme xos",
        "flag": "extremexos\\s*\\(.*\\)",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "\\w*\\s*\\w*\\d+(\\s|-)*\\w*\\d+(\\s|-)*\\w*",
        "dev_vendor": "foundry",
        "flag": "foundry.*inc.*\\s*\\w*\\s*\\w*\\d+(\\s|-)*\\w*\\d+(\\s|-)*\\w*,",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "(gs|gsm)\\d+\\w*-*\\w*",
        "dev_vendor": "zyxel",
        "flag": "(gs|gsm)\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "adsl",
        "dev_model": "VES-\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "zyxel",
        "flag": "Basic realm=\"VES-\\w*\\d+\\w*-*\\w* a",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "h3c\\s*\\s*\\s*\\w*\\d+\\w*-*\\w*-*\\w*",
        "dev_vendor": "h3c",
        "flag": "h3c\\s*\\s*\\s*\\w*\\d+\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "hp\\s+\\w*\\d+\\w*-*\\w*-*\\w*",
        "dev_vendor": "hp",
        "flag": "(hp|hpe)\\s+\\w*\\d+\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "quidway\\s*\\w*\\d+w*-*\\w*",
        "dev_vendor": "huawei",
        "flag": "hua\\s*wei.*\\s*quidway\\s*\\w*\\d+w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "\\w*\\d+\\s*-*\\w*",
        "dev_vendor": "juniper",
        "flag": "<div\\s*class=\"jweb-title\\s*uppercase\"> .*\\w*\\d+\\s*-*\\w*</div>",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "marconi\\s*\\w+\\d*-*\\w*",
        "dev_vendor": "marconi",
        "flag": "marconi\\s*\\w+\\d*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "msc-\\w*\\d+\\w*",
        "dev_vendor": "msc",
        "flag": "msc-\\w*\\d+\\w*.*hardware",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "mypower\\s*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "mypower",
        "flag": "mypower\\s*\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "ruijie",
        "flag": "ruijie.*switch\\(\\w*\\d+\\w*-*\\w*\\)",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "\\w+\\d+\\w*-*\\w*",
        "dev_vendor": "zte",
        "flag": "\\w+\\d+\\w*-*\\w*\\s*switch\\s*of\\s*zte",
        "is_dev": true
    },
    {
        "dev_class": "switch",
        "dev_model": "\\w*\\d+\\w*-*\\w*-*\\w*",
        "dev_vendor": "cisco",
        "flag": "switch\\s*\\w*\\d+\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "net-i",
        "flag": "digest\\s*realm=\"net-i\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "interlogix",
        "flag": "digest\\s*realm=\"interlogix\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "interlogix",
        "flag": "digest\\s*realm=\"operator\"",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "lilin",
        "flag": "digest\\s*realm=\"nvr_rtsp\"",
        "is_dev": true
    },
    {
        "dev_class": "raop",
        "dev_model": "",
        "dev_vendor": "apple airtunes",
        "flag": "digest\\s*realm=\"raop\"",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tigeryin",
        "flag": "digest\\s*realm=\"tigeryin\"",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "sonicwall",
        "dev_vendor": "dell",
        "flag": "<title>\\s*\\s*sonicwall",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "sonicwall",
        "dev_vendor": "dell",
        "flag": "server:\\s*sonicwall",
        "is_dev": true
    },
    {
        "dev_class": "vpn",
        "dev_model": "sonicwall",
        "dev_vendor": "dell",
        "flag": "server:\\s*sonicwall.*ssl.*vpn",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "\\w+\\s*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "huawei",
        "flag": "\\&\\w+\\s*\\w*\\d+\\w*-*\\w*\\&langfrombrows=\\w*-\\w*&copyright=",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "\\w+\\s*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "huawei",
        "flag": "\\&\\w+\\s*\\w*\\d+\\w*-*\\w*\\&langfrombrows=\\w*-\\w*&copyright=",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "",
        "dev_vendor": "colasoft",
        "flag": "(/uportal/framework/default.html|url=/sign\\.in\\.cola)",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "topsec_security_appliance_system",
        "dev_vendor": "topsec",
        "flag": "<title>天融信安全管理系统</title>",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "topsec_firewall",
        "dev_vendor": "topsec",
        "flag": "<title>topsec\\s*tos\\s*web\\s*user\\s*interface",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "",
        "dev_vendor": "topsec",
        "flag": "server:\\s*topsec",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "\\w*-\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "paloalto",
        "flag": "<h2>\\w*-\\w*\\d+\\w*-*\\w*</h2>\r\n<p>access\\s*to\\s*the\\s*web\\s*page\\s*you\\s*were",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "nsfocus",
        "flag": "<title>nsfocus",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "nsfocus",
        "flag": "server:\\s*nsfocus",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "secoway\\s*\\w*\\d*\\w*-*\\w*",
        "dev_vendor": "huawei",
        "flag": "\\&secoway\\s*\\w*\\d*\\w*-*\\w*\\&langfrombrows=\\w*-\\w*&copyright",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "Legendsec\\s*3600",
        "dev_vendor": "Legendsec",
        "flag": "Legendsec\\s*3600",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "secworld_next_generation_speed_firewall",
        "dev_vendor": "Legendsec",
        "flag": "网神下一代极速防火墙",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "secworld_next_generation_speed_firewall",
        "dev_vendor": "Legendsec",
        "flag": "secworld\\s*next\\s*generation\\s*speed\\s*firewall",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "fortinet",
        "flag": "<title>firewall\\s*notification",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "sangfor",
        "flag": "commonname:\\s*sangfor",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "sangfor_ngaf",
        "dev_vendor": "sangfor",
        "flag": "<title>sangfor\\s*\\|\\s*ngaf",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "\\w+\\s*\\d+\\.\\d+",
        "dev_vendor": "sangfor",
        "flag": "<title>sangfor\\s*\\|\\s*\\w+\\s*\\d+\\.\\d+",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "adaptive\\s*security\\s*appliance",
        "dev_vendor": "cisco",
        "flag": "adaptive\\s*security\\s*appliance\\s*http/1.(0|1)",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "secospace\\s*\\w*\\d*\\w*-*\\w*",
        "dev_vendor": "huawei",
        "flag": "\\&secospace\\s*\\w*\\d*\\w*-*\\w*\\&langfrombrows=\\w*-\\w*&copyright",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "eudemon",
        "dev_vendor": "huawei",
        "flag": "eudemon\\s*server\\s*1.0",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "venusense\\s*(fw|usg)",
        "dev_vendor": "adca",
        "flag": "venusense\\s*(fw|usg)",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "zscaler",
        "flag": "location:\\s*https://gateway.zscaler.net:443/",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "netent",
        "flag": "网康下一代防火墙",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "",
        "dev_vendor": "topsec",
        "flag": "/js/report/horizontalreportpanel.js",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "network_traffic_audit_system",
        "dev_vendor": "topsec",
        "flag": "onclick=\"dlg_download",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "topflow",
        "dev_vendor": "topsec",
        "flag": "天融信topflow",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "ikuai",
        "flag": "/resources/images/land_prompt_ico01.gif",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "mixr",
        "dev_vendor": "xiaomi",
        "flag": "tx-server:\\s*mixr",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "nsfocus_nf",
        "dev_vendor": "nsfocus",
        "flag": "nsfocus\\s*nf",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "pc_defense",
        "dev_vendor": "360",
        "flag": "x-safe-firewal",
        "is_dev": true
    },
    {
        "dev_class": "application_scanning_system",
        "dev_model": "rayengine",
        "dev_vendor": "webray",
        "flag": "rayengine",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "weidun",
        "flag": "firewall:\\s*www.weidun.com.cn",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "anhui",
        "flag": "protected-by:\\s*anhui\\s*web\\s*firewall",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "dnp",
        "flag": "powered\\s*by\\s*dnp\\s*firewall",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "dnp",
        "flag": "dnp_firewall_redirect",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "depends": ["microsoft:windows"],
        "dev_model": "winroute\\s*firewall",
        "dev_vendor": "kerio",
        "flag": "kerio\\s*winroute\\s*firewall",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "paloalto_firewall",
        "flag": "access\\s*to\\s*the\\s*web\\s*page\\s*you\\s*were\\s*trying\\s*to\\s*visit\\s*has\\s*been\\s*blocked\\s*in\\s*accordance\\s*with\\s*company\\s*policy",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "",
        "dev_vendor": "barracuda",
        "flag": "http://www.barracudanetworks.com?a=bsf_product\"\\s*class=\"transbutton\"",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "binarysec-via",
        "dev_vendor": "binarysec",
        "flag": "x-binarysec-via",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "mail_information_security_gateway",
        "dev_vendor": "spammark",
        "flag": "spammark邮件信息安全网关",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "bigip",
        "dev_vendor": "f5",
        "flag": "server:\\s*bigip",
        "is_dev": true
    },
    {
        "dev_class": "vpn",
        "dev_model": "",
        "dev_vendor": "juniper",
        "flag": "welcome.cgi?p=logo",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "networks_application_acceleration_platform",
        "dev_vendor": "juniper",
        "flag": "rl-sticky-key",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "networks_application_acceleration_platform",
        "dev_vendor": "juniper",
        "flag": "juniper\\s*networks\\s*application\\s*acceleration\\s*platform",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "juniper-netscreen-secure-access",
        "dev_vendor": "juniper",
        "flag": "/dana-na/auth/welcome.cgi",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "\\w*\\d+\\w*-*\\w*-*\\w*",
        "dev_vendor": "mypower",
        "flag": "mypower\\s*\\w*\\d+\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "",
        "dev_vendor": "hillstone",
        "flag": "organization:\\s*hillstone\\s*networks",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "",
        "dev_vendor": "hillstone",
        "flag": "<title>.*hillstone\\s*networks",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "",
        "dev_vendor": "checkpoint",
        "flag": "organization:\\s*check\\s*point",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "check\\s*point\\s*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "checkpoint",
        "flag": "<title>check\\s*point\\s*\\w*\\d+\\w*-*\\w*\\s*appliance",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "watchfire",
        "dev_vendor": "ibm",
        "flag": "watchfiresessionid",
        "is_dev": true
    },
    {
        "dev_class": "ips",
        "dev_model": "",
        "dev_vendor": "sniper",
        "flag": "<title>sniper\\s*login",
        "is_dev": true
    },
    {
        "dev_class": "ips",
        "dev_model": "",
        "dev_vendor": "sniper",
        "flag": "server:\\s*sniper-*\\w*/\\d+.0",
        "is_dev": true
    },
    {
        "dev_class": "security_gateway",
        "dev_model": "messaging gateway",
        "dev_vendor": "symantec",
        "flag": "messaging gateway",
        "is_dev": true
    },
    {
        "dev_class": "endpoint_protection",
        "dev_model": "",
        "dev_vendor": "symantec",
        "flag": "<title>symantec\\s*endpoint\\s*protection\\s*manager",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "",
        "dev_vendor": "hp",
        "flag": "hp-chai\\w*/\\d+",
        "is_dev": true
    },
            {
        "dev_class": "printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\s*\\w*\\s*\\d+\\w*",
        "dev_vendor": "savin",
        "flag": "savin\\s*\\w+-*\\w*\\s*\\w*\\s*\\w*\\s*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "laserjet\\s*\\w*\\s*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "hp",
        "flag": "hp.*\\s*laserjet\\s*\\w*\\s*\\w*\\d+\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "",
        "dev_vendor": "canon",
        "flag": "canon\\s*http\\s*server",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "",
        "dev_vendor": "canon",
        "flag": "catwalk",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": ":\\s*(ir-adv(\\s*|&nbsp;)|lbp|mf)\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "canon",
        "flag": ":\\s*(ir-adv(\\s*|&nbsp;)|lbp)\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "mfc printer",
        "dev_model": "(mfc|nc)-\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "brother",
        "flag": "brother\\s*(mfc|nc)-\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "laser printer",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "dell",
        "flag": "dell\\s*\\w*\\d+\\w*-*\\w*.*\\s*laser",
        "is_dev": true
    },
    {
        "dev_class": "laser printer",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "dell",
        "flag": "dell\\s*laser.*\\w*\\d+\\w*-*\\w*.*\\s*",
        "is_dev": true
    },
    {
        "dev_class": "laser printer",
        "dev_model": "",
        "dev_vendor": "dell",
        "flag": "ews-nic\\d*/\\d+",
        "is_dev": true
    },
    {
        "dev_class": "laser printer",
        "dev_model": "dell\\s*\\w*\\d+\\w*-*\\w*\\s*\\w*\\s*\\w*",
        "dev_vendor": "dell",
        "flag": "dell\\s*\\w*\\d+\\w*-*\\w*\\s*\\w*\\s*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "laser printer",
        "dev_model": "dell\\s*\\w*\\s*\\w*\\s*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "dell",
        "flag": "dell\\s*\\w*\\s*\\w*\\s*\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "",
        "dev_vendor": "epson",
        "flag": "epson_linux\\s*upnp/1.0\\s*epson\\s*upnp\\s*sdk/1.0",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "",
        "dev_vendor": "epson",
        "flag": "epson-http/1.0",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "",
        "dev_vendor": "epson",
        "flag": "epson\\s*http\\s*server",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "officejet.*\\s*\\w*\\d+\\w*-*\\w*-*\\w*",
        "dev_vendor": "hp",
        "flag": "hp\\s*officejet.*\\s*\\w*\\d+\\w*-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "docuprint",
        "flag": "<td\\s*class=std_2>docuprint\\s*\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "mp\\s*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "ricoh",
        "flag": "ricoh\\s*mp\\s*\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "",
        "dev_vendor": "ricoh",
        "flag": "<title>.*web\\s*image\\s*monito",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "samsung\\s*\\w*-*\\w*\\d+\\w*\\s*series\\s*-*\\s*\\w*-*\\w*\\d*\\w*",
        "dev_vendor": "samsung",
        "flag": "(hp\\s*http\\s*server;\\s*){0,1}samsung\\s*\\w*-*\\w*\\d+\\w*\\s*series\\s*-*\\s*\\w*-*\\w*\\d*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "adc\\s*\\w*\\d+\\w*",
        "dev_vendor": "aurora",
        "flag": "aurora\\s*adc\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "security_appliance",
        "dev_model": "",
        "dev_vendor": "colasoft",
        "flag": "/uportal/framework/default.html|url=/sign\\.in\\.cola",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": ":\\s*(ir-adv(\\s*|&nbsp;)|lbp|mf)\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "canon",
        "flag": ":\\s*(ir-adv(\\s*|&nbsp;)|lbp)\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "trendnet",
        "flag": "trendnet.s*alls*rightss*reserved",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tp-link",
        "flag": "Digest realm=\"TP-LINK",
        "is_dev": true
    },
     {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tp-link",
        "flag": "intellinet",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "axis",
        "flag": "to use the axis web application, enable javascript.</span",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "vb-\\w*\\d+\\w*",
        "dev_vendor": "canon",
        "flag": "<span>vb-\\w*\\d+\\w*\\s*viewer",
        "is_dev": true
    },
    {
        "dev_class": "integration printer",
        "dev_model": "\\w+ \\w*\\d+\\w*",
        "dev_vendor": "ricoh-aficio",
        "flag": "ricoh aficio \\w+ \\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "oki",
        "flag": "<title>\\w*\\d+\\w*-*\\w*.*okilogo",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "fuji xerox",
        "flag": "fuji xerox \\w+-*\\w*\\s*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "\\w+-*\\w* \\w*\\d+\\w*",
        "dev_vendor": "aurora",
        "flag": "aurora \\w+-*\\w* \\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "integration printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "canon",
        "flag": "canon \\w+-*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "integration printer",
        "dev_model": "dell.*\\w*\\d+\\w*",
        "dev_vendor": "dell",
        "flag": "dell.*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "integration printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "epson",
        "flag": "epson \\w+-*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "integration printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "generic",
        "flag": "generic\\s\\w+-\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "lexmark",
        "flag": "lexmark\\s*\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "lenovo",
        "flag": "lenovo\\s*\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "integration printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "toshiba",
        "flag": "toshiba\\s*\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "samsung",
        "flag": "samsung\\s*\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "",
        "dev_vendor": "samsung",
        "flag": "<title>SAMSUNG TECHWIN NVR Web",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "SRN-\\d+\\w*-*\\w*",
        "dev_vendor": "samsung",
        "flag": "nvr.model_name=\"SRN-\\d+\\w*-*\\w*\"",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "konica",
        "flag": "konica\\s*\\w+-*\\w*\\s*\\w*\\s*\\w*\\s*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "kmb\\w*\\d+\\w*",
        "dev_vendor": "unkown",
        "flag": "kmb\\w*\\d+\\w* smtpd",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "ztc.*\\w+",
        "dev_vendor": "zebra",
        "flag": "ztc.*\\w+</h1>",
        "is_dev": true
    },
    {
        "dev_class": "printer",
        "dev_model": "zxp.{0,20}printer",
        "dev_vendor": "zebra",
        "flag": "zxp.{0,20}printer",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "\\w+-*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "sharp",
        "flag": "<title>top page - \\w+-*\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "integration printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\d+\\w*",
        "dev_vendor": "canon",
        "flag": "canon \\w+-*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "network printer",
        "dev_model": "\\w+-*\\w*\\s*\\w*\\s*\\w*\\s*\\d+\\w*",
        "dev_vendor": "lanier",
        "flag": "lanier\\s*\\w+-*\\w*\\s*\\w*\\s*\\w*\\s*\\d+\\w*.*network printer",
        "is_dev": true
    },
    {
        "dev_class": "fax",
        "dev_model": "network fax",
        "dev_vendor": "myfax",
        "flag": "<h1>language checking...",
        "is_dev": true
    },
    {
        "dev_class": "fax",
        "dev_model": "faxkeeper",
        "dev_vendor": "hanyuan",
        "flag": "welcome (tiandacopper|hanyuan|dowaytech) faxkeeper",
        "is_dev": true
    },
    {
        "dev_class": "fax",
        "dev_model": "ufax2",
        "dev_vendor": "aineton",
        "flag": "<title>ufax2",
        "is_dev": true
    },
    {
        "dev_class": "scanner",
        "dev_model": "ts\\d+",
        "dev_vendor": "canon",
        "flag": "canon ts\\d+",
        "is_dev": true
    },
    {
        "dev_class": "nas",
        "dev_model": "maxtronic\\s*\\w+-*\\w*\\s*\\w*\\s*\\w*\\s*\\d+\\w*",
        "dev_vendor": "raidweb",
        "flag": "maxtronic\\s*\\w+-*\\w*\\s*\\w*\\s*\\w*\\s*\\d+\\w*",
        "is_dev": true
    },
       {
        "dev_class": "nas",
        "dev_model": "",
        "dev_vendor": "qnap",
        "flag": "http server.*redirect.html",
        "is_dev": true
    },
    {
        "dev_class": "nas",
        "dev_model": "qnap.*nas",
        "dev_vendor": "qnap",
        "flag": "<title>qnap.*nas",
        "is_dev": true
    },
    {
        "dev_class": "nas",
        "dev_model": "\\w+.*synology.*diskstatio",
        "dev_vendor": "synology",
        "flag": "<title>\\w+.*synology.*diskstation",
        "is_dev": true
    },
    {
        "dev_class": "nas",
        "dev_model": "",
        "dev_vendor": "snap",
        "flag": "server: snap appliance",
        "is_dev": true
    },
    {
        "dev_class": "nas",
        "dev_model": "\\w+-*\\w* \\d+\\.\\d+\\.\\d+",
        "dev_vendor": "qnap",
        "flag": "linux \\w+-*\\w* \\d+\\.\\d+\\.\\d+",
        "is_dev": true
    },
    {
        "dev_class": "print server",
        "dev_model": "",
        "dev_vendor": "axis",
        "flag": "<title>network print server",
        "is_dev": true
    },
    {
        "dev_class": "print server",
        "dev_model": "d-link.*print\\s*server",
        "dev_vendor": "d-link",
        "flag": "d-link.*print\\s*server",
        "is_dev": true
    },
    {
        "dev_class": "print server",
        "dev_model": "axis_\\w*\\d+\\w*",
        "dev_vendor": "axis",
        "flag": "prodhelp\\?prod=axis_\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "print server",
        "dev_model": "axis.*print\\s*server",
        "dev_vendor": "axis",
        "flag": "axis.*print\\s*server",
        "is_dev": true
    },
    {
        "dev_class": "print server",
        "dev_model": "okilan \\w*\\d+\\w*-*\\w*",
        "dev_vendor": "oki",
        "flag": "oki okilan \\w*\\d+\\w*-*\\w*.*printserver",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "",
        "dev_vendor": "nbx",
        "flag": "<title>nbx netset",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "virata-emweb/r\\d+_\\d+_*d*",
        "dev_vendor": "nbx",
        "flag": "server: virata-emweb/r\\d+_\\d+_*d*",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "espace \\w*\\d+\\w*-*\\w*",
        "dev_vendor": "huawei",
        "flag": "<title>huawei espace \\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "ip phone \\w+-\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "cisco",
        "flag": "size=\"4\">cisco ip phone \\w+-\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "",
        "dev_vendor": "solvonet",
        "flag": "basic realm=\"solvonet\"",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "unified ip phone",
        "dev_vendor": "cisco",
        "flag": "size=\"4\">cisco unified ip phone",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "rfip",
        "dev_vendor": "easy-link",
        "flag": "realm=\"easy-link rfip",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "\\w*-\\w*\\d+\\w*",
        "dev_vendor": "ida",
        "flag": "ealm=\"ida \\w*-\\w*\\d+\\w* voip",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "",
        "dev_vendor": "ida",
        "flag": "server: allegro-software-rompager/4.01",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "\\w+-\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "enterprise",
        "flag": "realm=\"enterprise ip phone \\w+-\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "\\w* \\w+-*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "ericsson",
        "flag": "<title> ericsson ip telephone, \\w* \\w+-*\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "\\w+-*\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "mitel",
        "flag": "realm=\"mitel \\w+-*\\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "espace \\w*\\d+\\w*-*\\w*",
        "dev_vendor": "huawei",
        "flag": "tsp_http_server/\\d+\\.\\d+\\.\\d+ espace \\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "access control",
        "dev_model": "人脸实时报警系统|人脸检索管理系统",
        "dev_vendor": "hikvison",
        "flag": "人脸实时报警系统|人脸检索管理系统",
        "is_dev": true
    },
    {
        "dev_class": "access control",
        "dev_model": "",
        "dev_vendor": "zkteco",
        "flag": "server: zk web server",
        "is_dev": true
    },
    {
        "dev_class": "access control",
        "dev_model": "",
        "dev_vendor": "zhengpu",
        "flag": "realm= \"正普门禁系统\"",
        "is_dev": true
    },
    {
        "dev_class": "access control",
        "dev_model": "z(e|m)m\\d+\\w*",
        "dev_vendor": "zkteco",
        "flag": "welcome to linux \\(z(e|m)m\\d+\\w*\\) for mips",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "cisco telepresence \\w*\\s*\\w*\\s*\\w*\\d+",
        "dev_vendor": "cisco",
        "flag": "welcome to the cisco telepresence \\w*\\s*\\w*\\s*\\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "ASA \\w*\\d+\\w*-*\\w*",
        "dev_vendor": "cisco",
        "flag": "Basic realm=\"Cisco ASA \\w*\\d+\\w*-*\\w*\"",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "CSR\\d+\\w*-*\\w*",
        "dev_vendor": "cisco",
        "flag": "Cisco IOS Software [Denali], CSR\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "firewall",
        "dev_model": "CSR\\d+\\w*-*\\w*",
        "dev_vendor": "cisco",
        "flag": "Hostname: csr\\d+\\w*-*\\w*-cgnat",
        "is_dev": true
    },
    {
        "dev_class": "mcu",
        "dev_model": "",
        "dev_vendor": "huawei",
        "flag": "<title>huawei mcu",
        "is_dev": true
    },
    {
        "dev_class": "mcu",
        "dev_model": "",
        "dev_vendor": "zte",
        "flag": "<title>digital mic",
        "is_dev": true
    },
    {
        "dev_class": "mcu",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "codian",
        "flag": "codian mcu \\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "mcu",
        "dev_model": "mse mcu blade \\w*\\d+\\w*",
        "dev_vendor": "codian",
        "flag": "codian mse mcu blade \\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "mcu",
        "dev_model": "\\w*\\d+/\\w*\\d+ \\w*\\d+\\w*",
        "dev_vendor": "cisco",
        "flag": "mcu: cisco telepresence profile \\w*\\d+/\\w*\\d+ \\w*\\d+\\w*",
        "is_dev": true
    },
    {
        "dev_class": "mcu",
        "dev_model": "\\w*\\d+\\w*-*\\w*-*\\w*",
        "dev_vendor": "cisco",
        "flag": "mcu: tandberg \\w*\\d+\\w*-*\\w*-*\\w* portable",
        "is_dev": true
    },
    {
        "dev_class": "nlw",
        "dev_model": "",
        "dev_vendor": "kodi",
        "flag": "<title>kodi",
        "is_dev": true
    },
    {
        "dev_class": "nlw",
        "dev_model": "",
        "dev_vendor": "subsonic",
        "flag": "<title>subsonic",
        "is_dev": true
    },
    {
        "dev_class": "media server",
        "dev_model": "",
        "dev_vendor": "h3c",
        "flag": "h3c multimediaware software\\. copyright",
        "is_dev": true
    },
    {
        "dev_class": "media server",
        "dev_model": "mx-one",
        "dev_vendor": "mivoice",
        "flag": "<title>mivoice mx-one provisioning manager",
        "is_dev": true
    },
    {
        "dev_class": "dvs",
        "dev_model": "",
        "dev_vendor": "xin shi yun",
        "flag": "新视云编码机管理系统",
        "is_dev": true
    },
    {
        "dev_class": "nlw",
        "dev_model": "",
        "dev_vendor": "avcon",
        "flag": "<title>avcon-",
        "is_dev": true
    },
    {
        "dev_class": "media server",
        "dev_model": "",
        "dev_vendor": "logitech",
        "flag": "server: logitech media server",
        "is_dev": true
    },
      {
        "dev_class": "media server",
        "dev_model": "",
        "dev_vendor": "yaan",
        "flag": "server: YAAN SOAP Server",
        "is_dev": true
    },
    {
        "dev_class": "nvr",
        "dev_model": "NVR\\d+-*\\w*-*\\w*",
        "dev_vendor": "uniview",
        "flag": "<title>NVR\\d+-*\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "media server",
        "dev_model": "ISC\\d+-*\\w*-*\\w*",
        "dev_vendor": "uniview",
        "flag": "<title>ISC\\d+-*\\w*-*\\w*",
        "is_dev": true
    },
     {
        "dev_class": "dvr",
        "dev_model": "DVR\\d+-*\\w*-*\\w*",
        "dev_vendor": "uniview",
        "flag": "<title>DVR\\d+-*\\w*-*\\w*",
        "is_dev": true
    },
     {
        "dev_class": "media server",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "uniview",
        "flag": "uniview multimedia.*\\w*\\d+\\w*-*\\w*.*copyright",
        "is_dev": true
    },

    {
        "dev_class": "media server",
        "dev_model": "",
        "dev_vendor": "broadworks",
        "flag": "broadworks media server",
        "is_dev": true
    },
    {
        "dev_class": "tvrs",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "flag": "<title>电视墙",
        "is_dev": true
    },
    {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "flag": "/doc/page/login.asp?_",
        "is_dev": true
    },
      {
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "flag": "Hikvision Digital Technology Co., Ltd. All Rights Reserved.",
        "is_dev": true
    },
    {
        "dev_class": "voip",
        "dev_model": "\\w*\\d+\\w*-*\\w*",
        "dev_vendor": "samsung",
        "flag": "size=\"3\">Internet\\s*SIP Phone \\w*\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {    
        "dev_class": "router",
        "dev_model": "Mikrotik",
        "dev_vendor": "Mikrotik",
        "flag": "Mikrotik",
        "is_dev": true 
    },
    {    
        "dev_class": "router",
        "dev_model": "DD-WRT.*",
        "dev_vendor": "DD-WRT",
        "flag": "DD-WRT.*",
        "is_dev": true 
    },
    {    
        "dev_class": "firewall",
        "dev_model": "venustech",
        "dev_vendor": "venustech",
        "flag": "venustech*",
        "is_dev": true
    },
      {
        "dev_class": "router",
        "dev_model": "TP-LINK\\s*\\w*\\s*\\s*\\w*\\s*\\s*\\w*\\s*\\s*\\w*\\s*\\s*\\w*\\s*\\w+\\d+\\w*-*\\w*",
        "dev_vendor": "tp-link",
        "flag": "realm=\"TP-LINK\\s*\\w*\\s*\\s*\\w*\\s*\\s*\\w*\\s*\\s*\\w*\\s*\\s*\\w*\\s*\\w+\\d+\\w*-*\\w*",
        "is_dev": true
    },
    {
        "dev_class": "router",
        "dev_model": "\\w+\\d+\\w*-*\\w*",
        "dev_vendor": "tenda",
        "flag": "ys_target = \"\\w+\\d+\\w*-*\\w*\".*text\\('TENDA",
        "is_dev": true
    },
     {
        "dev_class": "router",
        "dev_model": "[a-z,A-Z]+\\d+\\w*-*\\w*",
        "dev_vendor": "hiwifi",
        "flag": "sys_board\">[a-z,A-Z]+\\d+\\w*\\s*-*\\s*\\w*\\.*\\w*\\.*\\w*\\.*\\w*\\.*\\w*\\.*\\w*</span>.*http://www.hiwifi.com/app",
        "is_dev": true
    },
      {
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "wayos",
        "flag": "www\\.wayos\\.cn",
        "is_dev": true
    },
    {
        "app_class": "bbs",
        "app_version": "",
        "app_name": "discuz:discuzx",
        "flag": "<meta name=\"generator\" content=\"Discuz!",
        "is_dev": false
    },
    {
        "app_class": "bbs",
        "app_version": "",
        "app_name": "discuz:discuzx",
        "flag": "<script src=\".*?logging\\.js",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "phpwind:phpwind",
        "flag": "<meta name=\"generator\" content=\"(phpwind|PHPWind)",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "phpwind:phpwind",
        "flag": "Powered by <a href=\"http://www.phpwind.net/\" target=\"_blank\" CMS:rel=\"nofollow\">phpwind",
        "is_dev": false
    },
     {
        "app_class": "cache-plugin",
        "app_version": "",
        "implies": {"wordpress:wordpress": "plugin"},
        "app_name": "wordpress:wp-super-cache",
        "flag": "WP-Super-Cache",
        "is_dev": false
    },
     {
        "app_class": "blog",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "wordpress:wordpress",
        "flag": "content=\"WordPress\\s*\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "blog",
        "app_version": "",
        "app_name": "wordpress:wordpress",
        "flag": "<meta name=\"generator\" content=\"WordPress",
        "is_dev": false
    },
     {
        "app_class": "blog",
        "app_version": "",
        "app_name": "wordpress:wordpress",
        "flag": "\"[^\"]+/wp-content/[^\"]+\"",
        "is_dev": false
    },
     {
        "app_class": "blog",
        "app_version": "",
        "app_name": "rainbowsoft:z-blog",
        "flag": "<link rel=\"stylesheet\" rev=\"stylesheet\" href=\".*zb_users",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "phpcms:phpcms",
        "flag": "<link href=\\\"templates/default/skins/default/phpcms.css\\\"",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "phpcms:phpcms",
        "flag": "Powered by (PHPCMS|Phpcms)",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "qibosoft:qibosoft",
        "flag": "Powered by <a href=\"http://www.qibosoft.com\" target=\"_blank\">qibosoft",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "yidacms:yidacms",
        "flag": "Powered by YidaCms",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "yidacms:yidacms",
        "flag": "Powered by <a href=\"http://yidacms.com\" target=\"_blank\">YidaCms",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "cmseasy:cmseasy",
        "flag": "<meta name=\"author\" content=\"CmsEasy Team\" />",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "cmseasy:cmseasy",
        "flag": "Powered by <a href=\"http://www.cmseasy.cn\" title=\"CmsEasy.*?\" target=\"_blank\">CMS:CmsEasy</a>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "shopex:ecshop",
        "flag": "Powered by ECShop",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "ljcms:ljcms",
        "flag": "Powered by <a href=\"http://www.8cms.com/\" target=\"_blank\">LJCMS",
        "is_dev": false
    },
     {
        "app_class": "cms",
         "depends": ["asp","microsoft:sql_server|access"],
        "app_version": "d+\\./d*\\./d*",
        "app_name": "ljcms:ljcms",
        "flag": "<meta name=\"keywords\" content=\"LJCMS,LJcms v/d+\\./d*\\./d*\"/>",
        "is_dev": false
    },
     {
        "app_class": "cms",
         "depends": ["asp","microsoft:sql_server|access"],
        "app_version": "",
        "app_name": "kesion:kesioncms",
        "flag": "src=\"/ks_inc",
        "is_dev": false
    },
     {
        "app_class": "cms",
         "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "metinfo:metinfo",
        "flag": "Powered by <a href=\"http://www.MetInfo.cn\"",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "shopex:shopex",
        "flag": "if (Shop.set != undefined&&Shop.set.refer_timeout)",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "shopex:shopex",
        "flag": "app/b2c/statics/js_mini/shoptools_min.js",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "maccms:maccms",
        "flag": "Copyright .*? maccms\\.com Inc",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "shopnc:shopnc",
        "flag": "index.php?act=show_joinin&op=index",
        "is_dev": false
    },
     {
        "app_class": "cms",
         "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "shopnc:shopnc",
        "flag": "Powered by <a href=\"http://www.shopnc.net\" target=\"_blank\" style=\"color:#FF6600\">ShopNC",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "dedecms:dedecms",
        "flag": "Powered by <a target=\"_blank\" href=\"http://www.dedecms.com/\">DedeCMS</a>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "dedecms:dedecms",
        "flag": "Powered by <a href=\"http://www.dedecms.com/\">DedeCMS",
        "is_dev": false
    },
     {
        "app_class": "bbs",
        "app_version": "",
        "app_name": "startbbs:startbbs",
        "flag": "class=\"startbbs",
        "is_dev": false
    },
     {
        "app_class": "bbs",
        "app_version": "",
        "app_name": "startbbs:startbbs",
        "flag": "Powered by <a href=\"http://www.startbbs.com\"",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["php:php"],
        "app_version": "",
        "app_name": "drupal:drupal",
        "flag": "X-Drupal-Cache",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "drupal:drupal",
        "flag": "X-Drupal-Dynamic-Cache",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "drupal:drupal",
        "flag": "X-Generator\\s*:\\s*Drupal",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "waikucms:waikucms",
        "flag": "Powered by <b>WaiKuCms",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "yongyou:yongyou_nc",
        "flag": "src=logo/images/ufida_nc.png",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "cacti:cacti",
        "flag": "<title>.*?Cacti.*?</title>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "cacti:cacti",
        "flag": "Set-Cookie\\s*:\\s*Cacti=",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "zabbix:zabbix",
        "flag": "Set-Cookie\\s*:\\s*zbx_sessionid",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "destoon:destoon_b2b",
        "flag": "Powered by DESTOON",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "destoon:destoon_b2b",
        "flag": "DESTOON B2B SYSTEM",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "",
        "app_name": "winmail_project:winmail",
        "flag": "Winmail Mail Server",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "",
        "app_name": "coremail_xt_project:coremail_xt",
        "flag": "Coremail[^>]+<\\/title>",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "",
        "app_name": "winmail_project:winmail",
        "flag": "Set-Cookie\\s*:\\s*magicwinmail",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "",
        "app_name": "winmail_project:winmail",
        "flag": "Powered by Winmail Server",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "",
        "app_name": "turbomail:turbomail",
        "flag": "Powered by TurboMail",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "\\d{4}-\\d{4}",
        "app_name": "idccenter:webmail",
        "flag": "\\d{4}-\\d{4}\\s*webmail.idccenter.net",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "",
        "app_name": "microsoft:outlook",
        "flag": "X-OWA-Version",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "",
        "app_name": "microsoft:outlook",
        "flag": "Outlook Web (Access|App)\\s*(?=<\\/title>)",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "anymacro:anymacro_mail_system",
        "flag": "sec.anymacro.com",
        "is_dev": false
    },
     {
        "app_class": "Mailserver",
        "app_version": "",
        "app_name": "extmail:extmail",
        "flag": "powered by.*?Extmail",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "ibm:lotus_protector_for_mail_security",
        "flag": "IBM Lotus iNotes[^>]+(?=<\\/title>)",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "squirrelmail:squirrelmail",
        "flag": "SquirrelMail Project Team",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "squirrelmail:squirrelmail",
        "flag": "Set-Cookie\\s*:\\s*SQMSESSID",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "eqmail:eqmail",
        "flag": "Powered by EQMail",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "tmailer:tmailer",
        "flag": "TMailer Collaboration Suite Web Client",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "synacor:zimbra_collaboration_suite",
        "flag": "Set-Cookie\\s*:\\s*ZM_TEST",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "synacor:zimbra_collaboration_suite",
        "flag": "zimbra[^>]+(?=<\\/title>)",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "synacor:zimbra_collaboration_suite",
        "flag": "Zimbra,?\\s*Inc. All rights reserved",
        "is_dev": false
    },
     {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "bxemail:bxemail",
        "flag": "abc@bxemail.com",
        "is_dev": false
    },
     {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "horde:groupware",
        "flag": "<title>[^>]+?Horde",
        "is_dev": false
    },
     {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "horde:groupware",
        "flag": "/themes/graphics/horde-power1.png",
        "is_dev": false
    },
     {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "atmail:atmail",
        "flag": "powered by Atmail",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "phpmyadmin:phpmyadmin",
        "flag": "href=\"phpmyadmin.css.php",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "phpmyadmin:phpmyadmin",
        "flag": "<title>phpMyAdmin</title>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "phpstudy:phpstudy",
        "flag": "<title>phpStudy.*?</title>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql","apache:apache","microsoft:windows"],
        "app_version": "",
        "app_name": "wamp:wamp",
        "flag": "<title>WAMPSERVER Homepage</title>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["apache:apache","phpmyadmin:phpmyadmin"],
        "app_version": "",
        "app_name": "appserv_open_project:appserv",
        "flag": "<title>AppServ Open Project.*?</title>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "upupw:upupw",
        "flag": "<meta name=\\\"author\\\" content=\\\"UPUPW\\\" />",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["linux:linux","php:php","nginx:nginx","mysql:mysql"],
        "app_version": "",
        "app_name": "lnmp:lnmp",
        "flag": "<title>.*?LNMP.*?</title>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["linux:linux","php:php","nginx:nginx","mysql:mysql","apache:apache"],
        "app_version": "",
        "app_name": "lanmp:lanmp",
        "flag": "<title>.*?lanmp.*?</title>",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["jsp:jsp","apache:tomcat","j2ee:j2ee","j2ee:servlet"],
        "app_version": "",
        "app_name": "jboss:jboss",
        "flag": "X-Powered-By\\s*:\\s*JBoss",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "jboss:jboss",
        "flag": "JBoss, Home of Professional Open Source",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "bea:weblogic_integration",
        "flag": "<META NAME=\\\"GENERATOR\\\" CONTENT=\\\"WebLogic Server\\\">",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "depends": ["java:java"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "oracle:glassfish_server",
        "flag": "Server: GlassFish Server.*\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "depends": ["java:java"],
         "app_version": "",
        "app_name": "jenkins:jenkins",
        "flag": "X-Jenkins\\s*:",
        "is_dev": false
    },
     {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:hadoop",
        "flag": "<title>Hadoop Administration</title>",
        "is_dev": false
    },
     {
        "app_class": "bigdata",
         "depends": ["java:java"],
        "app_version": "",
        "app_name": "elasticsearch:elasticsearch",
        "flag": "\"cluster_name\" : \"elasticsearch\"",
        "is_dev": false
    },
     {
        "app_class": "server",
        "app_version": "",
        "app_name": "apache:tomcat",
        "flag": "<title>Apache Tomcat/.*?</title>",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "",
        "app_name": "nginx:nginx",
        "flag": "Server: nginx",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "\\d+\\.\\d+\\.\\d+",
        "app_name": "nginx:nginx",
        "flag": "nginx/\\d+\\.\\d+\\.\\d+",
        "is_dev": false
    },
     {
        "app_class": "server",
        "app_version": "",
        "app_name": "rejetto:http_file_server",
        "flag": "Set-Cookie\\s*:\\s*HFS_SID",
        "is_dev": false
    },
     {
        "app_class": "server",
        "app_version": "",
        "app_name": "http:http_basic",
        "flag": "WWW-Authenticate",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "topsec:topsec-waf",
        "flag": "<META NAME=\"Copyright\" CONTENT=\"Topsec Network Security Technology Co.,Ltd\"/>",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "topsec:topsec-waf",
        "flag": "<META NAME=\"DESCRIPTION\" CONTENT=\"Topsec web UI\"/>",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "360:360wzb",
        "flag": "X-Powered-By-360wzb",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "baidu:anquanbao",
        "flag": "X-Powered-By-Anquanbao",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "depends": ["nginx:nginx"],
        "app_version": "",
        "app_name": "yunjiasu:yunjiasu",
        "flag": "Server\\s*:\\s*yunjiasu-nginx",
        "is_dev": false
    },
    
      {
        "app_class": "waf",
        "app_version": "",
        "app_name": "yunsuo:yunsuo",
        "flag": "yunsuo_session_verify",
        "is_dev": false
    },
    {
        "app_class": "waf",
        "app_version": "",
        "app_name": "leadsec:leadsec",
        "flag": "<title>网御waf",
        "is_dev": false
    },
    {
        "app_class": "waf",
        "app_version": "",
        "app_name": "nsfocus:nsfocus",
        "flag": "/images/logo/nsfocus.png",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "f5:big-ip",
        "flag": "BIGipServer",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "binarysec:binarysec",
        "flag": "x-binarysec-cache",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "binarysec:binarysec",
        "flag": "x-binarysec-via",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "\\d+\\.\\d+",
        "app_name": "safedog:safedog",
        "flag": "waf/\\d+\\.\\d+",
        "is_dev": false
    },
    {
        "app_class": "waf",
        "app_version": "waf\\s*\\d+\\.\\d+",
        "app_name": "spring:websecurity",
        "flag": "websecurity:\\s*waf\\s*\\d+\\.\\d+",
        "is_dev": false
    },
    {
        "app_class": "waf",
        "app_version": "",
        "app_name": "safe3:safe3",
        "flag": "safe3\\s*web\\s*firewall",
        "is_dev": false
    },
    {
        "app_class": "waf",
        "app_version": "\\d+\\.*\\d*\\.\\d",
        "app_name": "anzu:anzuwaf",
        "flag": "x-powered-by:\\s*anzuwaf/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "blockdos:blockdos",
        "flag": "Server\\s*:\\s*BlockDos\\.net",
        "is_dev": false
    },
     {
        "app_class": "waf",
         "depends": ["nginx:nginx"],
        "app_version": "",
        "app_name": "nginx:cloudflare",
        "flag": "Server\\s*:\\s*cloudflare-nginx",
        "is_dev": false
    },
     {
        "app_class": "cdn",
        "app_version": "",
        "app_name": "aws:cloudfront",
        "flag": "Server\\s*:\\s*cloudfront",
        "is_dev": false
    },
     {
        "app_class": "cdn",
        "app_version": "",
        "app_name": "aws:cloudfront",
        "flag": "X-Cache\\s*:\\s*cloudfront",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "comodo:comodo",
        "flag": "Protected by COMODO",
        "is_dev": false
    },
     {
        "dev_class": "gateway",
        "dev_model": "DataPower",
        "dev_vendor": "ibm",
        "flag": "X-Backside-Transport\\s*:\\s*\\A(OK|FAIL)",
        "is_dev": true
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "crunchbase:denyall",
        "flag": "Set-Cookie\\s*:\\s*\\Asessioncookie=",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "applicure:dotdefender",
        "flag": "X-dotDefender-denied",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "imperva:incapsula",
        "flag": "X-CDN\\s*:\\s*Incapsula",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "jiasule:jiasule",
        "flag": "Set-Cookie\\s*:\\s*jsluid=",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "akamai:akamaighost",
        "flag": "Server\\s*:\\s*AkamaiGHost",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "trustwave:modsecurity",
        "flag": "Server\\s*:\\s*Mod_Security",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "netcontinuum:netcontinuum",
        "flag": "Cneonction\\s*:\\s*\\Aclose",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "netcontinuum:netcontinuum",
        "flag": "nnCoection\\s*:\\s*\\Aclose",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "netcontinuum:netcontinuum",
        "flag": "Set-Cookie\\s*:\\s*citrix_ns_id",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "cloudxns:newdefend",
        "flag": "Server\\s*:\\s*newdefend",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "nsfocus:nsfocus",
        "flag": "Server\\s*:\\s*NSFocus",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "safe3:safe3",
        "flag": "X-Powered-By\\s*:\\s*Safe3WAF",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "safe3:safe3",
        "flag": "Safe3 Web Firewall",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "safedog:safedog",
        "flag": "Server\\s*:\\s*Safedog",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "safedog:safedog",
        "flag": "Set-Cookie\\s*:\\s*Safedog",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "sonicwall:aventail_web_proxy_agent",
        "flag": "Server\\s*:\\s*SonicWALL",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "riverbed:stingray",
        "flag": "Set-Cookie\\s*:\\s*\\AX-Mapping-",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "sucuri:sucuri",
        "flag": "Server\\s*:\\s*Sucuri/Cloudproxy",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "united-security-providers:secure_entry_server",
        "flag": "Server\\s*:\\s*Secure Entry Server",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "varnish:varnish",
        "flag": "X-Varnish",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "varnish:varnish",
        "flag": "Server\\s*:\\s*varnish",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "nginx:wallarm",
        "flag": "Server\\s*:\\s*nginx-wallarm",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "webknight:webknight",
        "flag": "Server\\s*:\\s*WebKnight",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "yundun:yundun",
        "flag": "Server\\s*:\\s*YUNDUN",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "yundun:yundun",
        "flag": "X-Cache\\s*:\\s*YUNDUN",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "",
        "app_name": "yunsuo:yunsuo",
        "flag": "Set-Cookie\\s*:\\s*yunsuo",
        "is_dev": false
    },
     {
        "app_class": "waf",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "360:panyun",
        "flag": "panyun/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
     {
        "app_class": "server",
        "app_version": "\\d+\\.\\d+\\.\\d+",
        "app_name": "apache:apache",
        "flag": " Apache/\\d+\\.\\d+\\.\\d+",
        "is_dev": false
    },
     {
        "app_class": "server",
        "app_version": "\\d+\\.\\d+\\.\\w*-*\\w*",
        "app_name": "openssl:openssl",
        "flag": "OpenSSL/\\d+\\.\\d+\\.\\w*-*\\w*",
        "is_dev": false
    },
     {
        "app_class": "web_langeuage",
        "app_version": "\\d+\\.\\d+\\.\\d+",
        "app_name": "php:php",
        "flag": "PHP/\\d+\\.\\d+\\.\\d+",
        "is_dev": false
    },
     {
        "app_class": "os",
          "depends": ["linux:linux"],
        "app_version": "",
        "app_name": "canonical:ubuntu_linux",
        "flag": "Server\\s*:\\s*.*(Ubuntu)",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "miranda-im:miranda",
        "flag": "Miranda FTP server",
        "is_dev": false
    },
     {
        "dev_class": "plc",
        "dev_model": "PAC320-XX",
        "dev_vendor": "Parker",
        "flag": "Parker FTP server",
        "is_dev": true
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "spftp:spftpd",
        "flag": "220 spFTP",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "ncftpd:ncftpd_ftp_server",
        "flag": "NcFTPd",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "netwin:surgeftp",
        "flag": "SurgeFTP",
        "is_dev": false
    },
     {
        "dev_class": "router",
        "dev_model": "netnumen_u31_r10",
        "dev_vendor": "zte",
        "flag": "ZTE FTP",
        "is_dev": true
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "solarwinds:serv-u",
        "flag": "Serv-U FTP",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "filezilla-project:filezilla_server",
        "flag": "FileZilla Server",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "windriver:vxworks_ftpd",
        "flag": "Wind River FTP",
        "is_dev": false
    },
      {
        "app_class": "os",
        "app_version": "",
        "app_name": "windriver:vxworks",
        "flag": "Wind River FTP",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "proftpd:proftpd",
        "flag": "ProFTPD",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "blah:blah_ftpd",
        "flag": "blah FTP",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "bftpd_project:bftpd",
        "flag": "bftpd",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "depends": ["vsftpd_project:vsftpd"],
        "app_version": "",
        "app_name": "beasts:vsftpd",
        "flag": "vsftpd",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "depends": ["microsoft:windows"],
        "app_version": "",
        "app_name": "microsoft:ftp_service",
        "flag": "Microsoft FTP",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "pureftpd:pure-ftpd",
        "flag": "Pure-FTPd",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "biscom:secure_file_transfer",
        "flag": "/bds/stylesheets/fds.css",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "crushftp:crushftp",
        "flag": "Server: CrushFTP HTTP Server",
        "is_dev": false
    },
     {
        "app_class": "ftp",
        "app_version": "",
        "app_name": "crushftp:crushftp",
        "flag": "CrushFTP WebInterface",
        "is_dev": false
    },
     {
        "app_class": "os",
        "app_version": "",
        "app_name": "ui:airos",
        "flag": "Set-Cookie\\s*:\\s*AIROS",
        "is_dev": false
    },
     {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\w*\\.*\\w*-*\\w*-*\\w*",
        "app_name": "apache:apache",
        "flag": "Server: Apache-Admin/\\d+\\.*\\d*\\.*\\w*\\.*\\w*-*\\w*-*\\w* \\(iTools 9.0.5",
        "is_dev": false
    },
     {
        "app_class": "server",
        "depends": ["macos:.macos"],
        "app_version": "\\d+\\.*\\d*\\.*\\w*\\.*\\w*-*\\w*-*\\w*",
        "app_name": "tenon:itools",
        "flag": "Server\\s*:.*\\(iTools \\d+\\.*\\d*\\.*\\w*\\.*\\w*-*\\w*-*\\w*",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "",
        "app_name": "fnal:scientific_linux",
        "flag": "Server\\s*:.*\\(Scientific Linux\\)",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "Debian",
        "app_name": "debian:debian_linux",
        "flag": "Server\\s*:.*\\(Debian\\)",
        "is_dev": false
    },
     {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "oracle:solaris",
        "flag": "Server\\s*:\\s*Oracle Solaris \\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "\\d+",
        "app_name": "freebsd:freebsd",
        "flag": "Server\\s*:.*freebsd\\d+\\)",
        "is_dev": false
    },
     {
        "app_class": "server",
         "implies": {"plone:plone": "plugin"},
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "zope:zope_management_interface",
        "flag": "Server\\s*:\\s*Zope/\\(\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
     {
        "app_class": "web_langeuage",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "python:python",
        "flag": "Server\\s*:.*python \\d+\\.\\d+\\.\\d+",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "",
        "app_name": "linuxmint:linuxmint",
        "flag": "X-Talaria-Flavor\\s*:\\s*mint",
        "is_dev": false
    },
     {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*/64-bit",
        "app_name": "ntop:ntopng",
        "flag": "Server\\s*:\\s*ntop/\\d+\\.*\\d*\\.*\\d*/64-bit \\(x86_64-redhat-linux-gnu\\)",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "x86_64",
        "app_name": "redhat:enterprise_linux_server",
        "flag": "Server\\s*:.*\\(x86_64-redhat-linux-gnu\\)",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "",
        "app_name": "redhat:enterprise_linux_server",
        "flag": "Server\\s*:.*\\(Red Hat",
        "is_dev": false
    },
     {
        "app_class": "web_langeuage",
        "app_version": "\\d+\\.\\d+\\.\\d+",
        "app_name": "python:python",
        "flag": "Server\\s*:.*Python/\\d+\\.\\d+\\.\\d+",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux","opensuse:leap"],
        "app_version": "",
        "app_name": "opensuse:opensuse",
        "flag": "Server\\s*:.*openSUSE",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "",
        "app_name": "opensuse:opensuse",
        "flag": "X-Opensuse-Runtimes: ",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "",
        "app_name": "fedoraproject:fedora",
        "flag": "Server\\s*:.*\\(Fedora\\)",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "\\d+\\.*\\d*",
        "app_name": "fedoraproject:fedora",
        "flag": "Server\\s*:\\sFedora/\\d+\\.*\\d*",
        "is_dev": false
    },
     {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "",
        "app_name": "centos:centos",
        "flag": "Server\\s*:.*\\(centos\\)",
        "is_dev": false
    },
    {
        "app_class": "web_middleware",
        "app_version": "\\d+\\.*\\d*\\.*\\d* (SP\\d*)*",
        "app_name": "bea:weblogic_integration",
        "flag": "Server\\s*:\\s*WebLogic Server \\d+\\.*\\d*\\.*\\d* (SP\\d*)*",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "ibm:websphere",
        "flag": "Server\\s*:\\s*WebSphere Application Server/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "ibm:websphere",
        "flag": "Server\\s*:\\s*IBM_HTTP_Server",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "oracle:enterprise_manager_for_fusion_applications",
        "flag": "<title>Welcome to Oracle Fusion Middleware</title>",
        "is_dev": false
    },
     {
        "dev_class": "RTLSDR",
        "dev_model": "1090",
        "dev_vendor": "Dump1090",
        "flag": "Server\\s*:\\s*Dump1090",
        "is_dev": true
    },
     {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "redhat:hornetq",
        "flag": "server:HornetQ/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "wso2:api_manager",
        "flag": "Server\\s*:\\s*WSO2 Carbon Server",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "wso2:api_manager",
        "flag": "<title>WSO2 Management Console",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "wso2:api_manager",
        "flag": "<title>WSO2 Enterprise Integrator (WSO2 EI)",
        "is_dev": false
    },
     {
        "app_class": "web_middleware",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "webesp:webesp",
        "flag": "Server\\s*:\\s*Webesp/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "webesp:webesp",
        "flag": "Server\\s*:\\s*Webesp",
        "is_dev": false
    },
     {
        "dev_class": "firewall",
        "dev_model": "NS\\d+\\.*\\d*\\.*\\d*",
        "dev_vendor": "citrix",
        "flag": "NetScaler NS\\d+\\.*\\d*\\.*\\d*: Build \\d+\\.*\\d*\\.*\\d*\\.*nc",
        "is_dev": true
    },
     {
        "app_class": "web_middleware",
        "depends": ["linux:linux","php:php","mysql:mysql","apache:apache"],
        "app_version": "",
        "app_name": "turnkey_web_tools:php_live_helper",
        "flag": "<title>TurnKey LAMP",
        "is_dev": false
    },
     {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "thinkphp:thinkphp",
        "flag": "X-Powered-By\\s*:\\s*ThinkPHP",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["php:php"],
        "app_version": "",
        "app_name": "thinkphp:thinkphp",
        "flag": "Set-Cookie\\s*:\\s*think_template",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "depends": ["thinkphp:thinkphp"],
        "app_version": "",
        "app_name": "74cms:74cms",
        "flag": "X-Powered-By\\s*:\\s*QSCMS",
        "is_dev": false
    },
      {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "swiftmailer:swiftmailer",
        "flag": "X-Generator\\s*:\\s*Swiftlet",
        "is_dev": false
    },
      {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "swiftmailer:swiftmailer",
        "flag": "220 swiftlet-eath.numbauction.net ESMTP Postfix",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "oracle:application_server",
        "flag": "X-Oracle-Dms-Ecid",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "oracle:application_server",
        "flag": "X-ORACLE-DMS-RID",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "managedfusion:managedfusion",
        "flag": "X-Rewritten-By\\s*:\\s*ManagedFusion",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "managedfusion:managedfusion",
        "flag": "X-ManagedFusion-Rewriter-Version\\s*:\\s*\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "microsoft:internet_information_server",
        "flag": "Server: Microsoft-IIS/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["php:php"],
        "app_version": "",
        "app_name": "php:php-cgi",
        "flag": "Content-Type\\s*:\\s*php-cgi",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "php:php-cgi",
        "flag": "Server\\s*:\\s*PHP-CGI/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "apple:webobjects",
        "flag": "X-Webobjects-Loadaverage",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "apple:webobjects",
        "flag": "X-Webobjects-Servlet",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["php:php","apache:apache","nginx:nginx","microsoft:sql_server"],
        "app_version": "",
        "app_name": "cakefoundation:cakephp",
        "flag": "Set-Cookie\\s*:\\s*CAKEPHP",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "bacula:bacula-web",
        "flag": "<title>Bacula-Web",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "zikula:zikula_application_framework",
        "flag": "Set-Cookie\\s*:\\s*ZIKULASID",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "mozart:mozart",
        "flag": "X-Powered-Cms\\s*:\\s*Mozart Framework",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "nette:application",
        "flag": "X-Powered-By\\s*:\\s*Nette Framework",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "fatfreeframework:fat-free_framework",
        "flag": "X-Powered-By\\s*:\\s*Fat-Free Framework",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "lightbend:play_framework",
        "flag": "Server\\s*:\\s*Play! Framework;\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
   {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "lightbend:play_framework",
        "flag": "Server\\s*:\\s*Play! Framework",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "lightbend:play_framework",
        "flag": "Set-Cookie\\s*:\\s*PLAY_ERRORS=",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "lightbend:play_framework",
        "flag": "Set-Cookies\\s*:\\s*PLAY_SESSION",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "restlet:restlet",
        "flag": "Server\\s*:\\s*Restlet-Framework/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "restlet:restlet",
        "flag": "Server\\s*:\\s*Restlet-Framework",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "kohanaframework:kohana",
        "flag": "Set-Cookie\\s*:\\s*kohanasession",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "flowplayer:flowplayer_html5",
        "flag": "/flowplayer",
        "is_dev": false
    },
     {
        "app_class": "web_framework",
        "app_version": "\\d+\\.\\d+\\.\\d+",
        "app_name": "flowplayer:flowplayer_html5",
        "flag": "flowplayer-\\d+\\.\\d+\\.\\d+\\.swf",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "siemens:simit",
        "flag": "X-Powered-By\\s*:\\s*SIMIT framework/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "siemens:simit",
        "flag": "X-Powered-By\\s*:\\s*SIMIT framework",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["microsoft:aspx"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "microsoft:.net_framework",
        "flag": "X-Aspnet-Version\\s*:\\s*\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "microsoft:.net_framework",
        "flag": "X-Powered-By\\s*:\\s*ASP\\.NET",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["microsoft:.net_framework"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "microsoft:.net_framework",
        "flag": "X-AspNetMvc-Version\\s*:\\s*\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "magento:magento",
        "flag": "alt=\"Magento Commerce\"",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "magento:magento",
        "flag": "<script type=\"text/x-magento-init\">",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "magento:magento",
        "flag": "Set-Cookie\\s*:\\s*frontend=",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "magento:magento",
        "flag": "<script [^>]+data-requiremodule=\"mage/",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "depends": ["microsoft:.net_framework"],
        "app_version": "",
        "app_name": "emah:elmah",
        "flag": "Exception Details: </b>Elmah.ApplicationException:",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "emah:elmah",
        "flag": "ElmahLogView\": \"Elmah Log View\",\"ReadoutReadPeriod",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.\\w*",
        "app_name": "eclipse:jetty",
        "flag": "Server\\s*:\\s*Jetty\\(\\d+\\.*\\d*\\.*\\d*\\.\\w*\\)",
        "is_dev": false
    },
      {
        "app_class": "server",
          "depends": ["java:java"],
        "app_version": "",
        "app_name": "apache:activemq",
        "flag": "WWW-Authenticate\\s*:\\s*.*realm=\"ActiveMQRealm\"",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.\\w*",
        "app_name": "apache:activemq",
        "flag": "Magic:ActiveMQ.*Version:\\d+\\.*\\d*\\.*\\d*\\.\\w*",
        "is_dev": false
    },
      {
        "app_class": "oams",
        "app_version": "",
        "app_name": "zabbix:zabbix",
        "flag": "<title>Zabbix",
        "is_dev": false
    },
      {
        "app_class": "oams",
        "depends": ["php:php"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "zabbix:zabbix",
        "flag": "Welcome to</span>Zabbix \\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "oams",
        "depends": ["linux:linux","php:php"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*-*\\d*",
        "app_name": "zabbix:zabbix",
        "flag": "Linux zabbix \\d+\\.*\\d*\\.*\\d*-*\\d*",
        "is_dev": false
    },
      {
        "app_class": "os",
        "depends": ["linux:linux"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "debian:debian_linux",
        "flag": "SMP Debian \\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "oams",
        "depends": ["linux:linux","php:php"],
        "app_version": "",
        "app_name": "parallels:parallels_plesk_panel_lin",
        "flag": "X-Powered-By.*:\\s*PleskLin",
        "is_dev": false
    },
    {
        "app_class": "oams",
        "depends": ["microsoft:windows","php:php"],
        "app_version": "",
        "app_name": "parallels:parallels_plesk_panel_win",
        "flag": "X-Powered-By.*:\\s*PleskWin",
        "is_dev": false
    },
      {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:solr",
        "flag": "<html ng-app=\"solrAdminApp\">\n",
        "is_dev": false
    },
      {
        "app_class": "bigdata",
          "depends": ["perl:perl","java:java"],
        "app_version": "",
        "app_name": "apache:solr",
        "flag": "<title>Solr Admin",
        "is_dev": false
    },
      {
        "app_class": "oa",
        "depends": ["apache:apache"],
        "app_version": "",
        "app_name": "tongda:tongda-oa",
        "flag": "shortcut icon\"\\s*href=\".*/tongda.ico",
        "is_dev": false
    },
    {
        "app_class": "oa",
        "app_version": "",
        "app_name": "tongda:tongda-oa",
         "favicon": {
            "hash": -759108386
        },
        "is_dev": false
    },
      {
        "app_class": "oa",
        "app_version": "2015",
        "app_name": "tongda:tongda-oa",
        "flag": "<title>Office Anywhere 2015版 网络智能办公系统",
        "is_dev": false
    },
    {
        "app_class": "oa",
        "app_version": "2013",
        "app_name": "tongda:tongda-oa",
        "flag": "<title>Office Anywhere 2013版 网络智能办公系统",
        "is_dev": false
    },
      {
        "app_class": "oa",
          "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "tongda:tongda-oa",
        "flag": "<title>通达OA网络智能办公系统",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "depends": ["php:php"],
        "app_version": "",
        "app_name": "fastvelocity:minify",
        "flag": "href=\"/Minify.php",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "fastvelocity:minify",
        "flag": "src=\"minify.php",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "phpmyadmin:phpmyadmin",
        "flag": "<h1>.*lang=\".*\">phpMyAdmin",
        "is_dev": false
    },
      {
        "app_class": "oa",
        "depends": ["java:java","microsoft:sql_server|oracle:database_server","apache:apache"],
        "app_version": "",
        "app_name": "weaver:ecology",
        "flag": "Set-Cookie\\s*:\\s*ecology",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "",
        "app_name": "wvs:wvs",
        "flag": "Server\\s*:\\s*WVS",
        "is_dev": false
    },
      {
        "app_class": "oa",
        "app_version": "",
        "app_name": "weaver:ecology",
        "flag": "/js/jquery/jquery_wev8.js",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["getbootstrap:bootstrap"],
        "app_version": "",
        "app_name": "getbootstrap:bootstrap-table",
        "flag": "bootstrap-table(?:\\.min)?\\.js",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "getbootstrap:bootstrap",
        "flag": "bootstrap.min.css",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "depends": ["mongodb:mongodb","php:php"],
          "app_version": "",
        "app_name": "avinu:phpmoadmin",
        "flag": "<title>phpMoAdmin",
        "is_dev": false
    },
      {
        "app_class": "cms",
          "depends": ["java:java"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\s*\\w*",
        "app_name": "liferay:liferay_portal",
        "flag": "Liferay-Portal\\s*:\\s*Liferay.*\\d+\\.*\\d*\\.*\\d*\\s*\\w*",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "liferay:liferay_portal",
        "flag": "Liferay-Portal\\s*:\\s*Liferay",
        "is_dev": false
    },
      {
        "app_class": "blog",
          "depends": ["perl:perl"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "sixapart:movable_type",
        "flag": "content=\"Movable Type.*\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "adobe:coldfusion",
        "flag": "/cfajax/",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "adobe:coldfusion",
        "flag": "/cfajax/",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "adobe:coldfusion",
        "flag": "Set-Cookie\\s*:\\s*CFTOKEN=",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "sap:business_one_2005-a",
        "flag": "<title>SAP NetWeaver",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "\\d+\\.\\d*\\.*\\d*\\.*\\d*",
        "app_name": "sap:business_one_2005-a",
        "flag": "Server\\s*:\\s*SAP J2EE Engine/\\d+\\.\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
          "depends": ["php:php"],
        "app_version": "\\d+\\.\\d*\\.*\\d*\\.*\\d*",
        "app_name": "zend:zend_framework",
        "flag": "X-Powered-By\\s*:.*Zend Core/\\d+\\.\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "",
        "app_name": "anpm:anpm",
        "flag": "Server\\s*:\\s*ANPM Web Server",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "\\d+\\.\\d*\\.*\\d*\\.*\\d*",
        "app_name": "mediawiki:mediawiki",
        "flag": "content=\"MediaWiki\\s*\\d+\\.\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "cms",
          "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "mediawiki:mediawiki",
        "flag": "<title>user's Wiki!",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "mediawiki:mediawiki",
        "flag": "body class=\"mediawiki",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "mediawiki:mediawiki",
        "flag": "<title>.*MediaWiki</title>",
        "is_dev": false
    },
      {
        "app_class": "web_framewrok",
          "depends": ["java:java"],
        "app_version": "\\d+\\.\\d*\\.*\\d*/*\\d+\\.\\d*\\.*\\d*",
        "app_name": "barracudanetworks:yosemite_server_backup",
        "flag": "Server\\s*:\\s*BarracudaHTTP \\d+\\.\\d*\\.*\\d*/*\\d+\\.\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "encoder-plugin",
          "implies": {"php:php": "plugin"},
        "app_version": "",
        "app_name": "ioncube:php_encoder",
        "flag": "<span>ionCube Loader developed by ionCube Ltd.",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "\\d+\\.\\d*\\.*\\d*",
        "app_name": "ioncube:php_encoder",
        "flag": "ionCube24.*v\\d+\\.\\d*\\.*\\d*,",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "\\d+\\.\\d*\\.*\\d*",
        "app_name": "parallels:parallels_plesk_panel",
        "flag": "<title>Plesk Onyx \\d+\\.\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "parallels:parallels_plesk_panel",
        "flag": "<title>Plesk",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "\\d+\\.\\d*\\.*\\d*",
        "app_name": "parallels:parallels_plesk_panel",
        "flag": "<title>Plesk \\d+\\.\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "parallels:parallels_plesk_panel",
        "flag": "Parallels IP Holdings GmbH\\. All rights reserved\\.",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["python:python"],
        "app_version": "\\d+\\.*\\d*-*\\d*-*\\d*",
        "app_name": "flask-cors_project:flask-cors",
        "flag": "tag: \"flask-\\d+\\.*\\d*-*\\d*-*\\d*",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "flask-cors_project:flask-cors",
        "flag": "<title>Flask",
        "is_dev": false
    },
      {
        "app_class": "oa",
        "app_version": "\\d+\\.*\\d*\\.*\\d*SP\\d*",
        "app_name": "seeyon:seeyon",
        "flag": "<title>.*管理软件 V\\d+\\.*\\d*\\.*\\d*SP\\d*",
        "is_dev": false
    },
      {
        "app_class": "oa",
        "app_version": "\\d+_*\\d*_*\\d*SP\\d*",
        "app_name": "seeyon:seeyon",
        "flag": "/seeyon/.*(js|ico)\\?V=V\\d+_*\\d*_*\\d*SP\\d+",
        "is_dev": false
    },
      {
        "app_class": "oa",
        "app_version": "",
        "app_name": "seeyon:seeyon",
        "flag": "var _ctxPath = '/seeyon'",
        "is_dev": false
    },
      {
        "app_class": "server",
        "depends": ["seeyon:seeyon","apache:tomcat","mysql:mysql"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "seeyon:seeyon-server",
        "flag": "Server: Seeyon-Server/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "oa",
          "depends": ["apache:tomcat","mysql:mysql"],
        "app_version": "",
        "app_name": "weaver:e-bridge",
        "flag": "<title>泛微云桥e-Bridge",
        "is_dev": false
    },
      {
       "app_class": "oa",
        "app_version": "",
        "app_name": "weaver:e-bridge",
        "flag": "content=\"泛微云桥e-Bridge",
        "is_dev": false
    },
      {
        "app_class": "cms",
          "depends": ["java:java"],
        "app_version": "",
        "app_name": "fasterxml:jackson",
        "flag": "Server\\s*:\\s*windows-jackson-1",
        "is_dev": false
    },
      {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "oracle:glassfish_server",
        "flag": "Server\\s*:\\s*GlassFish Server",
        "is_dev": false
    },
      {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "oracle:glassfish_server",
        "flag": "<title>GlassFish Server",
        "is_dev": false
    },
      {
        "app_class": "web_middleware",
        "app_version": "",
        "app_name": "oracle:glassfish_server",
        "flag": "title=\"Log In to GlassFish",
        "is_dev": false
    },
      {
        "app_class": "server",
          "depends": ["microsoft:windows","microsoft:.net_framework"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "microsoft:sharepoint_enterprise_server",
        "flag": "MicrosoftSharePointTeamServices: \\d+\\.*\\d*\\.*\\d*\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:spark",
        "flag": "<title>.*Spark.*</title>",
        "is_dev": false
    },
      {
        "app_class": "bigdata",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "apache:spark",
        "flag": "Server\\s*:\\s*Sparkred/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "bigdata",
        "app_version": "",
        "app_name": "apache:spark",
        "flag": "Server\\s*:\\s*Sparkred",
        "is_dev": false
    },
      {
        "app_class": "web_langeuage",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "ruby:ruby",
        "flag": "Server\\s*:.*Ruby/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["ruby:ruby"],
        "app_version": "",
        "app_name": "rubyonrails:rails",
        "flag": "Server\\s*:\\s*mod_(?:rails|rack)",
        "is_dev": false
    },
      {
         "app_class": "web_framework",
        "app_version": "",
        "app_name": "rubyonrails:rails",
        "flag": "X-Powered-By\\s*:\\s*mod_(?:rails|rack)",
        "is_dev": false
    },
      {
        "app_class": "oams",
        "depends": ["rubyonrails:rails"],
        "app_version": "",
        "app_name": "gitlab:gitlab",
        "flag": "<title>Sign in · GitLab",
        "is_dev": false
    },
      {
        "app_class": "oams",
        "app_version": "",
        "app_name": "gitlab:gitlab",
        "flag": "<title>登录 · GitLab",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "depends": ["rubyonrails:rails"],
        "app_version": "",
        "app_name": "chatspace:chatspace",
        "flag": "<title>ChatSpace",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "rubyonrails:rails",
        "flag": "<title>Ruby on Rails",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "",
        "app_name": "webdav:webdav",
        "flag": "Ms-Author-Via: DAV",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "\\d*",
        "app_name": "webdav:webdav",
        "flag": "Server:.*\\s*DAV/*\\d*",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "webdav:webdav",
        "flag": "Server: webdav-server/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
         "app_class": "server",
        "app_version": "",
        "app_name": "webdav:webdav",
        "flag": "Server: webdav-server",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "cakefoundation:cakephp",
        "flag": "Set-Cookie: CAKEPHP",
        "is_dev": false
    },
      {
        "app_class": "editor-plugin",
        "app_version": "",
        "app_name": "ewebeditor:ewebeditor",
        "flag": "window._CONFIG\\['ewebeditor'\\] ",
        "is_dev": false
    },
      {
        "app_class": "editor-plugin",
        "app_version": "",
        "app_name": "ewebeditor:ewebeditor",
        "flag": "img src=\".*/ewebeditor/uploadfile/",
        "is_dev": false
    },
      {
        "app_class": "editor-plugin",
        "app_version": "",
        "app_name": "ewebeditor:ewebeditor",
        "flag": "img src=\".*ewebeditor/uploadfile/",
        "is_dev": false
    },
      {
        "app_class": "server",
          "depends": ["java:java"],
        "implies": {"sonatype:nexus_repository_manager": "server"},
        "app_version": "\\d+\\.*\\d*\\.*\\d*-*\\d*",
        "app_name": "sonatype:nexus_repository_manager",
        "flag": "Server: Nexus/\\d+\\.*\\d*\\.*\\d*-*\\d*",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "sonatype:nexus_repository_manager",
        "flag": "<title>Nexus Repository Manager",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "sonatype:nexus_repository_manager",
        "flag": "<title>Sonatype Nexus",
        "is_dev": false
    },
      {
       "app_class": "server",
        "app_version": "",
        "app_name": "sonatype:nexus_repository_manager",
        "flag": "Server: Nexus",
        "is_dev": false
    },
      {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "coremail_xt_project:coremail_xt",
        "flag": "\\+OK Welcome to coremail Mail Pop3 Server",
        "is_dev": false
    },
      {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "coremail_xt_project:coremail_xt",
        "flag": "<title>Coremail",
        "is_dev": false
    },
      {
        "app_class": "editor-plugin",
        "app_version": "",
        "app_name": "baidu:ueditor",
        "flag": "src=\".*ueditor/ueditor.config.js",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["java:java"],
        "app_version": "",
        "app_name": "pivotal_software:spring_framework",
        "flag": "<title>Spring Boot Admin",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "pivotal_software:spring_framework",
        "flag": "X-Application-Context:",
        "is_dev": false
    },
      {
        "app_class": "cms",
         "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "dedecms:dedecms",
        "flag": "link href=\".*dedecms.css",
        "is_dev": false
    },
      {
        "app_class": "cms",
        "app_version": "",
        "app_name": "dedecms:dedecms",
        "flag": "Power by DedeCms",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "depends": ["nodejs:node.js"],
        "app_version": "",
        "app_name": "nodered:node-red",
        "flag": "<title>Node-RED",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "nodejs:node.js",
        "flag": "X-Powered-By: Express",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "nodejs:node.js",
        "flag": "Server\\s*:.*node.js",
        "is_dev": false
    },
      {
        "app_class": "server",
        "depends": ["java:java"],
        "app_version": "2",
        "app_name": "apache:axis",
        "flag": "<title>Axis 2",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "",
        "app_name": "apache:axis",
        "flag": " href=\".*axis-style.css",
        "is_dev": false
    },
      {
        "app_class": "log-plugin",
        "implies": {"rap2hpoutre:rap2hpoutre": "plugin"},
        "app_version": "",
        "app_name": "laravel_log_viewer_project:laravel_log_viewer",
        "flag": "Rap2hpoutre.*aravelLogViewer",
        "is_dev": false
    },
      {
        "app_class": "server",
        "depends": ["java:java"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "caucho:resin",
        "flag": "Server\\s*:.*Resin/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "",
        "app_name": "unbit:uwsgi",
        "flag": "Server: uWSGI",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "unbit:uwsgi",
        "flag": "Server\\s*:.*mod_uwsgi/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "server",
          "depends": ["java:java"],
        "app_version": "",
        "app_name": "jaas:jaas",
        "flag": "href=\".*jaas/sienge-app.css",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "\\d+\\.\\d*\\.*\\d*",
        "app_name": "jquery:jquery",
        "flag": "src=.*jquery-\\d+\\.\\d*\\.*\\d*.min.js",
        "is_dev": false
    },
      {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "jquery:jquery",
        "flag": "src=.*jquery.js",
        "is_dev": false
    },
      {
        "app_class": "oams",
        "depends": ["cgi:cgi"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "awstats:awstats",
        "flag": "name=\"generator\" content=\"AWStats \\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
      {
        "app_class": "oams",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "awstats:awstats",
        "flag": "<title>Statistics for",
        "is_dev": false
    },
      {
        "app_class": "server",
        "app_version": "",
        "app_name": "microsoft:internet_information_server",
        "flag": "Server:.*IIS",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "microsoft:internet_information_server",
        "flag": "Server:.*IIS/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "acme:mini-httpd",
        "flag": "Server:.*MiniHttpd \\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
       "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "acme:mini-httpd",
        "flag": "Server:.*MiniHttpd/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "",
        "app_name": "acme:mini-httpd",
        "flag": "Server: MiniHttpd",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "ibm:lotus_protector_for_mail_security",
        "flag": "Server: Lotus-Domino",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "ibm:lotus_protector_for_mail_security",
        "flag": "Server: Lotus-Domino/\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "\\d+\\.*\\d*\\.*\\d*",
        "app_name": "ibm:lotus_protector_for_mail_security",
        "flag": "Server: Lotus-Domino/Release-*\\d+\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "depends": ["python:python"],
        "app_version": "",
        "app_name": "djangoproject:django",
        "flag": "<title>.*Django.*</title>",
        "is_dev": false
    },
    {
        "app_class": "editor-plugin",
        "app_version": "\\d+",
        "app_name": "ckeditor:ckeditor",
        "flag": "src=.*ckeditor\\.js\\?v=\\d+",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "typo3:typo3",
        "flag": "This website is powered by TYPO3",
        "is_dev": false
    },
     {
        "app_class": "cms",
        "app_version": "",
        "app_name": "typo3:typo3",
        "flag": "name=\"generator\" content=\"TYPO3 CMS\"",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "episerver:ektron_cms",
        "flag": "Set-Cookie: EktGUID",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "episerver:ektron_cms",
        "flag": "src=.*ektron\\.js",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "shopex:ecshop",
        "flag": "Set-Cookie: ECS_ID",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "shopex:ecshop",
        "flag": "src=\"api/cron.php",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql","apache:apache","nginx:nginx","phpmyadmin:phpmyadmin","microsoft:windows"],
        "app_version": "",
        "app_name": "phpstudy:phpstudy",
        "flag": "<title>phpStudy",
        "is_dev": false
    },
    {
        "app_class": "editor-plugin",
        "depends": ["microsoft:windows"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "microsoft:frontpage",
        "flag": "Server:.*FrontPage/\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "editor-plugin",
        "app_version": "",
        "app_name": "microsoft:frontpage",
        "flag": "Server:.*FrontPage",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["microsoft:.net_framework","microsoft:sql_server"],
        "app_version": "",
        "app_name": "umbraco:umbraco_cms",
        "flag": "Powered by Umbraco",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "umbraco:umbraco_cms",
        "flag": "html xmlns:umbraco=\"http://umbraco\\.org",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["jenkins:mercurial"],
        "app_version": "",
        "app_name": "atlassian:bitbucket",
        "flag": "content=\"Bitbucket\"",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "atlassian:crucible",
        "flag": "<title>.*Crucible\\s*\\d+\\.*\\d*\\.*\\d*\\.*\\d*</title>",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["mysql:mysql","java:java"],
        "app_version": "",
        "app_name": "atlassian:crucible",
        "flag": "<title>.*Crucible.*</title>",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "depends": ["java:java"],
        "app_version": "",
        "app_name": "apache:shiro",
        "flag": "Set-Cookie: rememberMe",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "apache:shiro",
        "flag": "class=\"fa fa-arrow-circle-o-right m-r-xs\"></i> shiro</li>",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "apache:shiro",
        "flag": "<title>.*Shiro权限管理系统</title>",
        "is_dev": false
    },
    {
        "app_class": "bbs",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "vbulletin:vbulletin",
        "flag": "\"generator\" content=\"vBulletin \\d+\\.*\\d*\\.*\\d*\\.*\\d*\"",
        "is_dev": false
    },
    {
        "app_class": "bbs",
        "app_version": "",
        "app_name": "vbulletin:vbulletin",
        "flag": "Powered by vBulletin",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "lighttpd:lighttpd",
        "flag": "Server:.*lighttpd/\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "",
        "app_name": "lighttpd:lighttpd",
        "flag": "Server:.*lighttpd",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "ibm:websphere",
        "flag": "Server: WebSphere Application Server/\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "ibm:websphere",
        "flag": "WebSphere Application Server V\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "ibm:websphere",
        "flag": "<title>WebSphere",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "depends": ["java:java"],
        "app_version": "",
        "app_name": "ibm:websphere",
        "flag": "Server: Fado Websphere \\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["mysql:mysql|postgresql:postgresql|oracle:database_server|microsoft:sql_server","git:git","java:java"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "atlassian:jira",
        "flag": "data-name=\"jira\" data-version=\"\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "atlassian:jira",
        "flag": "<title>.*JIRA.*</title>",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "atlassian:jira",
        "flag": "WRM._unparsedData\\[\"jira",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "",
        "app_name": "apache:tomcat",
        "flag": "Server: Apache-Coyote",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "apache:tomcat",
        "flag": "Apache Tomcat/\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "",
        "app_name": "apache:tomcat",
        "flag": "<title>Apache Tomcat",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "apache:tomcat",
        "flag": "Server: Apache Tomcat \\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "",
        "app_name": "apache:tomcat",
        "flag": "realm=\"Tomcat Manager Application",
        "is_dev": false
    },
    {
        "app_class": "bbs",
        "app_version": "",
        "app_name": "discuz:discuzx",
        "flag": "content=\"Discuz",
        "is_dev": false
    },
    {
        "app_class": "bbs",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "discuz:discuzx",
        "flag": "content=\"Discuz!\\s*\\w*\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "bbs",
        "app_version": "",
        "app_name": "discuz:discuzx",
        "flag": "Powered by Discuz",
        "is_dev": false
    },
    {
        "app_class": "oams",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "webalizer:webalizer",
        "flag": "Webalizer Version \\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "oams",
        "app_version": "",
        "app_name": "webalizer:webalizer",
        "flag": "Generated by The Webalizer",
        "is_dev": false
    },
    {
        "app_class": "oams",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "webalizer:webalizer",
        "flag": "Webalizer  Ver. \\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "server",
        "depends": ["ruby:ruby"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "ruby-lang:webrick",
        "flag": "Server:.*WEBrick/\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "web_middleware",
        "depends": ["j2ee:j2ee"],
        "app_version": "",
        "app_name": "bea:weblogic_integration",
        "flag": "CONTENT=\"WebLogic Server",
        "is_dev": false
    },
    {
        "app_class": "web_middleware",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "bea:weblogic_integration",
        "flag": "Weblogic Application Server \\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "server",
        "implies": {"bea:weblogic_integration": "server"},
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "bea:weblogic_server",
        "flag": "Server:.*WebLogic Server \\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "server",
        "app_version": "",
        "app_name": "bea:weblogic_server",
        "flag": "Server:.*WebLogic Server",
        "is_dev": false
    },
    {
        "app_class": "oams",
        "app_version": "",
        "app_name": "vmware:esxi",
        "flag": "content=\"VMware ESXi",
        "is_dev": false
    },
    {
        "app_class": "oams",
        "depends": ["vmware:esxi"],
        "app_version": "",
        "app_name": "vmware:vsphere",
        "flag": "content=\"VMware vSphere",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "jenkins:jenkins",
        "flag": "\\[Jenkins\\]</title>",
        "is_dev": false
    },
    {
        "app_class": "web_framework",
        "app_version": "",
        "app_name": "clipboard:clipboard",
        "flag": "src=.*clipboard.min.js",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["microsoft:.net_framework","microsoft:sql_server|mysql:mysql"],
        "app_version": "",
        "app_name": "siteserver:siteserver",
        "flag": "Powered by SiteServer CMS",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "siteserver:siteserver",
        "flag": "Powered by <a href=\"https://www\\.siteserver",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "jboss:jboss",
        "flag": "Server:.*JBoss-\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "jboss:jboss",
        "flag": "Server:.*JBossWeb-\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\d*",
        "app_name": "jboss:jboss",
        "flag": "<title>.*JBoss.*\\d+\\.*\\d*\\.*\\d*\\.*\\d*</title>",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "app_name": "jboss:jboss",
        "flag": "JBossWeb/\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "is_dev": false
    },
    {
        "app_class": "oams",
        "app_version": "",
        "app_name": "gitlab:gitlab",
        "flag": "<meta content=\"https?://[^/]+/assets/gitlab_logo-",
        "is_dev": false
    },
    {
       "app_class": "oams",
        "app_version": "",
        "app_name": "gitlab:gitlab",
        "flag": "<header class=\"navbar navbar-fixed-top navbar-gitlab with-horizontal-nav\">",
        "is_dev": false
    },
    {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "axigen:axigen_mail_server",
        "flag": "Server: Axigen-Webmail",
        "is_dev": false
    },
    {
        "app_class": "MailServer",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "app_name": "axigen:axigen_mail_server",
        "flag": "<title>AXIGEN Webmail.*\\d+\\.*\\d*\\.*\\d*\\.*\\w*</title>",
        "is_dev": false
    },
    {
        "app_class": "MailServer",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "app_name": "kerio:kerio_mailserver",
        "flag": "Server: Kerio MailServer \\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "is_dev": false
    },
    {
        "app_class": "MailServer",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "app_name": "kerio:kerio_mailserver",
        "flag": "<title>Kerio MailServer \\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "is_dev": false
    },
    {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "fresh-webmail:fresh-webmail",
        "flag": "Server: FRESH WebMail",
        "is_dev": false
    },
    {
        "app_class": "MailServer",
        "depends": ["microsoft:windows"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "app_name": "winwebmail:winwebmail_server",
        "flag": "ESMTP on WinWebMail \\[\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "is_dev": false
    },
    {
        "app_class": "MailServer",
        "app_version": "",
        "app_name": "winwebmail:winwebmail_server",
        "flag": "<title>WinWebMail",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "implies": {"elasticsearch:elasticsearch": "client"},
        "app_version": "",
        "app_name": "elasticsearch:elasticsearch-client",
        "flag": "<body ng-app=\"elasticsearchSqlApp\"",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "depends": ["java:java"],
        "app_version": "",
        "app_name": "elasticsearch:elasticsearch-client",
        "flag": "<title>Elasticsearch-sql client",
        "is_dev": false
    },
    {
        "app_class": "bigdata",
        "implies": {"elasticsearch:elasticsearch": "hq"},
        "app_version": "",
        "app_name": "elasticsearch:elasticsearch-hq",
        "flag": "<title>Elastic Search - HQ",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "plone:plone",
        "flag": "content=\"Plone - http://plone",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["python:python","mysql:mysql"],
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "app_name": "plone:plone",
        "flag": "Server:.*Plone/\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "depends": ["php:php","mysql:mysql"],
        "app_version": "",
        "app_name": "joomla:joomla\\!",
        "flag": "(?:<div[^>]+id=\"wrapper_r\"|<(?:link|script)[^>]+(?:feed|components)/com_|<table[^>]+class=\"pill)\\;confidence:50",
        "is_dev": false
    },
    {
         "app_class": "cms",
        "app_version": "",
        "app_name": "joomla:joomla\\!",
        "flag": "name=\"generator\" content=\"Joomla",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "",
        "app_name": "joomla:joomla\\!",
        "flag": "<script type=\"application/json\" class=\"joomla-script-options new\">",
        "is_dev": false
    },
    {
        "app_class": "cms",
        "app_version": "\\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "app_name": "joomla:joomla\\!",
        "flag": "content=\"Joomla! \\d+\\.*\\d*\\.*\\d*\\.*\\w*",
        "is_dev": false
    },
    {
        "app_class": "os",
        "app_version": "64bit",
        "app_name": "microsoft:windows",
        "flag": "Server:.*\\(Win64\\)",
        "is_dev": false
    },
    {
        "app_class": "os",
        "app_version": "32bit",
        "app_name": "microsoft:windows",
        "flag": "Server:.*\\(Win32\\)",
        "is_dev": false
    },
    {
        "app_class": "os",
        "app_version": "32bit:10.0",
        "app_name": "microsoft:windows",
        "flag": "Server:.*\\(windows/10.0 x86",
        "is_dev": false
    },
    {
        "app_class": "os",
        "app_version": "6.1",
        "app_name": "microsoft:windows",
        "flag": "Server:.*Microsoft-Windows/6.1",
        "is_dev": false
    },
    {
        "app_class": "os",
        "app_version": "64bit:6.3",
        "app_name": "microsoft:windows",
        "flag": "Server:.*Windows 6.3 amd64",
        "is_dev": false
    },
    {
        "app_class": "os",
        "app_version": "64bit:6.1",
        "app_name": "microsoft:windows",
        "flag": "Server:.*Windows 7/6.1 amd64",
        "is_dev": false
    },
    {
        "app_class": "os",
        "app_version": "5.1",
        "app_name": "microsoft:windows",
        "flag": "Server:.*Windows/5.1",
        "is_dev": false
    },
    {
        "app_class": "os",
        "app_version": "64bit:6.1",
        "app_name": "microsoft:windows",
        "flag": "Server:.*Windows Server 2008/6.1 amd64",
        "is_dev": false
    }
]`

var (
	IoTDeviceRules         []*IotDevRule
	ApplicationDeviceRules []*IotDevRule
)

func loadFromRaw(ruleRawStr string) {
	var res []map[string]interface{}
	err := json.Unmarshal([]byte(ruleRawStr), &res)
	if err != nil {
		return
	}
	for _, r := range res {
		fp := &IotDevRule{}
		fp.AppClass = utils.MapGetString(r, "app_class")
		fp.AppVersion = utils.MapGetString(r, "app_version")
		if !strings.HasPrefix(fp.AppVersion, "(?i)") {
			fp.AppVersionRegexp, _ = regexp.Compile("(?i)" + fp.AppVersion)
		} else {
			fp.AppVersionRegexp, _ = regexp.Compile(fp.AppVersion)
		}

		fp.AppName = utils.MapGetString(r, "app_name")
		fp.DeviceVendor = utils.MapGetString(r, "dev_vendor")
		fp.DeviceModel = utils.MapGetString(r, "dev_model")
		fp.DeviceClass = utils.MapGetString(r, "dev_class")
		if !strings.HasPrefix(fp.DeviceModel, "(?i)") {
			fp.DeviceModelRegexp, _ = regexp.Compile(`(?i)` + fp.DeviceModel)
		} else {
			fp.AppVersionRegexp, _ = regexp.Compile(fp.AppVersion)
		}
		fp.Flag = utils.MapGetString(r, "flag")
		if !strings.HasPrefix(fp.Flag, "(?i)") {
			fp.FlagRegexp, _ = regexp.Compile("(?i)" + fp.Flag)
		} else {
			fp.FlagRegexp, _ = regexp.Compile(fp.Flag)
		}
		fp.IsDevice = utils.MapGetBool(r, "is_dev")

		switch ret := utils.MapGetRaw(r, "depends").(type) {
		case []interface{}:
			for _, i := range ret {
				fp.Depends = append(fp.Depends, fmt.Sprint(i))
			}
		}
		sort.Strings(fp.Depends)

		switch ret := utils.MapGetRaw(r, "implies").(type) {
		case map[string]interface{}:
			fp.Implies = make(map[string]string)
			for k, i := range ret {
				fp.Implies[k] = fmt.Sprint(i)
			}
		}

		favicon := utils.MapGetRaw(r, "favicon")
		if favicon != nil {
			continue
		}

		if fp.IsDevice {
			IoTDeviceRules = append(IoTDeviceRules, fp)
		} else {
			ApplicationDeviceRules = append(ApplicationDeviceRules, fp)
		}
	}
}

func init() {
	loadFromRaw(ruleRaw)
	loadFromRaw(extraRuleRaw)
}

type IoTDevMatchResult struct {
	VendorProduct string
	Version       string

	Rule *IotDevRule
}

func (i *IoTDevMatchResult) GetCPE() string {
	if i.Version == "" {
		i.Version = "*"
	}
	if i.Rule.IsDevice {
		return fmt.Sprintf("cpe:/a:%v:%v:*", i.VendorProduct, i.Version)
	}
	return fmt.Sprintf("cpe:/a:%v:%v:*", i.VendorProduct, i.Version)
}

func (i *IotDevRule) Match(result []byte) (*IoTDevMatchResult, error) {
	if i.FlagRegexp != nil && i.Flag != "" {
		mRes := &IoTDevMatchResult{}
		mRes.Rule = i
		if i.IsDevice {
			mRes.VendorProduct = fmt.Sprintf("*:%v", i.DeviceVendor)
			mRes.Version = i.DeviceModel
		} else {
			mRes.VendorProduct = i.AppName
			mRes.Version = i.AppVersion
		}
		matched := i.FlagRegexp.Find(result)
		if matched == nil {
			return nil, utils.Errorf("match failed")
		}

		if i.IsDevice {
			if i.DeviceModelRegexp != nil {
				verRaw := i.DeviceModelRegexp.Find(matched)
				if verRaw != nil {
					mRes.Version = string(verRaw)
				}
			} else {
				mRes.Version = i.DeviceModel
			}
		} else {
			if i.AppVersionRegexp != nil {
				verRaw := i.AppVersionRegexp.Find(matched)
				if verRaw != nil {
					mRes.Version = string(verRaw)
				}
			} else {
				mRes.Version = i.AppVersion
			}
		}
		return mRes, nil
	} else {
		return nil, utils.Errorf("rule regexp(iot flag) empty.")
	}
}

func MatchAll(banner []byte) []*IoTDevMatchResult {
	var res []*IoTDevMatchResult
	for _, i := range IoTDeviceRules {
		r, err := i.Match(banner)
		if err != nil {
			continue
		}
		res = append(res, r)
	}

	for _, i := range ApplicationDeviceRules {
		r, err := i.Match(banner)
		if err != nil {
			continue
		}
		res = append(res, r)
	}

	return res
}

/*
173.195.109.182:80
205.215.7.83:80
111.26.194.18:81
218.58.98.114:8009
223.84.144.235:1080
223.71.1.62:8888
114.32.98.114:80
114.32.143.137:80
 200.24.149.222:85
220.248.163.55:80
58.223.2.19,222.177.15.68 554
 scan-service -t 77.137.231.151,81.173.96.126 -p 8080,8082
+-------------------------+------------+
|          端口           | 指纹（简） |
+-------------------------+------------+
| tcp://77.245.102.146:80 |            |
| tcp://81.23.1.46:80     |            |
+-------------------------+------------+

*/

/*
LINKSYS /video.htm 未授权访问
app="LINKSYS-Internet-Camera"

http://195.60.132.24/
https://195.60.132.24:8888/

y-cam
app="Y-cam-Cube"

183.82.248.116

116.73.15.27

*/
