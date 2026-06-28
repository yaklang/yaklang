package codec;

import java.util.BitSet;

// Differential-execution regression battery for the <clinit> contiguous-prefix hoist barrier
// (illegal-forward-reference + semantic-reorder of a static-final initializer). This mirrors the
// commons-codec URLCodec shape exactly:
//   - SAFE is allocated by the FIRST <clinit> action (a hoistable prefix initializer).
//   - SAFE is then MUTATED by a loop of set(...) side effects.
//   - DERIVED is cloned from the mutated SAFE as the LAST <clinit> action, and DERIVED is DECLARED
//     BEFORE SAFE in the field table.
//
// The dumper emits every lifted field initializer ABOVE the static{} block, so lifting
// `DERIVED = (BitSet) SAFE.clone()` would both forward-reference SAFE (declared later) AND run the
// clone before the set(...) loop, capturing an EMPTY BitSet -> a different fingerprint. The barrier
// must keep DERIVED (and TABLE) as blank-final stores at the end of the static block. Disabling the
// barrier (JDEC_NO_CLINIT_HOIST_BARRIER=1) makes this battery fail to recompile or diverge.
public class StaticInitForwardRef {

	static final BitSet DERIVED;
	static final BitSet SAFE = new BitSet(256);
	static final int[] TABLE;
	static int sideCounter;

	static {
		for (int i = 'a'; i <= 'z'; i++) {
			SAFE.set(i);
		}
		for (int i = '0'; i <= '9'; i++) {
			SAFE.set(i);
		}
		SAFE.set('-');
		SAFE.set('_');
		sideCounter = SAFE.cardinality();
		int[] t = new int[8];
		for (int i = 0; i < t.length; i++) {
			t[i] = i * i + sideCounter;
		}
		TABLE = t;
		DERIVED = (BitSet) SAFE.clone();
	}

	public static void main(String[] args) {
		long acc = 1125899906842597L;
		acc = acc * 131 + DERIVED.cardinality();
		acc = acc * 131 + SAFE.cardinality();
		acc = acc * 131 + sideCounter;
		for (int i = 0; i < TABLE.length; i++) {
			acc = acc * 131 + TABLE[i];
		}
		acc = acc * 131 + (DERIVED.get('a') ? 1 : 0);
		acc = acc * 131 + (DERIVED.get('5') ? 1 : 0);
		acc = acc * 131 + (DERIVED.get('Z') ? 1 : 0);
		System.out.println("fp=" + acc);
	}
}
