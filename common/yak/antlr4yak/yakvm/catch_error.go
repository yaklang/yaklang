package yakvm

func (v *Frame) catchErrorRun(catchCodeIndex, id int) (err interface{}) {
	// 备份作用域，退出 try block 后恢复作用域，防止异常退出后作用域不正确
	scopeBackpack := v.scope

	// 退出Try的情况
	// 1. (OpExitCatchError)OpBreak
	// 2. (OpExitCatchError)OpContinue
	// 3. (OpExitCatchError)OpReturn
	// 4. panic
	// 5. 正常执行完try-block退出
	// 1/2情况不需要退出作用域，因为break和continue会自己处理作用域
	// 情况3需要手动复原作用域到try-catch-finally外
	// 情况4需要手动复原作用域到try-catch-finally
	// 情况5需要手动复原作用域到try-catch-finally
	defer func() {
		// 除1/2情况，都需要恢复到try-catch-finally作用域
		if !(v.codes[v.codePointer+1].Opcode == OpBreak || v.codes[v.codePointer+1].Opcode == OpContinue) {
			v.scope = scopeBackpack
			// 情况3需要退出try-catch-finally作用域
			if v.codes[v.codePointer+1].Opcode == OpReturn {
				v.ExitScope()
			}
		}

		//出现错误后为 err 赋值并跳转到 catch block
		if err != nil {
			v.codePointer = catchCodeIndex
			if id > 0 {
				NewValueRef(id).Assign(v, NewAutoValue(err))
			}
		}
	}()
	v.codePointer++
	v.continueExec()
	err = v.recover().GetData()
	return
}
