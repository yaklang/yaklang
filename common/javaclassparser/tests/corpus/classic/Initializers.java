public class Initializers {
    static final int[] TABLE;
    static int counter;
    final String id;
    int instanceVal;

    static {
        TABLE = new int[256];
        for (int i = 0; i < TABLE.length; i++) {
            TABLE[i] = i * i;
        }
        counter = 100;
    }

    {
        instanceVal = 7;
    }

    Initializers(String id) {
        this.id = id;
        this.instanceVal += 1;
    }

    public int lookup(int i) {
        return TABLE[i];
    }
}
