package org.benf.cfr.reader;

import java.io.File;
import java.io.FileInputStream;

public class TryCatch1 {
	public TryCatch1() {
	}
	public static void main(String[] var0) {
		System.out.println(2);
		try{
			File var1 = new File("");
			FileInputStream var2 = new FileInputStream(var1);
			try{
				System.out.println(1);
				var2.close();
			}catch(Throwable var3){
				try{
					var2.close();
				}catch(Throwable var4){
					var3.addSuppressed(var4);
				}
				throw var3;
			}
		}catch(Exception var1){
			var1.printStackTrace();
		}
	}
}