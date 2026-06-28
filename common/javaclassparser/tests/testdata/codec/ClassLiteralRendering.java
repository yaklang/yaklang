package codec;

// Differential-execution regression battery for class-literal (ldc Class constant) rendering.
//
// A JavaClassValue is the Class object produced by `Type.class`. It must render as `Type.class` in
// every value position, while a STATIC method call on a type must stay the bare `Type.method(...)`.
// Before the fix the decompiler dropped the `.class`, so:
//   - `String.class.getName()`      came out as `String.getName()`     (cannot find symbol)
//   - `return Integer.class;`        came out as `return Integer;`      (cannot find symbol)
// while a static call like `Integer.parseInt(s)` must NOT become `Integer.class.parseInt(s)`.
// This battery exercises the inline literal/receiver/static/array forms AND the "stored in a local"
// form (`Class<?> c = Long.class; c.getName()`): the capturing local must be declared `Class`, not
// the referenced type, or the later member reads fail to recompile ("cannot find symbol"). With any
// of these bugs the decompiled source fails to recompile.
public class ClassLiteralRendering {

	static String typeNames() {
		StringBuilder sb = new StringBuilder();
		sb.append(String.class.getName());
		sb.append('|');
		sb.append(Integer.class.getName());
		sb.append('|');
		sb.append(int[].class.getName());
		sb.append('|');
		sb.append(byte[][].class.getName());
		sb.append('|');
		sb.append(Long.class.getName());
		sb.append('|');
		Object o = "probe";
		sb.append(o.getClass().getName());
		sb.append('|');
		sb.append(Long.class.isPrimitive());
		sb.append('|');
		sb.append(nameOf(Double.class));
		sb.append('|');
		sb.append(selfName());
		sb.append('|');
		sb.append(localClassLiteral());
		return sb.toString();
	}

	// Class literal stored in a local variable and read several times (so it is NOT folded back to
	// an inline `Long.class.getName()`): the capturing local must be declared `Class`, not the
	// referenced type `Long`. Before the fix it came out as `Long c = Long.class;` and every member
	// read (`c.getName()`, `c.isPrimitive()`, `c.getSimpleName()`) failed to recompile because Long
	// has no such instance methods ("cannot find symbol").
	static String localClassLiteral() {
		Class<?> c = Long.class;
		StringBuilder sb = new StringBuilder();
		sb.append(c.getName());
		sb.append('|');
		sb.append(c.isPrimitive());
		sb.append('|');
		sb.append(c.getSimpleName());
		return sb.toString();
	}

	// Class literal flowing through an argument (value position): must render `nameOf(Double.class)`.
	static String nameOf(Class<?> c) {
		return c.getName();
	}

	// Instance method called on the CURRENT class's own class literal: must render
	// `ClassLiteralRendering.class.getName()`, NOT a bare unqualified `getName()` (which would be
	// mistaken for a static self-call and fail to compile).
	static String selfName() {
		return ClassLiteralRendering.class.getName();
	}

	static int staticCalls(String s) {
		return Integer.parseInt(s) + Math.max(3, 4) + Integer.valueOf(7).intValue();
	}

	public static void main(String[] args) {
		long acc = 1125899906842597L;
		String names = typeNames();
		for (int i = 0; i < names.length(); i++) {
			acc = acc * 131 + names.charAt(i);
		}
		acc = acc * 131 + staticCalls("100");
		System.out.println("fp=" + acc + " names=" + names);
	}
}
