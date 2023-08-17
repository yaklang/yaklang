package dap

import (
	"errors"

	"github.com/google/shlex"
)

const (
	HelpInfo = `
h, help                          : show help info

`
	msgHelp = `show help info`
	// msgNext       = `step next`
	// msgIn         = `step into function`
	// msgOut        = `step out function`
	// msgRun        = `run until breakpoint`
	// msgWatch      = `set observe breakpoint that is triggered when <expr> is modified`
	// msgUnwatch    = `remove <expr> observe breakpoint`
	// msgObs        = `observe <expr>`
	// msgUnobs      = `un-observe <expr>`
	// msgShowobs    = `show all observe expressions`
	// msgBreakPoint = `set normal/conditional breakpoint in line <line>, set [condition] to set conditional breakpoint`
	// msgClear      = `clear breakpoint in line <line> or clear all breakpoints`
	// msgEnable     = `enable breakpoint in line <line> or enable all breakpoints`
	// msgDisable    = `disable breakpoint in line <line> or disable all breakpoints`
)

var (
	errNoCmd = errors.New("command not available")
	// errWatchNoExpr      = errors.New("watch/unwatch command need expression")
	// errObsNoExpr        = errors.New("obs/unobs command need expression")
	// errBreakPointNoLine = errors.New("breakpoint command need line number")
)

type cmdfunc func(args []string) (string, error)

type command struct {
	aliases []string
	helpMsg string
	cmdFn   cmdfunc
}

func debugCommands(ds *DebugSession) []command {
	return []command{
		{aliases: []string{"help", "h"}, cmdFn: ds.cmdHelp, helpMsg: msgHelp},
		// {aliases: []string{"in"}, cmdFn: ds.cmdIn, helpMsg: msgIn},
		// {aliases: []string{"out"}, cmdFn: ds.cmdOut, helpMsg: msgOut},
		// {aliases: []string{"next", "n"}, cmdFn: ds.cmdIn, helpMsg: msgNext},
		// {aliases: []string{"run", "r"}, cmdFn: ds.cmdRun, helpMsg: msgRun},
		// {aliases: []string{"watch"}, cmdFn: ds.cmdWatch, helpMsg: msgWatch},
		// {aliases: []string{"unwatch"}, cmdFn: ds.cmdUnWatch, helpMsg: msgUnwatch},
		// {aliases: []string{"obs"}, cmdFn: ds.cmdObs, helpMsg: msgObs},
		// {aliases: []string{"unobs"}, cmdFn: ds.cmdUnobs, helpMsg: msgUnobs},
		// {aliases: []string{"showobs"}, cmdFn: ds.cmdShowobs, helpMsg: msgShowobs},
		// {aliases: []string{"break", "b"}, cmdFn: ds.cmdBreakPoint, helpMsg: msgBreakPoint},
	}
}

func (ds *DebugSession) dbgCommand(cmdStr string) (string, error) {
	args, err := shlex.Split(cmdStr)
	if err != nil {
		return "", err
	}
	cmdname := args[0]
	for _, cmd := range debugCommands(ds) {
		for _, alias := range cmd.aliases {
			if alias == cmdname {
				return cmd.cmdFn(args[1:])
			}
		}
	}

	return "", errNoCmd
}

func (ds *DebugSession) cmdHelp(args []string) (string, error) {
	if len(args) > 0 {
		needHelpCmdName := args[0]
		for _, cmd := range debugCommands(ds) {
			for _, alias := range cmd.aliases {
				if alias == needHelpCmdName {
					return cmd.helpMsg, nil
				}
			}
		}
		return "", errNoCmd
	} else {
		return HelpInfo, nil
	}
}

// func (ds *DebugSession) cmdIn(args []string) (string, error) {
// 	err := ds.debugger.StepIn()
// 	return "", err
// }

// func (ds *DebugSession) cmdOut(args []string) (string, error) {
// 	err := ds.debugger.StepOut()
// 	return "", err
// }

// func (ds *DebugSession) cmdNext(args []string) (string, error) {
// 	err := ds.debugger.StepNext()
// 	return "", err
// }

// func (ds *DebugSession) cmdRun(args []string) (string, error) {
// 	ds.debugger.Continue()
// 	return "", nil
// }

// func (ds *DebugSession) cmdWatch(args []string) (string, error) {
// 	if len(args) > 0 {
// 		expr := args[0]
// 		err := ds.debugger.AddObserveBreakPoint(expr)
// 		return "", err
// 	} else {
// 		return "", errWatchNoExpr
// 	}
// }

// func (ds *DebugSession) cmdUnWatch(args []string) (string, error) {
// 	if len(args) > 0 {
// 		expr := args[0]
// 		err := ds.debugger.RemoveObserveBreakPoint(expr)
// 		return "", err
// 	} else {
// 		return "", errWatchNoExpr
// 	}
// }

// func (ds *DebugSession) cmdObs(args []string) (string, error) {
// 	if len(args) > 0 {
// 		expr := args[0]
// 		err := ds.debugger.AddObserveExpression(expr)
// 		return "", err
// 	} else {
// 		return "", errObsNoExpr
// 	}
// }

// func (ds *DebugSession) cmdUnobs(args []string) (string, error) {
// 	if len(args) > 0 {
// 		expr := args[0]
// 		err := ds.debugger.RemoveObserveExpression(expr)
// 		return "", err
// 	} else {
// 		return "", errObsNoExpr
// 	}
// }

// func (ds *DebugSession) cmdShowobs(args []string) (string, error) {
// 	var buf bytes.Buffer

// 	undefined := yakvm.GetUndefined()
// 	observeExprs := ds.debugger.GetAllObserveExpressions()
// 	for expr, v := range observeExprs {
// 		valueStr := v.String()
// 		if v == nil || v == undefined {
// 			valueStr = "nil"
// 		}
// 		fmt.Fprintf(&buf, "%s: %s\n", expr, valueStr)
// 	}
// 	fmt.Fprintln(&buf)
// 	return buf.String(), nil
// }

// func (ds *DebugSession) cmdBreakPoint(args []string) (string, error) {
// 	if len(args) > 0 {
// 		line := args[0]
// 		lineNumber, err := strconv.Atoi(line)
// 		if err != nil {
// 			return "", err
// 		}

// 		condtion := ""
// 		if len(args) > 2 && args[1] == "if" {
// 			condtion = args[2]
// 		}

// 		if _, err := ds.debugger.SetBreakPoint(lineNumber, condtion, ""); err != nil {
// 			return "", err
// 		}
// 	} else {
// 		return "", errBreakPointNoLine
// 	}
// 	return "", nil
// }
