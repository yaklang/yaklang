package org.benf.cfr.reader;

public class TernaryExpressionTest {
	int getVar() {
		return 1;
	}
	void main() {
		int var1 = 1;
		System.out.println(((var1) == (2)) ? (this.getVar()) : (((var1) == (1)) ? (1) : (2)));
		String var2 = "s";
		String var3 = (((var2) == (null)) ? (var2 = "a") : (var2 = "b")).toString();
	}
}