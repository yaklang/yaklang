package org.benf.cfr.reader;

public class VarFold {
	public VarFold() {
	}
	void common() {
		System.out.println(1);
	}
	void commonNegative() {
		int var1 = 1;
		System.out.println(1);
		int var2 = var1;
	}
	void scope() {
		if ((1) > (1)){
			int var1 = 1;
			var1 = 3;
			System.out.println(var1);
		}
		int var1 = 2;
		int var2 = 1;
		System.out.println(var1);
		System.out.println(var2);
		int var3 = 2;
		System.out.println(1);
		System.out.println(var3);
	}
	void newExpression() {
		String var1 = new String("");
	}
}