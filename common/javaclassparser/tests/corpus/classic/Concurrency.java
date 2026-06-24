public class Concurrency {
    private final Object lock = new Object();
    private volatile int counter;
    private transient int cache;

    public void increment() {
        synchronized (lock) {
            counter++;
        }
    }

    public synchronized int read() {
        return counter;
    }

    public int blockSync(int[] data) {
        int sum = 0;
        synchronized (this) {
            for (int x : data) {
                sum += x;
            }
            cache = sum;
        }
        return sum;
    }
}
