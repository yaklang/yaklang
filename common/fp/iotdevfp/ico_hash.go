package iotdevfp

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var raw = `{
	"1":{
		"dev_class": "ip_cam",
		"dev_model": "",
		"dev_vendor": "tvt",
		"favicon": {
			"hash": 492290497
		},
		"is_dev": true
	},
	"2":{
		"dev_class": "",
		"dev_model": "",
		"dev_vendor": "axis",
		"favicon": {
			"hash": -1616143106
		},
		"is_dev": true
	},
	"3":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "juanvision",
        "favicon": {
            "hash": 90066852
        },
        "is_dev": true
    },
    "4":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "abus",
        "favicon": {
            "hash": -313860026
        },
        "is_dev": true
    },
	"5":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "apd",
        "favicon": {
            "hash": 347462278
        },
        "is_dev": true
    },
    "6":{
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "ruckus",
        "favicon": {
            "hash": -2069844696
        },
        "is_dev": true
    },
    "7":{
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "mikrotik",
        "favicon": {
            "hash": 1924358485
        },
        "is_dev": true
    },
    "8":{
        "dev_class": "router",
        "dev_model": "",
        "dev_vendor": "ruijie",
        "favicon": {
            "hash": 772273815
        },
        "is_dev": true
    },
    "9":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "zavio",
        "favicon": {
            "hash": 623744943
        },
        "is_dev": true
    },
    "10":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "intellinet",
        "favicon": {
            "hash": 405527018
        },
        "is_dev": true
    },
    "11":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "abelcam",
        "favicon": {
            "hash": 11685462
        },
        "is_dev": true
    },
    "12":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": " hikvision",
        "favicon": {
            "hash": 999357577
        },
        "is_dev": true
    },
    "13":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "xm030",
        "favicon": {
            "hash": 469671045
        },
        "is_dev": true
    },
    "14":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "hikvision",
        "favicon": {
            "hash": 999357577
        },
        "is_dev": true
    },
	"15":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "tp-link",
        "favicon": {
            "hash": -2101256422
        },
        "is_dev": true
    },
        "16":{
        "dev_class": "nas",
        "dev_model": "",
        "dev_vendor": "qnap",
        "favicon": {
            "hash": 911067989
        },
        "is_dev": true
    },
    "17":{
        "dev_class": "ip_cam",
        "dev_model": "",
        "dev_vendor": "mercury",
        "favicon": {
            "hash": -1257440778
        },
        "is_dev": true
    },
     "18":{
        "dev_class": "router",
        "dev_model": "honer Pro 2",
        "dev_vendor": "huawei",
        "favicon": {
            "hash": 665440234
        },
        "is_dev": true
    },
     "19":{
        "dev_class": "router",
        "dev_model": "Internet_Surfing_Management_Route",
        "dev_vendor": "volans",
        "favicon": {
            "hash": -842852785
        },
        "is_dev": true
    }
}`

type IcoHashFingerprint struct {
	DevClass    string
	DevModel    string
	DevVendor   string
	FaviconHash int64
	IsDevice    bool
}

var FaviconFps []*IcoHashFingerprint

func init() {
	var res = make(map[string]interface{})
	err := json.Unmarshal([]byte(raw), &res)
	if err != nil {
		log.Error("load ico hash failed")
		return
	}

	for _, val := range res {
		switch ret := val.(type) {
		case map[string]interface{}:
			fp := &IcoHashFingerprint{}
			fp.DevClass = utils.MapGetString(ret, "dev_class")
			fp.DevModel = utils.MapGetString(ret, "dev_model")
			fp.DevVendor = utils.MapGetString(ret, "dev_vendor")
			iconHash := utils.MapGetFloat64(utils.MapGetMapRaw(ret, "favicon"), "hash")
			fp.FaviconHash = int64(iconHash)
			fp.IsDevice = utils.MapGetBool(ret, "is_dev")
			FaviconFps = append(FaviconFps, fp)
		}
	}
}
