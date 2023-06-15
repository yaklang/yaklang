package antlr4nasl

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
