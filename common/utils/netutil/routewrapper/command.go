package routewrapper

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type CommandSpec struct {
	Name string
	Args []string
}

type CommandExecError struct {
	Cause   error
	Message string
}

func (err *CommandExecError) Error() (retval string) {
	if err.Cause == nil {
		retval = err.Message
	} else {
		retval = fmt.Sprintf("%s (cause: %s)", err.Message, err.Cause.Error())
	}
	return
}

func runCommand(cmd *exec.Cmd) (stdoutBuf []byte, stderrBuf []byte, err error) {
	wg := sync.WaitGroup{}
	stdout, _err := cmd.StdoutPipe()
	if _err != nil {
		err = &CommandExecError{_err, "Failed to obtain stdout pipe"}
		return
	}
	defer stdout.Close()
	stderr, _err := cmd.StderrPipe()
	if _err != nil {
		err = &CommandExecError{_err, "Failed to obtain stderr pipe"}
		return
	}
	defer stderr.Close()
	errs := make([]error, 0, 2)
	errsMtx := sync.Mutex{}
	wg.Add(1)
	go func(in io.Reader) {
		defer wg.Done()
		buf := make([]byte, 256)
		i := 0
		for {
			if i >= len(buf) {
				newBuf := make([]byte, len(buf)*2)
				copy(newBuf, buf)
				buf = newBuf
			}
			n, err := in.Read(buf[i:])
			i += n
			if err != nil {
				break
			}
		}
		buf = buf[0:i]
		if err != nil && err != io.EOF {
			errsMtx.Lock()
			errs = append(errs, err)
			errsMtx.Unlock()
		}
		stdoutBuf = buf
	}(stdout)
	wg.Add(1)
	go func(in io.Reader) {
		defer wg.Done()
		buf := make([]byte, 256)
		i := 0
		for {
			if i >= len(buf) {
				newBuf := make([]byte, len(buf)*2)
				copy(newBuf, buf)
				buf = newBuf
			}
			n, err := in.Read(buf[i:])
			i += n
			if err != nil {
				break
			}
		}
		buf = buf[0:i]
		if err != nil && err != io.EOF {
			errsMtx.Lock()
			errs = append(errs, err)
			errsMtx.Unlock()
		}
		stderrBuf = buf
	}(stderr)
	_err = cmd.Start()
	wg.Wait()
	if _err != nil {
		err = &CommandExecError{_err, fmt.Sprintf("Failed to execute command %s %s", cmd.Path, strings.Join(cmd.Args, " "))}
		return
	}
	_err = cmd.Wait()
	if _err != nil {
		err = &CommandExecError{_err, fmt.Sprintf("Failed to execute command %s %s", cmd.Path, strings.Join(cmd.Args, " "))}
		return
	}
	if len(errs) > 0 {
		err = &CommandExecError{_err, fmt.Sprintf("I/O error occurred during executing command %s %s", cmd.Path, strings.Join(cmd.Args, " "))}
		return
	}
	return
}

func (cmd CommandSpec) Run() ([]byte, []byte, error) {
	ocmd := exec.Command(cmd.Name, cmd.Args...)
	ctx, err := onBeforeCommandRun(ocmd)
	if err != nil {
		return nil, nil, err
	}
	defer onAfterCommandRun(ctx, ocmd)
	return runCommand(ocmd)
}

func (cmd CommandSpec) Clone() CommandSpec {
	args := make([]string, len(cmd.Args))
	copy(args, cmd.Args)
	return CommandSpec{
		Name: cmd.Name,
		Args: args,
	}
}
