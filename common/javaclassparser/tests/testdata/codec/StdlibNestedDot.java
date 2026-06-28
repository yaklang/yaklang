package codec;

import java.util.LinkedHashMap;
import java.util.Map;

// StdlibNestedDot exercises references to EXTERNAL (JDK) nested types. java.util.Map.Entry must be
// rendered with the dotted source spelling `Map.Entry`, never the binary flat form `Map$Entry` that
// Yak uses for its own flat units (javac cannot resolve `Map$Entry` as a source type name). The
// for-each over entrySet() emits a checkcast to java/util/Map$Entry; the instanceof and the explicit
// cast force the nested type into several more reference positions. A deterministic FNV-style
// fingerprint over a LinkedHashMap (stable iteration order) lets the round-trip oracle also catch any
// behavioral drift, not just a recompile failure.
public class StdlibNestedDot {
	static long fold(long fp, long v) {
		return fp * 1099511628211L + v;
	}

	public static String process(Map<String, Integer> m) {
		long fp = 1469598103934665603L;
		for (Map.Entry<String, Integer> e : m.entrySet()) {
			fp = fold(fp, (long) e.getKey().hashCode());
			fp = fold(fp, (long) e.getValue().intValue());
		}
		return Long.toHexString(fp);
	}

	public static String describe(Object o) {
		if (o instanceof Map.Entry) {
			Map.Entry e = (Map.Entry) o;
			return "entry:" + e.getKey() + "=" + e.getValue();
		}
		return "other";
	}

	public static void main(String[] args) {
		Map<String, Integer> m = new LinkedHashMap<String, Integer>();
		m.put("alpha", 1);
		m.put("beta", 2);
		m.put("gamma", 3);
		StringBuilder sb = new StringBuilder();
		sb.append("fp=").append(process(m)).append(';');
		for (Map.Entry<String, Integer> e : m.entrySet()) {
			sb.append(describe(e)).append(';');
		}
		sb.append("end");
		System.out.println(sb.toString());
	}
}
