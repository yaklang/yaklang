package org.benf.cfr.reader;

public class VarArgs {
	public VarArgs() {
	}
	void main(String... var1) {
		System.out.println(var1[0]);
	}
	void invoke() {
		String var1 = "a";
		this.main(new String[1]);
	}
}