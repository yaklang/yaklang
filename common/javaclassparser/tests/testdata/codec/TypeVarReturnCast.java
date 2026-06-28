package codec;

import java.util.ArrayList;
import java.util.List;

// TypeVarReturnCast reproduces the classic generic-accessor failure unmasked by method-Signature return
// recovery: a method whose return type is a BOUNDED type variable (T extends Comparable<T>) returns a
// LOCAL whose static type was inferred as the erased bound (Comparable), not the type variable. In
// bytecode `T max()` erases to `()Ljava/lang/Comparable;` and the local `best` is a Comparable; once Yak
// correctly recovers the return type to `T` (from the method Signature `()TT;`), `return best` no longer
// compiles ("incompatible types: Comparable cannot be converted to T"). Real decompilers (CFR/Fernflower)
// emit an unchecked `(T)` cast at the return site; this battery proves Yak does the same. A deterministic
// fingerprint over max() also catches behavioral drift, not just a recompile error.
public class TypeVarReturnCast<T extends Comparable<T>> {
	private final List<T> items = new ArrayList<T>();

	void add(T v) {
		items.add(v);
	}

	// return type recovers to T; body returns a local typed as the erased bound (Comparable) -> needs (T)
	T max() {
		T best = null;
		for (T item : items) {
			if (best == null || item.compareTo(best) > 0) {
				best = item;
			}
		}
		return best;
	}

	public static void main(String[] args) {
		TypeVarReturnCast<Integer> m = new TypeVarReturnCast<Integer>();
		m.add(7);
		m.add(3);
		m.add(9);
		m.add(1);
		m.add(5);
		Integer mx = m.max();
		long fp = 1469598103934665603L;
		fp = (fp ^ (long) mx.intValue()) * 1099511628211L;
		fp = (fp ^ (long) m.items.size()) * 1099511628211L;
		System.out.println("fp=" + Long.toHexString(fp));
	}
}
