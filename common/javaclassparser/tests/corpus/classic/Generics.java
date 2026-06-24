import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.HashMap;

public class Generics<T extends Comparable<T>> {
    private T value;
    private List<T> items = new ArrayList<T>();

    public void set(T v) {
        this.value = v;
        this.items.add(v);
    }

    public T max() {
        T best = null;
        for (T item : items) {
            if (best == null || item.compareTo(best) > 0) {
                best = item;
            }
        }
        return best;
    }

    public static <E> E first(List<E> list) {
        return list.isEmpty() ? null : list.get(0);
    }

    public Map<String, List<Integer>> build() {
        Map<String, List<Integer>> m = new HashMap<>();
        m.put("k", new ArrayList<Integer>());
        return m;
    }

    public int countUpper(List<? extends Number> nums) {
        int c = 0;
        for (Number n : nums) {
            if (n.intValue() > 0) {
                c++;
            }
        }
        return c;
    }
}
