public class Literals {
    static final int HEX = 0xCAFE;
    static final int OCT = 0777;
    static final int BIN = 0b1010_1010;
    static final int UNDERSCORE = 1_000_000;
    static final long BIG = 9_223_372_036_854_775_807L;
    static final float F = 3.14f;
    static final double D = 2.718281828;
    static final char C = '\n';
    static final char UC = '\u0041';
    static final String S = "hello\tworld\n";
    static final boolean B = true;

    public Object[] all() {
        return new Object[]{HEX, OCT, BIN, UNDERSCORE, BIG, F, D, C, UC, S, B};
    }
}
