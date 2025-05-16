package tests

import (
	"testing"
)

func TestJavaBasic_Variable_InIf(t *testing.T) {
	t.Run("test simple if", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a=1;
		println(a);
		if(c){
			a=2;
			println(a);
		}
		println(a);`, []string{
			"1",
			"2",
			"phi(a)[2,1]"}, t)
	})

	t.Run("test simple if with local variable", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a=1;
		println(a);
		if(c){
			var a = 2;
			println(a);
			}
			println(a);`, []string{
			"1",
			"2",
			"1",
		}, t)
	})
	t.Run("test multiple phi if", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		if (c) {
			a = 2;
		}
		println(a);
		println(a);
		println(a);
		`, []string{
			"phi(a)[2,1]",
			"phi(a)[2,1]",
			"phi(a)[2,1]",
		}, t)
	})
	t.Run("test simple if else", func(t *testing.T) {
		CheckJavaPrintlnValue(`
         var a=1;
         println(a);
		if (c){
			a=2;
			println(a);
		}else {
			a=3;
			println(a);
		}
		println(a);`, []string{"1", "2", "3", "phi(a)[2,3]"}, t)
	})

	t.Run("test simple if else with origin branch", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		println(a);
		if (c) {
			// a = 1
		} else {
			a = 3;
			println(a);
		}
		println(a); // phi(a)[1, 3]
		`, []string{
			"1",
			"3",
			"phi(a)[1,3]",
		}, t)
	})
	t.Run("test if-elseif", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		println(a);
		if (c) {
			a = 2;
			println(a);
		}else if (c == 2){
			a = 3 ;
			println(a);
		}
		println(a);
		`,
			[]string{
				"1",
				"2",
				"3",
				"phi(a)[2,3,1]",
			}, t)
	})
	t.Run("test with return, no DoneBlock", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		   var a = 1;
		println(a); // 1
		if (c) {
			return ;
		}
		println(a); // phi(a)[Undefined-a,1]
		`, []string{
			"1",
			"phi(a)[Undefined-a,1]",
		}, t)
	})

	t.Run("test with return in branch, no DoneBlock", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		   var a = 1;
		println(a); // 1
		if (c) {
			if (b) {
				a = 2;
				println(a); // 2
				return ;
			}else {
				a = 3;
				println(a); // 3
				return ;
			}
			println(a); // unreachable // phi[2, 3]
		}
		println(a); // phi(a)[Undefined-a,1]
		`, []string{
			"1",
			"2",
			"3",
			"phi(a)[Undefined-a,1]",
		}, t)
	})

	t.Run("in if sub-scope", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		if (c) {
			 a = 2;
		}
		println(a);
		`, []string{"Undefined-a"}, t)
	})

	t.Run("test if mutli parexpression", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		println(a);
		if ((x>10) && (y>20)) {
			a = 2;
			println(a);
		}
		println(a);
		`, []string{
			"1",
			"2",
			"phi(a)[2,1]",
		}, t)
	})
	t.Run("test param should be phi after return", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`
		package main;
class A {
	public void PathTravel(String filePath){
		if (!Utils.Validate(filePath)) {
			logger.error("Invalid file path: " + filePath);
			return;
		}
		println(filePath);		
	}
}
		`, []string{"phi(filePath)[Undefined-filePath,Parameter-filePath]"}, t)
	})

	t.Run("test param should not be phi after normal if statement", func(t *testing.T) {
		CheckAllJavaPrintlnValue(`
		package main;
class A {
	public void PathTravel(String filePath){
		if (!Utils.Validate(filePath)) {
			logger.error("Invalid file path: " + filePath);
		}
		println(filePath);		
	}
}
		`, []string{"Parameter-filePath"}, t)
	})
}

func TestJavaBasic_Variable_Switch(t *testing.T) {
	t.Run("test SwitchStatement", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a=1;
		switch(a){
		case 2:
			a = 22;
			println(a);
		case 3,4:
			a = 33;
			println(a);
		}
	    println(a);
		`, []string{
			"22",
			"33",
			"phi(a)[phi(a)[33,1],1]"},
			t)
	})
	t.Run("simple switch, has default but nothing", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		switch (a) {
		case 2: 
			a = 22;
			println(a);
		case 3, 4:
			a = 33;
			println(a);
		default: 
		}
		println(a); // phi[1, 22, 33]
		`, []string{
			"22", "33", "phi(a)[phi(a)[33,1],1]",
		}, t)
	})
	t.Run("switch build", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		switch (a) {
		case 2: 
			a = 22;
			println(a);
		default: 
			a = 44;
			println(44);
		}
		println(a); // 4
}
`, []string{"22", "44", "44"}, t)
	})
	t.Run("simple switch, has default", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		switch (a) {
		case 2: 
			a = 22;
			println(a);
		case 3, 4:
			a = 33;
			println(a);
		default: 
			a = 44;
			println(a);
		}
		println(a); // 4
		`, []string{
			"22", "33", "44", "44",
		}, t)
	})
	t.Run("test default statement before case 1", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		switch (a) {
		default:
			a = 44;
			println(a); 
		case 2: 
			a = 22;
			println(a);
		case 3, 4:
			a = 33;
			println(a);
		}
		println(a); 
		`, []string{
			"22", "33", "44", "44",
		}, t)
	})
	t.Run("test default statement before case 2", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a = 1;
		switch (a) {
		case 2: 
			a = 22;
			println(a);
		default:
			a = 44;
			println(a); 
		case 3, 4:
			a = 33;
			println(a);
		}
		println(a); 
		`, []string{
			"22", "33", "44", "44",
		}, t)
	})
}

func TestJavaBasic_Variable_SwitchArrow(t *testing.T) {
	t.Run("test switch arrow stmt", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a=1;
		switch(a){
		case 1 -> println(a);
		case 2 -> {
		a = 22;
		println(a);}
		case null -> println(a);
}
	    println(a);
		`, []string{
			"1",
			"22",
			"phi(a)[22,1]",
			"phi(a)[phi(a)[22,1],1]",
		},
			t)
	})
	t.Run("test switch arrow stmt with default", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		var a=1;
		switch(a){
		case 1 -> println(a);
		case 22,33 -> a = 22;
		default -> println(a);
}
	    println(a);
		`, []string{
			"1",
			"phi(a)[22,1]",
			"phi(a)[22,1]",
		},
			t)
	})

}

func TestYaklangBasic_Variable_ForLoop(t *testing.T) {
	t.Run("simple loop not change", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		for (int i=0; i < 10 ; i++) {
			println(a); // 1
		}
		println(a); //1 
		`,
			[]string{
				"1",
				"1",
			},
			t)
	})

	t.Run("test simple loop", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int i=0;
		for (i=0; i<10; i++) {
			println(i); // phi[0, i+1]
		}
		println(i);
		`,
			[]string{
				"phi(i)[0,add(i, 1)]",
				"phi(i)[0,add(i, 1)]",
			}, t)
	})

	t.Run("test loop with spin, signal phi", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		for (int i = 0; i < 10; i++) { // i=0; i=phi[0,1]; i=0+1=1
			println(a); // phi[0, $+1]
			a = 0;
			println(a) ;// 0 
		}
		println(a) ; // phi[0, 1]
		`,
			[]string{
				"phi(a)[1,0]",
				"0",
				"phi(a)[1,0]",
			},
			t)
	})

	t.Run("test loop with spin, double phi", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		for (int i = 0; i < 10; i ++) {
			a += 1;
			println(a) ;// add(phi, 1)
		}
		println(a);  // phi[1, add(phi, 1)]
		`,
			[]string{
				"add(phi(a)[1,add(a, 1)], 1)",
				"phi(a)[1,add(a, 1)]",
			},
			t)
	})

	t.Run("test infinite foroop ", func(t *testing.T) {
		CheckJavaPrintlnValue(`
          int a=1;
		  for(;;){
			println(a);
			}`,
			[]string{
				"1",
			},
			t)
	})

}

func TestYaklangBasic_CFG_Break(t *testing.T) {
	t.Run("simple break in loop", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		for (int i= 0; i < 10; i++ ){
			if (i == 5) {
				a = 2;
				break;
			}
		}
		println(a); // phi[1, 2]
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple continue in loop", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		for (int i = 0; i < 10; i++) {
			if (i == 5) {
				a = 2;
				continue;
			}
		}
		println(a); // phi[1, 2]
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple break in switch", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		a = 1;
		switch (a) {
		case 1:
			if (c) {
				 a = 2;
				 break;
			}
			a = 4;
		case 2:
			a = 3;
		}
		println(a) ;
		`, []string{
			"phi(a)[2,phi(a)[phi(a)[3,1],1]]",
		}, t)
	})

	t.Run("simple break in switch arrow stmt", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		switch (a) {
		case 1 -> {
		   		if (c) {
                    a = 2;
					break;
                  }
               	a = 4;}
		case 2-> a = 3;
		}
		println(a) ;
		`, []string{
			"phi(a)[2,phi(a)[3,1]]",
		}, t)
	})

	t.Run("test while with break", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		i = 1;
		while(i<10) { 
			println(i);
			i = 2;
			break;
		    println(i);
		}
		println(i);`, []string{
			"phi(i)[1,2]",
			"phi(i)[2,phi(i)[1,2]]",
		}, t)
	})

}

func TestJavaBasic_Variable_Try(t *testing.T) {
	t.Run("simple, no final", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		try {
			a = 2;
			println(a);
		} catch (Exception e) {
			println(a);
			a = 3;
		}
		println(a);`, []string{
			"2", "phi(a)[2,1]", "phi(a)[2,3]",
		}, t)
	})

	t.Run("simple, with final", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		try {
			a = 2;
			println(a);
		} catch (ArrayIndexOutOfBoundsException e) {
			println(a); // phi(1, 2)
			a = 3;
		} finally {
			println(a);// phi(2, 3)
		}
		println(a);// phi(2, 3)
		`, []string{
			"2", "phi(a)[2,1]", "phi(a)[2,3]", "phi(a)[2,3]",
		}, t)
	})

	t.Run("simple, no finally, has err", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		try {
		} catch (Exception e) {
			println(e);
		}
		println(e);
		`, []string{
			"Undefined-e", "Undefined-e",
		}, t)
	})

	t.Run("simple, has finally, has err", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		try {
		} catch (Exception e) {
			println(e);
		} finally {
			println(e);
		}
		println(e);
		`, []string{
			"Undefined-e",
			"Undefined-e",
			"Undefined-e",
		}, t)
	})
}

func TestJavaBasic_variable_Try_Multiple_cache(t *testing.T) {
	// simple  multiple catch
	t.Run("simple, multiple catch", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		try {
			a = 2;
			println(a); // 2
		} catch (ArrayIndexOutOfBoundsException e) {
			println(a); // phi(1, 2)
			a = 3;
		} catch (Exception e) {
			println(a); // phi(1, 2)
			a = 4;
		}
		println(a); // phi(2, 3, 4)
		`, []string{
			"2", "phi(a)[2,1]", "phi(a)[2,1]", "phi(a)[2,3,4]",
		}, t)
	})

	// simple with final
	t.Run("simple, multiple catch, with final", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		int a = 1;
		try {
			a = 2;
			println(a);
		} catch (ArrayIndexOutOfBoundsException e) {
			println(a); // phi(1, 2)
			a = 3;
		} catch (Exception e) {
			println(a); // phi(1, 2)
			a = 4;
		} finally {
			println(a); // phi(2, 3, 4)
			a = 5;
		}
		println(a); // 5 
		`, []string{
			"2", "phi(a)[2,1]", "phi(a)[2,1]", "phi(a)[2,3,4]", "5",
		}, t)
	})

}

func TestJavaBasic_Variable_While(t *testing.T) {
	t.Run("test simple while statement", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		i = 1;
		while(i<10) { 
			println(i);// phi
			i = 2; 
			println(i); // 2
		}
		println(i); // phi`, []string{
			"phi(i)[1,2]",
			"2",
			"phi(i)[1,2]",
		}, t)
	})
	t.Run("test  infinite while statement", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		i = 1;
		while(true) { 
			i = 2;
			println(i);
		}
		println(i);`, []string{
			"2",
			"phi(i)[1,2]",
		}, t)
	})

	t.Run("test do while statement", func(t *testing.T) {
		CheckJavaPrintlnValue(`
		i = 0;
		do {
			println(i);
			i++;
		} while (i < 10);
		println(i);`, []string{
			"phi(i)[0,add(i, 1)]",
			"phi(i)[0,add(i, 1)]",
		}, t)
	})
}
