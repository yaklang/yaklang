package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	csharpSemanticCompileStressWidth = 12
	csharpSemanticCompileBudget      = 30 * time.Second
)

func TestCSharpSSAMultiFileSemanticCompileBudget(t *testing.T) {
	vf, sourceBytes := newCSharpSemanticCompileFixture()
	budget := csharpSemanticCompileBudget
	if testing.Short() {
		// Short-mode CI is commonly shared and noisier. Keep this a regression
		// guard against runaway compilation, rather than a machine-speed test.
		budget = 60 * time.Second
	}

	start := time.Now()
	programs, err := ssaapi.ParseProjectWithFS(
		vf,
		ssaapi.WithLanguage(ssaconfig.CSHARP),
		ssaapi.WithMemory(),
	)
	compileDuration := time.Since(start)

	require.NoError(t, err)
	require.NotEmpty(t, programs)
	requireCSharpSemanticCompileSSA(t, programs[0])
	require.LessOrEqual(t, compileDuration, budget,
		"four-file C# SSA compile exceeded the deliberately broad CI budget")
	t.Logf("C# SSA COMPILE files=4 bytes=%d duration=%s budget=%s",
		sourceBytes, compileDuration, budget)
}

func BenchmarkCSharpSSAMultiFileSemanticCompile(b *testing.B) {
	vf, sourceBytes := newCSharpSemanticCompileFixture()
	var lastProgram *ssaapi.Program

	b.ReportAllocs()
	b.SetBytes(int64(sourceBytes))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		programs, err := ssaapi.ParseProjectWithFS(
			vf,
			ssaapi.WithLanguage(ssaconfig.CSHARP),
			ssaapi.WithMemory(),
		)
		if err != nil {
			b.Fatal(err)
		}
		if len(programs) == 0 {
			b.Fatal("C# SSA compile returned no programs")
		}
		lastProgram = programs[0]
	}
	b.StopTimer()

	// ParseProjectWithFS is expected to build IR. Checking SSA opcodes keeps this
	// benchmark from silently becoming another frontend-only parser benchmark.
	requireCSharpSemanticCompileSSA(b, lastProgram)
}

func requireCSharpSemanticCompileSSA(tb testing.TB, prog *ssaapi.Program) {
	tb.Helper()
	require.NotNil(tb, prog)
	require.Empty(tb, csharpErrorKindErrors(prog), prog.GetErrors().String())

	for _, blueprint := range []string{"Order", "OrderRepository", "OrderService", "OrdersController"} {
		require.NotEmpty(tb, prog.Ref(blueprint),
			"cross-file blueprint %s must reach SSA", blueprint)
	}
	mainParameters := prog.Ref("args")
	sinks := prog.Ref("sink")
	sinkCalls := sinks.GetUsers()
	require.True(tb, csharpValuesContainOpcode(mainParameters, "Parameter"),
		"entrypoint parameter must be emitted as SSA IR: %s", mainParameters.String())
	require.GreaterOrEqual(tb, csharpValuesCountOpcode(sinkCalls, "Call"), csharpSemanticCompileStressWidth,
		"generated controller bodies must be emitted as SSA calls: %s", sinkCalls.String())
}

func csharpValuesContainOpcode(values ssaapi.Values, opcode string) bool {
	return csharpValuesCountOpcode(values, opcode) > 0
}

func csharpValuesCountOpcode(values ssaapi.Values, opcode string) int {
	count := 0
	for _, value := range values {
		if value != nil && value.GetOpcode() == opcode {
			count++
		}
	}
	return count
}

func newCSharpSemanticCompileFixture() (*filesys.VirtualFS, int) {
	var model strings.Builder
	model.WriteString(`
namespace SemanticCompile.Models {
    public class Order {
        public string Id;
        public string Customer;
        public int Quantity;
        public decimal Total;
        public string Status;

        public Order(string id, string customer, int quantity, decimal total) {
            Id = id;
            Customer = customer;
            Quantity = quantity;
            Total = total;
            Status = "new";
        }
`)
	for i := 0; i < csharpSemanticCompileStressWidth; i++ {
		fmt.Fprintf(&model, `
        public int Score%02d(int seed) {
            int score = seed;
            for (int i = 0; i < 3; i++) {
                score += Quantity + i;
            }
            return score;
        }
`, i)
	}
	model.WriteString(`
    }
}
`)

	var repository strings.Builder
	repository.WriteString(`
using SemanticCompile.Models;

namespace SemanticCompile.Data {
    public class OrderRepository {
`)
	for i := 0; i < csharpSemanticCompileStressWidth; i++ {
		fmt.Fprintf(&repository, `
        public Order Load%02d(string id) {
            audit(id);
            return new Order(id, "loaded", %d, 2);
        }

        public Order Save%02d(Order order) {
            persist(order);
            return order;
        }
`, i, i+1, i)
	}
	repository.WriteString(`
    }
}
`)

	var service strings.Builder
	service.WriteString(`
using SemanticCompile.Data;
using SemanticCompile.Models;

namespace SemanticCompile.Services {
    public class OrderService {
        private OrderRepository repository;

        public OrderService() {
            repository = new OrderRepository();
        }
`)
	for i := 0; i < csharpSemanticCompileStressWidth; i++ {
		fmt.Fprintf(&service, `
        public Order Place%02d(string customer, int quantity, decimal price) {
            decimal total = quantity * price;
            var order = new Order(makeId(customer), customer, quantity, total);
            if (order.Score%02d(quantity) > 100) {
                audit("review");
            } else {
                audit("accepted");
            }
            return repository.Save%02d(order);
        }

        public Order Lookup%02d(string id) {
            return repository.Load%02d(id);
        }
`, i, i, i, i, i)
	}
	service.WriteString(`
    }
}
`)

	var controller strings.Builder
	controller.WriteString(`
using SemanticCompile.Models;
using SemanticCompile.Services;

namespace SemanticCompile.Controllers {
    public class OrdersController {
        private OrderService service;

        public OrdersController() {
            service = new OrderService();
        }
`)
	for i := 0; i < csharpSemanticCompileStressWidth; i++ {
		fmt.Fprintf(&controller, `
        public Order Create%02d(string customer, int quantity, decimal price) {
            string normalized = normalize(customer);
            Order created = service.Place%02d(normalized, quantity, price);
            sink(created);
            return created;
        }

        public Order Get%02d(string id) {
            return service.Lookup%02d(id);
        }
`, i, i, i, i)
	}
	controller.WriteString(`
    }

    public class Program {
        public static void Main(string[] args) {
            var controller = new OrdersController();
            sink(controller);
        }
    }
}
`)

	files := []struct {
		path string
		code string
	}{
		{path: "Models/Order.cs", code: model.String()},
		{path: "Data/OrderRepository.cs", code: repository.String()},
		{path: "Services/OrderService.cs", code: service.String()},
		{path: "Controllers/OrdersController.cs", code: controller.String()},
	}

	vf := filesys.NewVirtualFs()
	sourceBytes := 0
	for _, file := range files {
		vf.AddFile(file.path, file.code)
		sourceBytes += len(file.code)
	}
	return vf, sourceBytes
}
