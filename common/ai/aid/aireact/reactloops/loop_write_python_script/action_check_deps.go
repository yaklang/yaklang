package loop_write_python_script

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

const maxCheckAttempts = 3

var checkAndInstallDependencies = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"check_and_install_dependencies",
		"Check if all imported Python modules are available. If missing packages are found, this action will report them and you MUST use require_tool with tool_require_payload='bash' to install them. Do NOT call this action repeatedly if packages are still missing - use bash to install first, then call this again to verify.",
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("extra_packages",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Optional: additional package names to install that may not be directly detectable from import statements (e.g., 'python-dotenv' for 'import dotenv')")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			code := l.Get("full_python_code")
			if code == "" {
				return fmt.Errorf("no Python code found, write the script first using write_script")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			code := loop.Get("full_python_code")
			pythonCmd := loop.Get("python_command")
			pkgManager := loop.Get("pkg_manager")

			attemptCount, _ := strconv.Atoi(loop.Get("deps_check_attempts"))
			attemptCount++
			loop.Set("deps_check_attempts", strconv.Itoa(attemptCount))

			if attemptCount > maxCheckAttempts {
				r.AddToTimeline("deps_check_limit", fmt.Sprintf("Dependency check called %d times (limit: %d). Skipping further checks.", attemptCount, maxCheckAttempts))
				loop.Set("deps_checked", "true")
				loop.Set("deps_installed", "true")
				op.Feedback(fmt.Sprintf(
					"[WARNING] Dependency check has been called %d times. Maximum attempts (%d) exceeded.\n"+
						"Stop retrying. Use `directly_answer` to report the dependency issue to the user, "+
						"or use `require_tool` with `tool_require_payload: \"bash\"` to manually handle installation.",
					attemptCount, maxCheckAttempts,
				))
				return
			}

			if pythonCmd == "" {
				op.Feedback("Python is not available. Cannot check dependencies.")
				loop.Set("deps_checked", "true")
				loop.Set("deps_installed", "true")
				return
			}

			imports := parseImports(code)
			extraPkgs := action.GetStringSlice("extra_packages")
			for _, pkg := range extraPkgs {
				pkg = strings.TrimSpace(pkg)
				if pkg != "" {
					imports = append(imports, pkg)
				}
			}

			imports = filterStdlib(imports)
			if len(imports) == 0 {
				log.Infof("no third-party imports detected")
				r.AddToTimeline("deps_check", "No third-party dependencies detected")
				loop.Set("deps_checked", "true")
				loop.Set("deps_installed", "true")
				op.Feedback("No third-party dependencies detected. All imports are from the standard library.")
				return
			}

			log.Infof("checking %d potential third-party imports: %v", len(imports), imports)

			var missing []string
			var installed []string
			for _, mod := range imports {
				if !isModuleInstalled(pythonCmd, mod) {
					missing = append(missing, mod)
				} else {
					installed = append(installed, mod)
				}
			}

			loop.Set("deps_checked", "true")

			if len(missing) == 0 {
				r.AddToTimeline("deps_check", fmt.Sprintf("All %d dependencies are installed", len(imports)))
				loop.Set("deps_installed", "true")
				loop.Set("deps_check_attempts", "0")
				op.Feedback(fmt.Sprintf("All %d third-party dependencies are installed: %s", len(imports), strings.Join(imports, ", ")))
				return
			}

			loop.Set("deps_installed", "false")
			log.Infof("missing dependencies: %v", missing)
			r.AddToTimeline("deps_missing", fmt.Sprintf("Missing dependencies: %s", strings.Join(missing, ", ")))

			var installHint strings.Builder
			installHint.WriteString(fmt.Sprintf("Missing %d package(s): %s\n", len(missing), strings.Join(missing, ", ")))
			if len(installed) > 0 {
				installHint.WriteString(fmt.Sprintf("Already installed: %s\n", strings.Join(installed, ", ")))
			}
			installHint.WriteString("\n")
			installHint.WriteString("=== ACTION REQUIRED ===\n")
			installHint.WriteString("You MUST use `require_tool` to request the `bash` tool, then run the install command.\n")
			installHint.WriteString("Do NOT call `check_and_install_dependencies` again until you have installed the packages.\n\n")

			if pkgManager == "uv" {
				installHint.WriteString("Suggested install commands (try in order):\n")
				installHint.WriteString(fmt.Sprintf("  1. uv pip install --system %s\n", strings.Join(missing, " ")))
				installHint.WriteString(fmt.Sprintf("  2. uv pip install %s\n", strings.Join(missing, " ")))
				installHint.WriteString(fmt.Sprintf("  3. pip3 install --user %s\n", strings.Join(missing, " ")))
				installHint.WriteString(fmt.Sprintf("  4. pip install --user %s\n", strings.Join(missing, " ")))
			} else if pkgManager != "" {
				installHint.WriteString("Suggested install commands (try in order):\n")
				installHint.WriteString(fmt.Sprintf("  1. %s install --user %s\n", pkgManager, strings.Join(missing, " ")))
				installHint.WriteString(fmt.Sprintf("  2. %s install %s\n", pkgManager, strings.Join(missing, " ")))
			} else {
				installHint.WriteString("No package manager detected. Try:\n")
				installHint.WriteString(fmt.Sprintf("  1. pip3 install --user %s\n", strings.Join(missing, " ")))
				installHint.WriteString(fmt.Sprintf("  2. pip install --user %s\n", strings.Join(missing, " ")))
			}
			installHint.WriteString("\nIf all install commands fail, create a virtual environment first:\n")
			installHint.WriteString("  uv venv .venv && source .venv/bin/activate && uv pip install " + strings.Join(missing, " ") + "\n")
			installHint.WriteString("\nAfter successful installation, call `check_and_install_dependencies` again to verify.\n")

			op.DisallowNextLoopExit()
			op.Feedback(installHint.String())
		},
	)
}

var importRegex = regexp.MustCompile(`(?m)^\s*(?:import|from)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)

func parseImports(code string) []string {
	matches := importRegex.FindAllStringSubmatch(code, -1)
	seen := make(map[string]bool)
	var result []string
	for _, match := range matches {
		if len(match) > 1 {
			mod := match[1]
			if !seen[mod] {
				seen[mod] = true
				result = append(result, mod)
			}
		}
	}
	return result
}

var pythonStdlib = map[string]bool{
	"abc": true, "aifc": true, "argparse": true, "array": true, "ast": true,
	"asynchat": true, "asyncio": true, "asyncore": true, "atexit": true,
	"base64": true, "bdb": true, "binascii": true, "binhex": true,
	"bisect": true, "builtins": true, "bz2": true, "calendar": true,
	"cgi": true, "cgitb": true, "chunk": true, "cmath": true, "cmd": true,
	"code": true, "codecs": true, "codeop": true, "collections": true,
	"colorsys": true, "compileall": true, "concurrent": true,
	"configparser": true, "contextlib": true, "contextvars": true,
	"copy": true, "copyreg": true, "cProfile": true, "crypt": true,
	"csv": true, "ctypes": true, "curses": true, "dataclasses": true,
	"datetime": true, "dbm": true, "decimal": true, "difflib": true,
	"dis": true, "distutils": true, "doctest": true, "email": true,
	"encodings": true, "enum": true, "errno": true, "faulthandler": true,
	"fcntl": true, "filecmp": true, "fileinput": true, "fnmatch": true,
	"formatter": true, "fractions": true, "ftplib": true, "functools": true,
	"gc": true, "getopt": true, "getpass": true, "gettext": true,
	"glob": true, "grp": true, "gzip": true, "hashlib": true, "heapq": true,
	"hmac": true, "html": true, "http": true, "idlelib": true, "imaplib": true,
	"imghdr": true, "imp": true, "importlib": true, "inspect": true,
	"io": true, "ipaddress": true, "itertools": true, "json": true,
	"keyword": true, "lib2to3": true, "linecache": true, "locale": true,
	"logging": true, "lzma": true, "mailbox": true, "mailcap": true,
	"marshal": true, "math": true, "mimetypes": true, "mmap": true,
	"modulefinder": true, "multiprocessing": true, "netrc": true,
	"nis": true, "nntplib": true, "numbers": true, "operator": true,
	"optparse": true, "os": true, "ossaudiodev": true, "parser": true,
	"pathlib": true, "pdb": true, "pickle": true, "pickletools": true,
	"pipes": true, "pkgutil": true, "platform": true, "plistlib": true,
	"poplib": true, "posix": true, "posixpath": true, "pprint": true,
	"profile": true, "pstats": true, "pty": true, "pwd": true,
	"py_compile": true, "pyclbr": true, "pydoc": true, "queue": true,
	"quopri": true, "random": true, "re": true, "readline": true,
	"reprlib": true, "resource": true, "rlcompleter": true, "runpy": true,
	"sched": true, "secrets": true, "select": true, "selectors": true,
	"shelve": true, "shlex": true, "shutil": true, "signal": true,
	"site": true, "smtpd": true, "smtplib": true, "sndhdr": true,
	"socket": true, "socketserver": true, "spwd": true, "sqlite3": true,
	"ssl": true, "stat": true, "statistics": true, "string": true,
	"stringprep": true, "struct": true, "subprocess": true, "sunau": true,
	"symtable": true, "sys": true, "sysconfig": true, "syslog": true,
	"tabnanny": true, "tarfile": true, "telnetlib": true, "tempfile": true,
	"termios": true, "test": true, "textwrap": true, "threading": true,
	"time": true, "timeit": true, "tkinter": true, "token": true,
	"tokenize": true, "tomllib": true, "trace": true, "traceback": true,
	"tracemalloc": true, "tty": true, "turtle": true, "turtledemo": true,
	"types": true, "typing": true, "unicodedata": true, "unittest": true,
	"urllib": true, "uu": true, "uuid": true, "venv": true, "warnings": true,
	"wave": true, "weakref": true, "webbrowser": true, "winreg": true,
	"winsound": true, "wsgiref": true, "xdrlib": true, "xml": true,
	"xmlrpc": true, "zipapp": true, "zipfile": true, "zipimport": true,
	"zlib": true, "_thread": true, "__future__": true,
}

func filterStdlib(modules []string) []string {
	var result []string
	for _, mod := range modules {
		if !pythonStdlib[mod] {
			result = append(result, mod)
		}
	}
	return result
}

func isModuleInstalled(pythonCmd string, moduleName string) bool {
	cmd := exec.Command(pythonCmd, "-c", fmt.Sprintf("import %s", moduleName))
	return cmd.Run() == nil
}
