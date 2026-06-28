package codec;

import java.util.Map;

// GenericMethodReturn reproduces guava's AbstractMapEntry-style failure: a generic class implementing a
// generic interface (Map.Entry<K,V>) whose accessor methods return a TYPE VARIABLE. In bytecode the
// descriptors are erased (getKey()Ljava/lang/Object;) and the real return type lives ONLY in the method
// Signature attribute (()TK;). A zero-arg method parses to (nil params, K return), so the old
// `sigParams != nil` gate skipped exactly these and rendered `Object getKey()`. That no longer overrides
// Map.Entry.getKey() -> javac: "getKey() in GenericMethodReturn cannot implement getKey() in Map.Entry;
// return type Object is not compatible with K". The one-arg setValue exercises the already-working
// (TV;)TV; path. A deterministic fingerprint over the accessors also catches behavioral drift.
public class GenericMethodReturn<K, V> implements Map.Entry<K, V> {
	private final K key;
	private final V value;

	public GenericMethodReturn(K key, V value) {
		this.key = key;
		this.value = value;
	}

	// zero-arg generic return -> descriptor ()Ljava/lang/Object;, Signature ()TK; (the load-bearing case)
	public K getKey() {
		return key;
	}

	// zero-arg generic return -> descriptor ()Ljava/lang/Object;, Signature ()TV; (the load-bearing case)
	public V getValue() {
		return value;
	}

	// one-arg generic param+return -> Signature (TV;)TV; (pre-existing code path already handled this)
	public V setValue(V newValue) {
		throw new UnsupportedOperationException();
	}

	public static void main(String[] args) {
		GenericMethodReturn<String, Integer> e = new GenericMethodReturn<String, Integer>("alpha", 7);
		// Concatenation is Object-safe, so main compiles whether getKey/getValue return Object or K/V; the
		// load-bearing failure under the kill-switch comes from the override-incompatibility at the class
		// declaration, exactly as in guava. Output is a stable fingerprint.
		System.out.println("k=" + e.getKey() + ";v=" + e.getValue() + ";cls=" + e.getKey().getClass().getSimpleName());
	}
}
