package tests

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
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
// context is emissary
public class DocumentAction {

    private static final Logger LOG = LoggerFactory.getLogger(DocumentAction.class);

    public static final String UUID_TOKEN = "token";
    public static final String SUBMISSION_TOKEN = "SUBMISSION_TOKEN";

    @GET
    @Path("/Document.action")
    @Produces(MediaType.TEXT_HTML)
    @Template(name = "/document_form")
    public Map<String, Object> documentForm() {
        Map<String, Object> map = new HashMap<>();
        return map;
    }

    @GET
    @Path("/Document.action/{uuid}")
    @Produces(MediaType.APPLICATION_XML)
    public Response documentShow(@Context HttpServletRequest request, @PathParam("uuid") String uuid) {

        try {
            final WebSubmissionPlace wsp = (WebSubmissionPlace) Namespace.lookup("WebSubmissionPlace");
            final List<IBaseDataObject> payload = wsp.take(uuid);
            if (payload != null) {
                LOG.debug("Found payloads for token {}", uuid);
                List<IBaseDataObject> uncheckedPayloadList = (List<IBaseDataObject>) payload;
                List<IBaseDataObject> payloadList = new ArrayList<IBaseDataObject>();
                for (Object o : uncheckedPayloadList) {
                    if (o instanceof IBaseDataObject) {
                        payloadList.add((IBaseDataObject) o);
                    }
                }
                String xml = PayloadUtil.toXmlString(payloadList);
                return Response.ok().entity(xml).build();
            } else {
                return Response.status(400).entity("<error>uuid " + uuid + " not found</error>").build();
            }
        } catch (NamespaceException e) {
            LOG.error("WebSubmissionPlace error", e);
            return Response.status(500).entity("<error>" + e.getMessage() + "</error>").build();
        } catch (Exception e) {
            LOG.error("Error on {}", uuid, e);
            return Response.status(500).entity("<error>" + e.getMessage() + "</error>").build();

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
$get_path_handler(*  as $format_param1)
$get_path_handler(,* as $format_param2)

    `, ssaapi.QueryWithEnableDebug())
	require.NoError(t, err)
	res.Show()
}
