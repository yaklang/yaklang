package ssa

// vm buildin function
var buildin = make(map[string]*Function)

func init() {
	// print(...any) nil
	buildin["print"] = &Function{
		name: "print",
		user: []User{},
		// param
		ParamTyp: []Types{
			// any
		},
		hasEllipsis: true,
		// return
		ReturnTyp: []Types{
			// nil
		},
	}
}
