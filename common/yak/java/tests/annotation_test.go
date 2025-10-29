package tests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestDefAnnotation(t *testing.T) {
	code := `import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.*;
import java.lang.annotation.Target;

// 定义一个注解
@Retention(RetentionPolicy.RUNTIME)
@Target(ElementType.METHOD)
public @interface MyAnnotation {
    // 定义一个字符串类型的属性
    String value() default "default";
    
    // 定义一个枚举类型的属性
    enum Status { START, STOP }
    Status status() default Status.START;
    
    // 定义一个Class类型的属性
    Class<?> targetClass() default String.class;
    
    // 定义一个注解类型的属性
    OtherAnnotation otherAnnotation() default @OtherAnnotation;
    
    // 定义一个数组类型的属性
    String[] array() default {};
}

// 定义另一个注解
@interface OtherAnnotation {
    String name() default "other";
}

// 使用注解
public class MyClass {
    @MyAnnotation(value = "abc", status = MyAnnotation.Status.STOP, targetClass = MyClass.class, 
                  otherAnnotation = @OtherAnnotation(name = "otherName"), array = {"a", "b"})
    public void myMethod() {
        // method body
    }
}
class A{
    @Path("PATHPATH")
    public void AMethod() {
        MyClass c = new MyClass();
        c.myMethod();
    }
}
`

	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		{
			res, err := prog.SyntaxFlowWithError(`*yAnno*`)
			require.NoError(t, err)
			res.Show()
			require.Greater(t, res.GetValueCount("_"), 0)
		}

		{
			res, err := prog.SyntaxFlowWithError(`g"*PATHPATH*"`)
			require.NoError(t, err)
			res.Show()
			require.Equal(t, res.GetValueCount("_"), 1)
		}

		return nil

	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

func TestSearchAnnotation(t *testing.T) {
	code := `
@Path("")
// context is /api and is set in EmissaryServer.java
public class Pool {
    private final Logger logger = LoggerFactory.getLogger(this.getClass());

    public static final String POOL_ENDPOINT = "api/pool";
    public static final String POOL_CLUSTER_ENDPOINT = "api/cluster/pool";

    @GET
    @Path("/pool")
    @Produces(MediaType.APPLICATION_JSON)
    public Response pool() {
        return Response.ok().entity(this.lookupPool()).build();
    }

    @GET
    @Path("/cluster/pool")
    @Produces(MediaType.APPLICATION_JSON)
    public Response clusterPool() {
        MapResponseEntity entity = new MapResponseEntity();
        try {
            // Get our local mobile agents
            entity.append(this.lookupPool());
            // Get all of our peers agents
            EmissaryClient client = new EmissaryClient();
            for (String peer : lookupPeers()) {
                String remoteEndPoint = stripPeerString(peer) + "api/pool";
                MapResponseEntity remoteEntity = client.send(new HttpGet(remoteEndPoint)).getContent(MapResponseEntity.class);
                entity.append(remoteEntity);
            }
            return Response.ok().entity(entity).build();
        } catch (EmissaryException e) {
            // This should never happen since we already saw if it exists
            return Response.serverError().entity(e.getMessage()).build();
        }
    }
}
	`
	vf := filesys.NewVirtualFs()
	vf.AddFile("DocumentAction.java", code)

	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		res, err := prog.SyntaxFlowWithError(`
Path.__ref__?{opcode: function} as $path_handler
$path_handler?{.annotation.*?{have:"GET"}} as $ get_path_handler 
        `)
		require.NoError(t, err)
		res.Show()

		require.Equal(t, res.GetValueCount("path_handler"), 2)
		require.Equal(t, res.GetValueCount("get_path_handler"), 2)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
