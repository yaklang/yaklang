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
	void newExpression() {
		String var1 = new String("");
	}
}