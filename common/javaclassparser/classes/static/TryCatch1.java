package org.benf.cfr.reader;

import java.io.File;
import java.io.FileInputStream;

public class TryCatch1 {
	public static void main(String[] var0) {
		System.out.println(2);
		try{
			FileInputStream var1 = new FileInputStream(new File(""));
			try{
				System.out.println(1);
				var1.close();
			}catch(Throwable var2){
				try{
					var1.close();
				}catch(Throwable var3){
					var2.addSuppressed(var3);
				}
				throw var2;
			}
		}catch(Exception var1){
			var1.printStackTrace();
		}
	}
}