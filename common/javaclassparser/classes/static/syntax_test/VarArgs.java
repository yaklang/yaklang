package org.benf.cfr.reader;

public class VarArgs {
	public VarArgs() {
	}
	 void main(String... var1) {
		System.out.println(var1[0]);
	}
	 void invoke() {
		String var1 = "a";
		String[] var2 = new String[1];
		var2[0] = "a";
		this.main(var2);
	}
}