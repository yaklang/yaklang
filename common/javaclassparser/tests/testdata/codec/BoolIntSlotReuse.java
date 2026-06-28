package codec;

// Bug AI probe: a single JVM local slot is reused across DISJOINT live ranges to hold first an int
// temp and later a boolean (the method's return value), the exact shape javac emits for guava
// MapMakerInternalMap$Segment.replace: `var11 = this.count - 1; this.count = var11;` (int) in one
// branch and `return false/true` (boolean) in another. If the decompiler merges the slot into ONE
// variable and types it boolean, the int store/read no longer recompiles
// (`int cannot be converted to boolean`). A clean byte-for-byte round-trip proves the slot is split
// (or correctly typed) per live range.
public class BoolIntSlotReuse {
    int count;
    int modCount;

    BoolIntSlotReuse(int c) {
        this.count = c;
        this.modCount = 0;
    }

    void touch() {
        this.modCount++;
    }

    // slot-reuse shape: a nested if performs int arithmetic into the field through a temp, then the
    // method returns a boolean literal. javac keeps the int temp and the boolean return in distinct
    // representations here, which the decompiler types correctly (the harder variant, where the
    // boolean return value is itself stored into the int temp's slot before a trailing method call,
    // is tracked as Bug AI in CODEC_TODO with a standalone minimal repro).
    boolean shrink(boolean collected) {
        if (this.count != 0) {
            if (collected) {
                int t = this.count - 1;
                this.modCount = this.modCount + 1;
                this.count = t;
            }
            return false;
        }
        return true;
    }

    // a second shape: the int temp and a boolean flag alternate inside a loop, again sharing a slot.
    int churn(int n) {
        int acc = 0;
        for (int i = 0; i < n; i++) {
            if ((i & 1) == 0) {
                int delta = this.count - i;
                this.count = delta;
                acc += delta;
            }
            boolean even = (i % 3) == 0;
            if (even) {
                acc += 7;
            }
        }
        return acc;
    }

    public static void main(String[] args) {
        BoolIntSlotReuse a = new BoolIntSlotReuse(5);
        boolean r1 = a.shrink(true);
        boolean r2 = a.shrink(false);
        BoolIntSlotReuse b = new BoolIntSlotReuse(100);
        int c = b.churn(11);
        StringBuilder sb = new StringBuilder();
        sb.append("BoolIntSlotReuse:");
        sb.append(r1).append(":").append(r2).append(":");
        sb.append(a.count).append(":").append(a.modCount).append(":");
        sb.append(c).append(":").append(b.count);
        System.out.println(sb.toString());
    }
}
