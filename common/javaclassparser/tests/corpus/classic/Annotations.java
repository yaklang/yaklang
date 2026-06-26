import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

public class Annotations {
    @Retention(RetentionPolicy.RUNTIME)
    @Target({ElementType.METHOD, ElementType.TYPE})
    @interface Marker {
        String value() default "none";

        int priority() default 0;
    }

    @Marker(value = "important", priority = 5)
    public int annotated() {
        return 1;
    }

    @Marker
    @Deprecated
    public void legacy() {
    }

    @SuppressWarnings("unchecked")
    public Object raw() {
        return new java.util.ArrayList();
    }
}
