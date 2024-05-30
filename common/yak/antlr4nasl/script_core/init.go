package script_core

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/embed"
)

var naslLogger = log.GetLogger("NASL Logger")
var GlobalPrefs = map[string]string{
	"plugins_folder":           "MAGENI_NVT_DIR",
	"include_folders":          "MAGENI_NVT_DIR",
	"max_hosts":                "30",
	"max_checks":               "10",
	"be_nice":                  "yes",
	"log_whole_attack":         "no",
	"log_plugins_name_at_load": "no",
	"optimize_test":            "yes",
	"network_scan":             "no",
	"non_simult_ports":         "139, 445, 3389, Services/irc",
	"plugins_timeout":          "5",
	"scanner_plugins_timeout":  "5",
	"safe_checks":              "yes",
	"auto_enable_dependencies": "yes",
	"drop_privileges":          "no",
	// Empty options must be "\0", not NULL, to match the behavior of
	// prefs_init.
	"report_host_details":     "yes",
	"db_address":              "",
	"cgi_path":                "/cgi-bin:/scripts",
	"checks_read_timeout":     "5",
	"unscanned_closed":        "yes",
	"unscanned_closed_udp":    "yes",
	"timeout_retry":           "3",
	"expand_vhosts":           "yes",
	"test_empty_vhost":        "no",
	"open_sock_max_attempts":  "5",
	"time_between_request":    "0",
	"nasl_no_signature_check": "yes",
}

func PatchEngine(engine *ScriptEngine) {
	// 需要把ACT_SCAN的脚本都patch一遍
	engine.AddScriptPatch("nmap_mac.nasl", func(code string) string {
		codeBytes, err := embed.Asset("data/nasl-patches/" + "nmap_mac_patch.nasl")
		if err != nil {
			log.Errorf("read nmap_mac_patch.nasl error: %v", err)
			return code
		}
		return string(codeBytes)
	})
	engine.AddScriptPatch("apache_tomcat_config.nasl", func(code string) string {
		codeBytes, err := embed.Asset("data/nasl-patches/" + "apache_tomcat_config_patch.nasl")
		if err != nil {
			log.Errorf("read apache_tomcat_config_patch.nasl error: %v", err)
			return code
		}
		return string(codeBytes)
	})
	engine.AddScriptPatch("ping_host.nasl", func(code string) string {
		codeBytes, err := embed.Asset("data/nasl-patches/" + "ping_host_patch.nasl")
		if err != nil {
			log.Errorf("read ping_host_patch.nasl error: %v", err)
			return code
		}
		return string(codeBytes)
	})
	engine.AddScriptPatch("http_func", func(s string) string {
		s += `

function http_get_port( default_list, host, ignore_broken, ignore_unscanned, ignore_cgi_disabled, dont_use_vhosts ) {
 local_var final_port_list;

  final_port_list = http_get_ports(default_list:default_list,host:host,ignore_broken:ignore_broken,ignore_unscanned:ignore_unscanned,ignore_cgi_disabled:ignore_cgi_disabled,dont_use_vhosts:dont_use_vhosts);
  foreach port( final_port_list ) {
	return port;
  }
  return -1;
}
`
		return s
	})
	engine.AddScriptPatch("smtp_func", func(s string) string {
		s += `
function smtp_get_port( default_list, ignore_broken, ignore_unscanned ) {

  local_var final_port_list;

  final_port_list = smtp_get_ports(default_list:default_list,ignore_broken:ignore_broken,ignore_unscanned:ignore_unscanned);
	foreach port( final_port_list ) {
		return port;
	}
	return -1;
}
`
		return s
	})
	//engine.AddScriptPatch("gb_altn_mdaemon_http_detect.nasl", func(code string) string {
	//	codeLines := strings.Split(code, "\n")
	//	if len(codeLines) > 55 {
	//		codeLines[55] = "if ((res =~ \"MDaemon[- ]Webmail\" || res =~ \"Server\\s*:\\s*WDaemon\") && \"WorldClient.dll\" >< res) {"
	//		code = strings.Join(codeLines, "\n")
	//	}
	//	return code
	//})
}
