package tests

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed code/DynamicSecurityMetadataSource.java
var DynamicSecurityMetadataSource string

func TestRealJava_PanicInMemberCall(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("DynamicSecurityMetadataSource.java", DynamicSecurityMetadataSource)
	ssatest.CheckWithFS(vf, t, func(prog ssaapi.Programs) error {
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestA(t *testing.T) {
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

	prog, err := ssaapi.ParseProject(
		ssaapi.WithFileSystem(vf),
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
	require.NoError(t, err)
	// prog.Show()
	res, err := prog.SyntaxFlowWithError(`
Path.__ref__?{opcode: function} as $path_handler
$path_handler?{.annotation.*?{have:"GET"}} as $ get_path_handler 
$get_path_handler(* ?{opcode: param} as $format_param)

// 查找到后续被build的entity
Response...entity?{*...build()} as $builded_entity 
// 获取entity的参数 
$builded_entity(, * as $target)   


$target #{
    include: "* & $format_param" 
}-> as $xss 

    `)
	require.NoError(t, err)
	res.Show(sfvm.WithShowCode())

}
