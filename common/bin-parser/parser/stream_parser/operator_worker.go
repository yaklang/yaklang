package stream_parser

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

const (
	bridgeWorkerIdleLimit     = 32
	bridgeWorkerProgramLimit  = 16
	bridgeOperatorSourceLimit = 512
)

// Only idle workers are retained. A nested operator never waits for its parent
// to release a worker; extra concurrent leases are created and discarded when
// the idle budget is full. No engine is executed by two callers at once.
var bridgeWorkers = make(chan *bridgeWorker, bridgeWorkerIdleLimit)

type bridgeProgram struct {
	source  string
	symbols *yakvm.SymbolTable
	codes   []*yakvm.Code
}

type bridgeWorker struct {
	engine       *antlr4yak.Engine
	invocation   operatorInvocation
	emptySymbols *yakvm.SymbolTable
	programs     []bridgeProgram // exclusive, small LRU; most recently used first
}

func newBridgeWorker() *bridgeWorker {
	w := &bridgeWorker{engine: antlr4yak.New(), emptySymbols: yakvm.NewSymbolTable()}
	w.engine.GetVM().GetConfig().SetSuppressPanicDebugStack(true)
	w.engine.ImportLibs(w.invocation.library())
	return w
}

func (w *bridgeWorker) program(source string) (*bridgeProgram, error) {
	for i := range w.programs {
		if w.programs[i].source == source {
			p := w.programs[i]
			copy(w.programs[1:i+1], w.programs[:i])
			w.programs[0] = p
			return &w.programs[0], nil
		}
	}
	artifact, err := loadOperatorProgram(source)
	if err != nil {
		return nil, err
	}
	decoder := yakvm.NewCodesMarshaller()
	decoder.SetSourceCode(source)
	symbols, codes, err := decoder.Unmarshal(artifact)
	if err != nil {
		return nil, err
	}
	bindRuleProgramSource(codes, &source)
	if len(w.programs) < bridgeWorkerProgramLimit {
		w.programs = append(w.programs, bridgeProgram{})
	}
	copy(w.programs[1:], w.programs[:len(w.programs)-1])
	w.programs[0] = bridgeProgram{source: source, symbols: symbols, codes: codes}
	return &w.programs[0], nil
}

func (w *bridgeWorker) execute(ctx context.Context, source string, node *base.Node, operator func(*base.Node) (func(bool), error), modes []string, results ...*bridgeCallResult) (err error) {
	// Clear packet references and root variables after success, panic and
	// cancellation alike. Programs contain only immutable scalar literals.
	defer func() {
		w.invocation = operatorInvocation{}
		w.engine.GetVM().SetSymboltable(w.emptySymbols)
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	p, err := w.program(source)
	if err != nil {
		// Preparation is optional, and occurs before executing any callback.
		if len(results) > 0 && results[0] != nil {
			invocation := &operatorInvocation{node: node, operator: operator, modes: modes, bridgeResult: results[0]}
			engine := antlr4yak.New()
			engine.GetVM().GetConfig().SetSuppressPanicDebugStack(true)
			engine.ImportLibs(invocation.library())
			return evalOperatorProgram(ctx, engine, source)
		}
		return execFreshOperator(node, source, operator, modes)
	}
	w.invocation = operatorInvocation{node: node, operator: operator, modes: modes}
	if len(results) > 0 {
		w.invocation.bridgeResult = results[0]
	}
	w.engine.GetVM().SetSymboltable(p.symbols)
	// Execute the original program with its original source locations. No
	// generated wrapper, shortened result or native replacement of Yak errors.
	return w.engine.GetVM().ExecYakCode(ctx, source, p.codes, yakvm.None)
}

func execBridgeOperator(node *base.Node, source string, operator func(*base.Node) (func(bool), error), modes []string) error {
	return execBridgeOperatorResult(node, source, operator, modes, nil)
}
func execBridgeOperatorResult(node *base.Node, source string, operator func(*base.Node) (func(bool), error), modes []string, result *bridgeCallResult) error {
	var w *bridgeWorker
	select {
	case w = <-bridgeWorkers:
	default:
		w = newBridgeWorker()
	}
	defer func() {
		select {
		case bridgeWorkers <- w:
		default:
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return w.execute(ctx, source, node, operator, modes, result)
}

// This is a deliberately closed grammar, NOT an arbitrary Yak purity analysis.
// The exact two-statement bridge can neither retain a callback nor modify a
// library, spawn work, create mutable literals, or introduce dynamic symbols.
// Any other source (including equivalent formatting outside this grammar) goes
// through the ordinary evaluator with fresh runtime state.
func reusableBridgeOperator(source string) bool {
	if len(source) > bridgeOperatorSourceLimit {
		return false
	}
	call, check, ok := strings.Cut(strings.TrimSpace(source), "\n")
	if !ok || strings.TrimSpace(check) != "if err != nil { panic(err) }" {
		return false
	}
	call = strings.TrimSpace(call)
	if !strings.HasPrefix(call, "err = ") || !strings.HasSuffix(call, ")") {
		return false
	}
	name, args, ok := strings.Cut(call[len("err = "):len(call)-1], "(")
	if !ok || !reusableBridgeName(name) {
		return false
	}
	if args == "" {
		return true
	}
	// Current bridges take at most two literal scalar arguments.
	for count := 0; ; count++ {
		arg, rest, more := strings.Cut(args, ",")
		if count >= 2 || !immutableBridgeArgument(strings.TrimSpace(arg)) {
			return false
		}
		if !more {
			return true
		}
		args = rest
	}
}

func immutableBridgeArgument(arg string) bool {
	if arg == "true" || arg == "false" {
		return true
	}
	if len(arg) >= 2 && arg[0] == '"' && arg[len(arg)-1] == '"' {
		for _, c := range arg[1 : len(arg)-1] {
			// Excludes escapes, interpolation, nested calls and delimiters.
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
				return false
			}
		}
		return true
	}
	if len(arg) == 0 {
		return false
	}
	for _, c := range arg {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func reusableBridgeName(name string) bool {
	switch name {
	case "parseMemcachedFields",
		"parseCassandraFields",
		"parseTNSFields",
		"parseTDSFields",
		"parseLDAPFields", "parseSMB3TransformFields",
		"parsePostgreSQLFields",
		"parseMySQLFields",
		"parseIMAPFields",
		"parsePOP3Fields",
		"parseSMTPFields",
		"parseMQTTFields",
		"parseKerberosFields",
		"parseHTTP2Fields",
		"parseSSHPlaintextPacket",
		"parseX509CertificateDERPublicKey",
		"parseX509CertificateDERExtensions",
		"parseX509CertificateDER",
		"parseTLSChangeCipherSpecRecord",
		"parseTLS12CertificateAuth",
		"parseTLS12KeyExchange",
		"parseTLS12ControlHandshake",
		"parseTLSCertificateHandshake",
		"parseTLSServerHello",
		"parseZlibJSONRecord",
		"parseKCP",
		"parseSMTPReply",
		"parseGQUIC35",
		"parseBrowserMailslotDatagram",
		"parseEtcdVersionRecord",
		"parseS3SignatureV4Request",
		"parseKubernetesAPIRequest",
		"parseWinRMRecord",
		"parseGSSAPIToken",
		"parseSMB3Negotiate",
		"parseRMIRecord",
		"parseXTPMessage",
		"parseH225Message",
		"parseMegacoMessage",
		"parseZigbeeFrame",
		"parseLATMessage",
		"parseGIOPMessage",
		"parseSteamDiscovery":
		return true
	default:
		return false
	}
}
