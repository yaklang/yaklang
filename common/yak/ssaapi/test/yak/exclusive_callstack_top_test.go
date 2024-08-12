package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_CallStack_Normal_Parameter(t *testing.T) {
	t.Run("test level1", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return i
		}
		a = f(333333)
		`,
			ssatest.CheckTopDef_Equal("a", []string{"333333"}),
		)
	})
	t.Run("test external function", func(t *testing.T) {
		ssatest.Check(t, `a=f(333)`,
			ssatest.CheckTopDef_Equal("a", []string{"333", "Function-f"}),
			ssaapi.WithExternValue(map[string]any{
				"f": func(i int) int { return i },
			}))
	})
	t.Run("test undefined function", func(t *testing.T) {
		ssatest.Check(t, `a=f(333)`,
			ssatest.CheckTopDef_Equal("a", []string{"Undefined-f", "333"}),
		)
	})

	t.Run("test level1 object", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return {
				"i": i,
			}
		}
		obj = f(333333)
		a = obj.i
		`,
			ssatest.CheckTopDef_Equal("a", []string{"333333"}),
		)
	})

	t.Run("test level1 class", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			this =  {
				"i": i,
				// this will be set a class-blue-print
				"set": (i)=>{this.i = i}, 
			}
			return this
		}
		obj = f(333333)
		a = obj.i
		`,
			ssatest.CheckTopDef_Equal("a", []string{"333333"}),
		)
	})

	t.Run("test level1 php class", func(t *testing.T) {
		ssatest.Check(t, `
		<?php
		class A {
			public $i;
			public function __construct($i) {
				$this->i = $i;
			}
		}
		$obj  = new A(333333);
		$a = $obj->i;
		`,
			ssatest.CheckTopDef_Equal("$a", []string{"333333"}),
			ssaapi.WithLanguage(ssaapi.PHP),
		)
	})

	t.Run("test level2 simple", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return () => {
				return i 
			} 
		}
		f1 = f(333333)
		a = f1()
		`,
			ssatest.CheckTopDef_Equal("a", []string{"333333"}),
		)
	})

	t.Run("test level2", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return (j) => {
				return j + i
			} 
		}
		f1 = f(333333)
		a = f1(444444)
		`,
			ssatest.CheckTopDef_Equal("a", []string{"333333", "444444"}),
		)
	})

	t.Run("test level2 object", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return (j) => {
				return {
					"i": j + i,
				}
			} 
		}
		f1 = f(333333)
		obj = f1(444444)
		a = obj.i
		`,
			ssatest.CheckTopDef_Equal("a", []string{"333333", "444444"}),
		)
	})

	t.Run("test level3 test call-stack", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return (j) => {
				return (k) => {
					return j + i
				}
			} 
		}
		f1 = f(333333)
		f2 = f1(444444)
		a = f2(555555)
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444"}),
		)
	})

	t.Run("test level3", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return (j) => {
				return (k) => {
					return k + j + i
				}
			} 
		}
		f1 = f(333333)
		f2 = f1(444444)
		a = f2(555555)
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444", "555555"}),
		)
	})

	t.Run("test level3 object", func(t *testing.T) {
		ssatest.Check(t, `
		f = (i) => {
			return (j) => {
				return (k) => {
					return {
						"i": k + j + i
					}
				}
			} 
		}
		f1 = f(333333)
		f2 = f1(444444)
		obj = f2(555555)
		a = obj.i
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444", "555555"}),
		)
	})
}

func Test_CallStack_Normal_FreeValue(t *testing.T) {
	t.Run("test level1", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			return i
		}
		a = f()
		`, ssatest.CheckTopDef_Equal("a", []string{"333333"}),
		)
	})

	t.Run("test level1, object", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			return {
				"i": i,
			}
		}
		obj = f()
		a = obj.i
		`,
			ssatest.CheckTopDef_Equal("a", []string{"333333"}),
		)
	})

	t.Run("test level1 class", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			this =  {
				"i": i,
				// this will be set a class-blue-print
				"set": (i)=>{this.i = i},
			}
			return this
		}
		obj = f()
		a = obj.i
		`,
			ssatest.CheckTopDef_Equal("a", []string{"333333"}),
		)
	})

	t.Run("test level2", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			j = 444444
			return () => {
				return i + j
			}
		}
		f1 = f()
		a = f1()
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444"}),
		)
	})

	t.Run("test level2 object", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			j = 444444
			return () => {
				return {
					"i": j + i, 
				}
			}
		}
		f1 = f()
		obj = f1()
		a = obj.i
		println(a)
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444"}),
		)
	})

	t.Run("test level3", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			j = 444444
			return () => {
				k = 555555
				return () => {
					return i + j + k
				}
			}
		}
		f1 = f()
		f2 = f1()
		a = f2()
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444", "555555"}),
		)
	})

	t.Run("test level3 object", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			j = 444444
			return () => {
				k = 555555
				return () => {
					return {
						"i": i + j + k
					}
				}
			}
		}
		f1 = f()
		f2 = f1()
		obj = f2()
		a = obj.i
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444", "555555"}),
		)
	})
}

func Test_CallStack_FreeValue(t *testing.T) {

}

func Test_CallStack_Normal_SideEffect(t *testing.T) {
	t.Run("test level1", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			i = 444444
		}
		f()
		a = i
		`, ssatest.CheckTopDef_Equal("a", []string{"444444"}),
		)
	})

	// TODO: get top-def, will recursive by object
	t.Run("test level1, object member", func(t *testing.T) {
		code := `
		a = {}
		b = () => {
			a.b = 333333
		}
		b()
		c = a.b;
		`

		ssatest.Check(t, code,
			ssatest.CheckTopDef_Contain("c", []string{"333333"}),
		)
	})

	t.Run("test level2", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			j = 444444
			return () => {
				i = j
			}
		}
		f1 = f()
		f1()
		a = i
		`, ssatest.CheckTopDef_Equal("a", []string{"444444"}),
		)
	})

	t.Run("test level2 object", func(t *testing.T) {
		ssatest.Check(t, `
		obj = {}
		i = 333333
		f = () => {
			j = 444444
			return () => {
				obj.i = j + i
			}
		}
		f1 = f()
		f1()
		a = obj.i
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444"}),
		)
	})

	t.Run("test level3", func(t *testing.T) {
		ssatest.Check(t, `
		i = 333333
		f = () => {
			j = 444444
			return () => {
				k = 555555
				return () => {
					i = k
				}
			}
		}
		f1 = f()
		f2 = f1()
		f2()
		a = i
		`, ssatest.CheckTopDef_Equal("a", []string{"555555"}),
		)
	})

	t.Run("test level3 object", func(t *testing.T) {
		ssatest.Check(t, `
		obj = {}
		i = 333333
		f = () => {
			j = 444444
			return () => {
				k = 555555
				return () => {
					obj.i = i + j + k
				}
			}
		}
		f1 = f()
		f2 = f1()
		f2()
		a = obj.i
		`, ssatest.CheckTopDef_Equal("a", []string{"333333", "444444", "555555"}),
		)
	})

}
