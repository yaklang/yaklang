package org.benf.cfr.reader;

public class LogicalOperation {
	public LogicalOperation() {
	}
	boolean main() {
		int var1 = 1;
		boolean var2 = ((var1) == (3)) || ((var1) == (5));
		var2 = ((var1) == (3)) && ((var1) == (5));
		var2 = ((var1) == (3)) || (((var1) == (3)) && ((var1) == (5)));
		return var2;
	}
}