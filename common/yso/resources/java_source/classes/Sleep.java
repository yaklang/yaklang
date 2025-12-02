package payload;

public class Sleep {
    static String sleepTime = "{{time}}";
    static {
        try {
            Thread.sleep(Integer.parseInt(sleepTime));
        } catch (InterruptedException e) {
            e.printStackTrace();
        }
    }
}
