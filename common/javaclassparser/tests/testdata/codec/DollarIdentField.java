package codec;

// 关键词: 美元标识符, '$' 字段名, 反混淆, 后置语法校验容错
// Battery: an instance field literally named "$" -- a legal JVM AND javac identifier that real
// obfuscators emit (e.g. asm-6.0_BETA's MethodWriter renames fields to "$", "b", "r", ...). The
// decompiled output renders a standalone '$' as `this.$`; yak's Java grammar lexes a lone '$' as
// the dedicated Dollar token (for "${...}") rather than IDENTIFIER, so without the validator's
// '$'-tolerance the '$' method false-degrades to a throwing stub and the fingerprint diverges.
// A sibling field named "$$" (already a valid IDENTIFIER) guards against the tolerance over-reaching
// and corrupting two-dollar names.
public class DollarIdentField {
    private long $;
    private int $$;

    private long step(long seed) {
        this.$ = seed ^ 0x9E3779B97F4A7C15L;
        this.$$ = (int) (seed & 0x1F);
        for (int i = 0; i < this.$$ + 8; i++) {
            this.$ = (this.$ * 6364136223846793005L) + 1442695040888963407L;
            this.$ ^= (this.$ >>> 31);
        }
        return this.$ + this.$$;
    }

    public static void main(String[] args) {
        DollarIdentField d = new DollarIdentField();
        long acc = 1125899906842597L;
        for (int i = 1; i <= 200; i++) {
            acc = (acc * 31) + d.step(i);
        }
        System.out.println(Long.toHexString(acc));
    }
}
