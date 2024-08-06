package yakvm

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

type VMPanicSignal struct {
	Info           interface{}
	AdditionalInfo interface{}
}
type PanicInfo struct {
	code            *Code
	positionVerbose string
	codeReview      string
}

func newPanicInfo(code *Code, codeReview string) *PanicInfo {
	return &PanicInfo{code: code, codeReview: codeReview}
}

func (p *PanicInfo) SetPositionVerbose(s string) {
	p.positionVerbose = s
}

func (p *PanicInfo) String() string {
	if p.code.SourceCodeFilePath == nil {
		return p.positionVerbose
	}
	return fmt.Sprintf("File \"%s\", in %s", *p.code.SourceCodeFilePath, p.positionVerbose)
}

type VMPanic struct {
	contextInfos *vmstack.Stack
	data         interface{}
}

func NewVMPanic(i interface{}) *VMPanic {
	if err, ok := i.(error); ok {
		i = err.Error()
	}
	utils.Debug(func() {
		utils.PrintCurrentGoroutineRuntimeStack()
	})
	p := &VMPanic{vmstack.New(), i}
	return p
}

func (v *VMPanic) GetData() interface{} {
	if v == nil {
		return nil
	}
	return v.data
}

func (v *VMPanic) GetDataDescription() string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%#v", v.data)
}

func IsVMPanic(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*VMPanic)
	return ok
}

func (v *Frame) getCodeReview(sourceCode *string, code *Code, i *VMPanic) string {
	var codeReview string
	var codeOrigin string
	if sourceCode != nil {
		codeOrigin = *sourceCode
	} else {
		if v.originCode != "" {
			codeOrigin = v.originCode
		}
	}
	if codeOrigin == "" && code != nil {
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

	editor := memedit.NewMemEditor(codeOrigin)
	codeReview = editor.GetTextContextWithPrompt(memedit.NewRange(memedit.NewPosition(code.StartLineNumber, code.StartColumnNumber), memedit.NewPosition(code.EndLineNumber, code.EndColumnNumber)), 3)
	codeReview = strings.TrimSpace(codeReview)
	// scanner := bufio.NewScanner(strings.NewReader(codeOrigin))
	// scanner.Split(bufio.ScanLines)
	// flag := 0
	// for nowLine := 1; scanner.Scan() && nowLine <= code.EndLineNumber; nowLine++ {
	// 	text := scanner.Text()
	// 	runeText := []rune(text)
	// 	lenOfText := len(runeText)

	// 	if nowLine == code.StartLineNumber {
	// 		flag = 1 // 后面的行全红
	// 		if code.EndColumnNumber >= lenOfText && lenOfText > 0 {
	// 			codeReview += pio.Red(string(runeText[code.StartColumnNumber:]) + "\n")
	// 			continue
	// 		} else {
	// 			// 右闭区间
	// 			code.EndColumnNumber++
	// 		}
	// 		codeReview += string(runeText[:code.StartColumnNumber])
	// 		if code.EndLineNumber == code.StartLineNumber {
	// 			codeReview += pio.Red(string(runeText[code.StartColumnNumber:code.EndColumnNumber])) + string(runeText[code.EndColumnNumber:]) + "\n"
	// 			break
	// 		} else {
	// 			codeReview += pio.Red(string(runeText[code.StartColumnNumber:]) + "\n")
	// 		}

	// 		continue
	// 	}
	// 	if nowLine == code.EndLineNumber {
	// 		flag = 2 // 最后一行，影响范围的红
	// 	}
	// 	if flag == 1 {
	// 		codeReview += pio.Red(text + "\n")
	// 	}
	// 	if flag == 2 {
	// 		codeReview += pio.Red(string(runeText[:code.EndColumnNumber]))
	// 		codeReview += string(runeText[code.EndColumnNumber:])
	// 		break
	// 	}
	// }
	// codeReview = strings.TrimSpace(codeReview)
	return codeReview
}

func (v *Frame) panic(i *VMPanic) {
	code := v.codes[v.codePointer]
	sourceCodeP := code.SourceCodePointer
	codeReview := v.getCodeReview(sourceCodeP, code, i)
	i.contextInfos.Push(newPanicInfo(code, codeReview))
	v.coroutine.lastPanic = i
}

func (v *VMPanic) Error() string {
	var source []string
	for i := 0; i < v.contextInfos.Len(); i++ {
		iinfo := v.contextInfos.PeekN(i)
		if iinfo == nil {
			break
		}
		info := iinfo.(*PanicInfo)
		codeReview := info.codeReview

		if strings.Contains(codeReview, "\n") { // 多行显示
			source = append(source, fmt.Sprintf(
				"%s\n%s",
				info.String(),
				codeReview),
			)
		} else {
			source = append(source, fmt.Sprintf(
				"%s\n%s",
				info.String(),
				codeReview),
			)
		}
	}
	source = funk.Reverse(source).([]string)
	sources := strings.Join(source, "\n")
	return fmt.Sprintf("Panic Stack:\n%s\n\nYakVM Panic: %v", sources, v.GetData())
}

func (v *Frame) recover() *VMPanic {
	lastPanic := v.coroutine.lastPanic
	v.coroutine.lastPanic = nil
	return lastPanic
}

//func (v *Frame) SetPanicInfo(ps ...*VMPanic) {
//	if v == nil {
//		return
//	}
//	vm := v
//	for vm.parent != nil {
//		vm = vm.parent
//	}
//	vm.panics = append(vm.panics, ps...)
//}

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
