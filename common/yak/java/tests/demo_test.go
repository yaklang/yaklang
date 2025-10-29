package tests

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCaseForSimple(t *testing.T) {
	ssatest.Check(t, `
public class XXEController {

    @RequestMapping(value = "/one")
	public List<MetricBean> getMetricList(String domain) throws SQLException {
		String sql = String.format("select role from metrics.metric where domain_name = '%s'", domain);
		Connection connection = MetricDataSource.getConnection();
		PreparedStatement preparedStatement = connection.prepareStatement(sql);
		List<MetricBean> MetricList = new ArrayList<>();
		ResultSet resultSet = preparedStatement.executeQuery();
		preparedStatement.close();
		connection.close();
		return tdsqlMetricList;
	}
}
`, func(prog *ssaapi.Program) error {
		a := prog.SyntaxFlowChain("MetricDataSource.getConnection().prepareState*(* #-> * as $source)").Show().DotGraph()
		fmt.Println(string(a))
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
