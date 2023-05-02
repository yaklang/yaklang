package yakvm

import (
	"bufio"
	"fmt"
	"strings"
	"yaklang/common/go-funk"
	"yaklang/common/log"
	"yaklang/common/yak/antlr4yak/yakvm/vmstack"

	"github.com/kataras/pio"
)

type VMPanicSignal struct {
	Info interface{}
}
type PanicInfo struct {
	code           *Code
	postionVerbose string
	codeReview     string
}

func newPanicInfo(code *Code, codeReview string) *PanicInfo {
	return &PanicInfo{code: code, codeReview: codeReview}
}
func (p *PanicInfo) SetPostionVerbose(s string) {
	p.postionVerbose = s
}
func (p *PanicInfo) String() string {
	if p.code.SourceCodeFilePath == nil {
		return p.postionVerbose
	}
	return fmt.Sprintf("File \"%s\", in %s", *p.code.SourceCodeFilePath, p.postionVerbose)
}

type VMPanic struct {
	contextInfos *vmstack.Stack
	data         interface{}
}

func NewVMPanic(i interface{}) *VMPanic {
	if err, ok := i.(error); ok {
		i = err.Error()
	}
	p := &VMPanic{vmstack.New(), i}
	return p
}

func (v *Frame) getCodeReview(sourceCode *string, code *Code, i *VMPanic) string {
	var codeReview string
	if sourceCode == nil || *sourceCode == "" && code != nil {
		return fmt.Sprintf("not found source code binding [start %v:%v   end %v:%v]", code.StartLineNumber, code.StartColumnNumber, code.EndLineNumber, code.EndColumnNumber)
	}
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("BUG: handling panic code review failed: %v", i)
			if code.EndColumnNumber > 0 || code.EndLineNumber > 0 {
				codeReview = fmt.Sprintf("panic in code: %v:%v - %v:%v: %v", code.StartLineNumber, code.StartColumnNumber, code.EndLineNumber, code.EndColumnNumber, i)
			} else {
				codeReview = ""
			}
		}
	}()
	scanner := bufio.NewScanner(strings.NewReader(*sourceCode))
	scanner.Split(bufio.ScanLines)
	flag := 0
	for nowLine := 1; scanner.Scan() && nowLine <= code.EndLineNumber; nowLine++ {
		text := scanner.Text()
		runeText := []rune(text)
		lenOfText := len(text)

		if nowLine == code.StartLineNumber {
			flag = 1 // 后面的行全红
			if code.EndColumnNumber >= lenOfText && lenOfText > 0 {
				codeReview += pio.Red(string(runeText[code.StartColumnNumber:]) + "\n")
				continue
			} else {
				// 右闭区间
				code.EndColumnNumber++
			}
			codeReview += text[:code.StartColumnNumber]
			if code.EndLineNumber == code.StartLineNumber {
				codeReview += pio.Red(string(runeText[code.StartColumnNumber:code.EndColumnNumber])) + string(runeText[code.EndColumnNumber:]) + "\n"
				break
			} else {
				codeReview += pio.Red(string(runeText[code.StartColumnNumber:]) + "\n")
			}

			continue
		}
		if nowLine == code.EndLineNumber {
			flag = 2 // 最后一行，影响范围的红
		}
		if flag == 1 {
			codeReview += pio.Red(text + "\n")
		}
		if flag == 2 {
			codeReview += pio.Red(string(runeText[:code.EndColumnNumber]))
			codeReview += string(runeText[code.EndColumnNumber:])
			break
		}
	}
	codeReview = strings.TrimSpace(codeReview)
	return codeReview
}

func (v *Frame) panic(i *VMPanic) {
	code := v.codes[v.codePointer]
	sourceCodeP := code.SourceCodePointer
	codeReview := v.getCodeReview(sourceCodeP, code, i)
	i.contextInfos.Push(newPanicInfo(code, codeReview))
	v.vm.panic(i)
}

func (v *VMPanic) Error() string {
	var source []string
	iinfo := v.contextInfos.Pop()
	for {
		if iinfo == nil {
			break
		}
		info := iinfo.(*PanicInfo)
		code := info.code
		codeReview := info.codeReview
		lineReview := ""
		if code.EndLineNumber == code.StartLineNumber {
			lineReview = fmt.Sprintf("--> %d", code.StartLineNumber)
		} else {
			lineReview = fmt.Sprintf("--> %d-%d", code.StartLineNumber, code.EndLineNumber)
		}
		if strings.Contains(codeReview, "\n") { // 多行显示
			lineReviewLen := len(lineReview) + 1
			codeReview = strings.ReplaceAll(codeReview, "\n", "\n"+strings.Repeat(" ", lineReviewLen))
			source = append(source, fmt.Sprintf(
				"%s\n%s %s",
				info.String(),
				pio.Green(lineReview),
				codeReview),
			)
		} else {
			source = append(source, fmt.Sprintf(
				"%s\n%s %s",
				info.String(),
				pio.Green(lineReview),
				codeReview),
			)
		}

		iinfo = v.contextInfos.Pop()
	}
	source = funk.Reverse(source).([]string)
	sources := strings.Join(source, "\n")
	return fmt.Sprintf("Panic Stack:\n%s\n\nYakVM Panic: %v", sources, v.GetData())
}

func (v *Frame) recover() *VMPanic {
	return v.vm.recover()
}

func (v *VMPanic) GetData() interface{} {
	if v == nil {
		return nil
	}
	return v.data
}

func (v *Frame) SetPanicInfo(ps ...*VMPanic) {
	if v == nil {
		return
	}
	vm := v
	for vm.parent != nil {
		vm = vm.parent
	}
	vm.panics = append(vm.panics, ps...)
}

//func (v *Frame) HandlePanic() {
//	if v == nil {
//		return
//	}
//	if v.parent == nil {
//		var buf bytes.Buffer
//		for _, i := range v.panics {
//			buf.WriteString(i.String())
//			buf.WriteByte('\n')
//		}
//		if buf.Len() > 0 {
//			panic(buf.String())
//		}
//	}
//}
