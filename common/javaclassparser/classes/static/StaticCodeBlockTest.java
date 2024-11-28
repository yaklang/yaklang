package org.benf.cfr.reader;

public class StaticCodeBlockTest {
	public StaticCodeBlockTest() {
		return;
	}
	static {
		System.out.println("load class");
		return;
	}
}