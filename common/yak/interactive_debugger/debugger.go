package debugger

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type InteractiveDebugger struct {
	prompt *Prompt
}

func NewInteractiveDebugger() *InteractiveDebugger {
	return &InteractiveDebugger{
		prompt: NewPrompt(">>>"),
	}
}

func (i *InteractiveDebugger) Init() func(g *yakvm.Debugger) {
	return func(g *yakvm.Debugger) {
		// 在第一个opcode执行的时候开始回调
		// g.SetBreakPoint(true, 1)
		fmt.Printf("Yak version: %s\nType 'help' for help info.\n", consts.GetYakVersion())
		g.Callback()
	}
}

func (i *InteractiveDebugger) GetPrettySourceCode(g *yakvm.Debugger, start int, ends ...int) string {
	var (
		buf    strings.Builder
		prefix string
	)

	sourceCodeLines := g.SourceCodeLines()
	breakpoints := g.Breakpoints()
	currentLine := g.CurrentLine()
	end := len(sourceCodeLines) - 1
	if len(ends) > 0 {
		end = ends[0]
	}

	if start < 0 {
		start = 0
	} else if start >= len(sourceCodeLines) {
		start = len(sourceCodeLines) - 1
	}
	if end < 0 {
		end = 0
	} else if end >= len(sourceCodeLines) {
		end = len(sourceCodeLines) - 1
	}

	for index := start; index <= end; index++ {
		prefix = "  "
		for _, bp := range breakpoints {
			if bp.LineIndex-1 == index {
				if bp.On {
					prefix = "**"
					if bp.ConditionCode != "" {
						prefix = "-*"
					}
				} else {
					prefix = "--"
				}
				break
			}
		}
		if currentLine-1 == index {
			prefix = "->"
		}

		buf.WriteString(fmt.Sprintf("%s %4d │ %s\n", prefix, index+1, sourceCodeLines[index]))
	}

	return buf.String()
}

func (i *InteractiveDebugger) CallBack() func(g *yakvm.Debugger) {
	return func(g *yakvm.Debugger) {
		// 显示回调信息
		if desc := g.Description(); desc != "" && g.CurrentLine() > 1 {
			fmt.Println(desc)
			g.ResetDescription()
		}

		for {
			input, err := i.prompt.Input()
			if err != nil {
				if err == io.EOF {
					fmt.Println("\nInteractive debugger exit")
					os.Exit(0)
				}
				fmt.Printf("Interactive debugger Input error: %v\n", err)
				continue
			}
			if input == "" {
				continue
			}

			commands := strings.Split(input, " ")
			switch commands[0] {
			case "h", "help":
				fmt.Printf(HelpInfo)
			case "exit":
				fmt.Println("Interactive debugger exit")
				os.Exit(0)
			case "l", "list":
				var err error
				lineNumber := g.CurrentLine()
				if len(commands) == 2 {
					lineNumber, err = strconv.Atoi(commands[1])
					if err != nil {
						fmt.Printf("Interactive debugger list error: %v\n", err)
						continue
					}
				}
				fmt.Printf("%s\n", i.GetPrettySourceCode(g, lineNumber-5, lineNumber+5))
			case "la":
				fmt.Printf("%s\n", i.GetPrettySourceCode(g, 0))
			case "so":
				lineNumber := g.CurrentLine()
				if len(commands) == 2 {
					lineNumber, err = strconv.Atoi(commands[1])
					if err != nil {
						fmt.Printf("Interactive debugger show opcode error: %v\n", err)

						continue
					}
				}
				showOpcodes := make([]*yakvm.Code, 0)
				_, startCodeIndex, _ := g.GetLineFirstCode(lineNumber)
				for _, code := range g.Codes() {
					if code.StartLineNumber == lineNumber {
						showOpcodes = append(showOpcodes, code)
					}
				}

				if startCodeIndex == -1 {
					fmt.Printf("Interactive debugger show opcode error: no opcode(s)\n")
					continue
				}

				fmt.Println(yakvm.OpcodesString(showOpcodes))
			case "sao":
				codes := g.Codes()
				fmt.Println(yakvm.OpcodesString(codes))
			case "eval":
				if len(commands) < 2 {
					fmt.Println()
					fmt.Printf("Interactive debugger eval code error: eval command need code\n")
					continue
				}
				code := input[5:]

				value, err := g.EvalExpression(code)
				if err != nil {
					fmt.Printf("Interactive debugger eval code error: %v\n", err)
					continue
				}
				fmt.Printf("%s\n", value)
			case "n", "next":
				g.StepNext()
				return
			case "in":
				g.StepIn()
				return
			case "out":
				err := g.StepOut()
				if err != nil {
					fmt.Printf("Interactive debugger step out error: %v\n", err)
					continue
				}
				return
			case "r", "run":
				return
			case "watch":
				if len(commands) < 2 {
					fmt.Printf("Interactive debugger watch error: watch command need expression\n")
					continue
				}
				expr := strings.Join(commands[1:], " ")
				if err := g.AddObserveBreakPoint(expr); err != nil {
					fmt.Printf("Interactive debugger watch error: %v\n", err)
				}
			case "unwatch":
				if len(commands) < 2 {
					fmt.Printf("Interactive debugger unwatch error: unwatch command need expression\n")
					continue
				}
				expr := strings.Join(commands[1:], " ")
				if err := g.RemoveObserveBreakPoint(expr); err != nil {
					fmt.Printf("Interactive debugger unwatch error: %v\n", err)
				}
			case "obs":
				if len(commands) < 2 {
					fmt.Printf("Interactive debugger obs error: obs command need expression\n")
					continue
				}
				expr := strings.Join(commands[1:], " ")
				if err := g.AddObserveExpression(expr); err != nil {
					fmt.Printf("Interactive debugger obs error: %v\n", err)
				}
			case "unobs":
				if len(commands) < 2 {
					fmt.Printf("Interactive debugger unobs error: unobs command need expression\n")
					continue
				}
				expr := strings.Join(commands[1:], " ")
				if err := g.RemoveObserveExpression(expr); err != nil {
					fmt.Printf("Interactive debugger unobs error: %v\n", err)
				}
			case "showobs":
				undefined := yakvm.GetUndefined()
				observeExprs := g.GetAllObserveExpressions()
				for expr, v := range observeExprs {
					valueStr := v.String()
					if v == nil || v == undefined {
						valueStr = "nil"
					}
					fmt.Printf("%s: %s\n", expr, valueStr)
				}
				fmt.Println()
			case "b", "break", "breakpoint":
				if len(commands) < 2 {
					fmt.Printf("Interactive debugger set breakpoint error: breakpoint command need line number\n")
					continue
				}

				line := commands[1]
				lineNumber, err := strconv.Atoi(line)
				if err != nil {
					fmt.Printf("Interactive debugger set breakpoint error: %v\n", err)
					continue
				}

				if len(commands) > 3 && commands[2] == "if" {
					condtionCode := strings.Join(commands[3:], " ")
					if err := g.SetCondtionalBreakPoint(lineNumber, condtionCode); err != nil {
						fmt.Printf("Interactive debugger set breakpoint error: %v\n", err)
					}
				} else if err := g.SetBreakPoint(false, lineNumber); err != nil {
					fmt.Printf("Interactive debugger set breakpoint error: %v\n", err)
				}
			case "clear":
				if len(commands) < 2 {
					g.ClearAllBreakPoints()
					continue
				}
				line := commands[1]
				lineNumber, err := strconv.Atoi(line)
				if err != nil {
					fmt.Printf("Interactive debugger clear breakpoint error: %v\n", err)
					continue
				}
				g.ClearBreakpointsInLine(lineNumber)
			case "enable":
				if len(commands) < 2 {
					g.EnableAllBreakPoints()
					continue
				}
				line := commands[1]
				lineNumber, err := strconv.Atoi(line)
				if err != nil {
					fmt.Printf("Interactive debugger enable breakpoint error: %v\n", err)
					continue
				}
				g.EnableBreakpointsInLine(lineNumber)
			case "disable":
				if len(commands) < 2 {
					g.DisableAllBreakPoints()
					continue
				}
				line := commands[1]
				lineNumber, err := strconv.Atoi(line)
				if err != nil {
					fmt.Printf("Interactive debugger disable breakpoint error: %v\n", err)
					continue
				}
				g.DisableBreakpointsInLine(lineNumber)
			case "p", "print":
				scope := g.VM().CurrentFM().CurrentScope()
				if len(commands) < 2 {
					scopeKVs := scope.GetAllNameAndValueInAllScopes()
					if len(scopeKVs) == 0 {
						fmt.Printf("Interactive debugger print variable error: no variable in current scope\n")
						continue
					}
					scopeKeys := make([]string, 0, len(scopeKVs))
					for k := range scopeKVs {
						scopeKeys = append(scopeKeys, k)
					}
					sort.Strings(scopeKeys)

					fmt.Println("------------------------------")
					for _, name := range scopeKeys {
						fmt.Printf("%s: %v\n", name, scopeKVs[name].Value)
					}
					fmt.Println("------------------------------")
				} else {
					varName := commands[1]
					if value, ok := scope.GetValueByName(varName); ok {
						fmt.Printf("%s: %v\n", varName, value)
					} else {
						fmt.Printf("Interactive debugger print variable error: no such variable: %s\n", varName)
					}
				}
			default:
				fmt.Printf("Unknown command: %s\n", commands[0])
			}
		}

	}
}
