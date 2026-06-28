package codec;

// Differential-execution regression battery for embedded-assignment-in-condition declarations
// (the INT-category case).
//
// javac compiles `(v = expr) == 0` / `(v = expr) < n` as: evaluate expr, dup, store v, branch on the
// dup'd value. The decompiler collapses the store back into the condition as an embedded assignment
// `(v = expr) == 0`, so v has no ordinary `T v = ...;` declaration and the dumper must synthesize one
// at method scope. Before the fix it guessed `Object v = null`, so the int store (`v = expr`) and any
// later arithmetic read (`v + i`) failed to recompile ("incompatible types" / "bad operand types").
// The synthesized declaration must instead be `int v = 0`, detected from the numeric comparison form.
// (The REFERENCE variant — `(s = parts[i]) != null` followed by `s.length()` — needs the real ref
// type threaded through; it is tracked separately in CODEC_TODO and intentionally not exercised here.)
public class EmbeddedAssignDecl {

	// Embedded assignment in an equality guard against an int literal, value read in arithmetic.
	static int firstZeroIndexSum(int[] a, int start) {
		int v;
		int i = start;
		while (i < a.length) {
			if ((v = a[i]) == 0) {
				return v + i;
			}
			i++;
		}
		return -1;
	}

	// Embedded assignment in a relational guard (numeric-only operator), accumulated afterwards.
	static long sumWhileBelow(int[] a, int limit) {
		long acc = 0;
		int v;
		int i = 0;
		while (i < a.length && (v = a[i]) < limit) {
			acc += v;
			i++;
		}
		return acc;
	}

	public static void main(String[] args) {
		int[] a = {3, 7, 0, 5, 2, 0, 9};
		long fp = 1469598103934665603L;
		fp = fp * 1099511628211L + firstZeroIndexSum(a, 0);
		fp = fp * 1099511628211L + sumWhileBelow(a, 6);
		System.out.println("fp=" + fp);
	}
}
