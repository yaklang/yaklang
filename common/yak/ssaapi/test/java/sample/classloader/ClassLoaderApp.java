import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import java.io.IOException;
import java.util.Base64;

@SpringBootApplication
public class DynamicClassLoadingApplication {

    public static void main(String[] args) {
        SpringApplication.run(DynamicClassLoadingApplication.class, args);
    }

    @RestController
    public class ClassLoaderController {

        @GetMapping("/loadClassFromBase64")
        public String loadClassFromBase64(@RequestParam String userRaw) {
            try {
                Base64ClassLoader loader = new Base64ClassLoader();
                Class<?> clazz = loader.defineClassFromBase64("com.example.DynamicClass", userRaw);
                Object instance = clazz.getDeclaredConstructor().newInstance();
                return "Class loaded and instance created: " + instance.toString();
            } catch (Exception e) {
                return "Error loading class: " + e.getMessage();
            }
        }

        @GetMapping("/loadExistingClass")
        public String loadExistingClass(@RequestParam String className) {
            try {
                FileSystemClassLoader loader = new FileSystemClassLoader();
                Class<?> clazz = loader.loadClass(className);
                Object instance = clazz.getDeclaredConstructor().newInstance();
                return "Class loaded and instance created: " + instance.toString();
            } catch (Exception e) {
                return "Error loading class: " + e.getMessage();
            }
        }

        // Custom class loader for Base64
        class Base64ClassLoader extends ClassLoader {
            public Class<?> defineClassFromBase64(String name, String base64) throws IOException {
                byte[] classBytes = Base64.getDecoder().decode(base64);
                return defineClass(name, classBytes, 0, classBytes.length);
            }
        }

        // Custom class loader for loading from the file system
        class FileSystemClassLoader extends ClassLoader {
            @Override
            public Class<?> loadClass(String name) throws ClassNotFoundException {
                return super.loadClass(name);
            }
        }
    }
}