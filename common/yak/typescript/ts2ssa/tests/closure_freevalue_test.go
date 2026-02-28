package tests

// TestClosureFreeValue 验证 arrow function lazy build 时，内层 closure 能正确捕获外层
// local const/let 变量为 FreeValue，而不是生成 Undefined。
//
// Bug 背景（已修复，见 build_from_ast.go VisitArrowFunction）：
//
// ts2ssa 对 arrow function 使用 lazy build 机制：
//   1. 遇到 arrow function 时，先用 StoreFunctionBuilder() 快照当前 builder 状态，
//      再将真正的编译逻辑注册为 lazy task（不立刻执行）。
//   2. 所有 AST 遍历完成后，统一执行所有 lazy task。
//
// StoredFunctionBuilder 有两个字段：
//   - Current *FunctionBuilder  活指针，指向父函数的 builder 对象本身
//   - Store   *FunctionBuilder  值快照，保存注册时 builder 的部分字段（含 CurrentBlock）
//
// 问题时间线（以三层嵌套为例）：
//
//   T1: 编译第二层 closure（forEach 回调）
//       执行 `const sanitizedName = relativePath`
//       → sanitizedName 被写入当前 BasicBlock（SubBlock）的 ScopeTable
//       执行 `zipEntry.async(...).then(innerArrow)` → 调用 VisitArrowFunction(innerArrow)
//       → store3 = StoreFunctionBuilder()
//         store3.Current = b.FunctionBuilder          (活指针)
//         store3.Store.CurrentBlock = b.CurrentBlock  (快照 = SubBlock，含 sanitizedName)
//       → 注册第三层 lazy task，暂时返回
//
//   T2: 第二层 closure 继续编译：
//       BuildSyntaxBlock 结束 → b.CurrentBlock 切换到 EndBlock
//       EndBlock.ScopeTable 是 shadow scope，不含 sanitizedName（块级作用域隔离）
//       b.Finish() → 第二层编译完成
//
//   T3: LazyBuild 执行第三层 lazy task：
//       SwitchFunctionBuilder(store3) → b.FunctionBuilder = store3.Current (活指针)
//       此时 store3.Current.CurrentBlock = EndBlock（T2 时已更新）
//       PushFunction(newFunc) → parentScope 基于 EndBlock 构建，不含 sanitizedName
//       getParentFunctionVariable("sanitizedName") → 找不到 → 生成 Undefined
//
// 修复方案：
//   在 PushFunction 调用前，用 store.Store.CurrentBlock（注册时快照的 SubBlock）
//   临时替换活指针上已失效的 EndBlock，确保 parentScope 正确包含父函数体内的 local 变量。

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// TestClosureFreeValue_TwoLevel 验证两层闭包正常工作（基线，修复前后均应通过）。
func TestClosureFreeValue_TwoLevel(t *testing.T) {
	t.Parallel()

	// forEach 回调内直接使用参数，内层不涉及额外 closure
	ssatest.CheckSyntaxFlow(t, `
const fs = require('fs');
[1].forEach((relativePath) => {
  fs.writeFile(relativePath, 'x', () => {});
});
`, `fs.writeFile(* as $path,)`, map[string][]string{
		"path": {"Parameter-relativePath"},
	}, ssaapi.WithLanguage(ssaconfig.JS))
}

// TestClosureFreeValue_ThreeLevel_DirectParam 验证三层嵌套时，内层 closure 能捕获
// 中间层的形参（Parameter）为 FreeValue。这是修复的核心场景之一。
func TestClosureFreeValue_ThreeLevel_DirectParam(t *testing.T) {
	t.Parallel()

	// 第三层直接引用第二层的形参 relativePath
	ssatest.CheckSyntaxFlow(t, `
const fs = require('fs');
[1].forEach((relativePath) => {
  Promise.resolve().then((content) => {
    fs.writeFile(relativePath, content, () => {});
  });
});
`, `fs.writeFile(* as $path,)`, map[string][]string{
		"path": {"FreeValue-relativePath"},
	}, ssaapi.WithLanguage(ssaconfig.JS))
}

// TestClosureFreeValue_ThreeLevel_LocalConst 验证三层嵌套时，内层 closure 能捕获
// 中间层定义的 local const 变量为 FreeValue（修复前此处生成 Undefined）。
//
// 修复前：fs.writeFile 的路径参数 = Undefined-x（无 DependOn）
// 修复后：fs.writeFile 的路径参数 = FreeValue-x（default → Parameter-relativePath）
func TestClosureFreeValue_ThreeLevel_LocalConst(t *testing.T) {
	t.Parallel()

	ssatest.CheckSyntaxFlow(t, `
const fs = require('fs');
[1].forEach((relativePath) => {
  const x = relativePath;
  Promise.resolve().then((content) => {
    fs.writeFile(x, content, () => {});
  });
});
`, `fs.writeFile(* as $path,)`, map[string][]string{
		"path": {"FreeValue-x"},
	}, ssaapi.WithLanguage(ssaconfig.JS))
}

// TestClosureFreeValue_ThreeLevel_LocalLet 验证 let 声明的变量同样能正确捕获。
func TestClosureFreeValue_ThreeLevel_LocalLet(t *testing.T) {
	t.Parallel()

	ssatest.CheckSyntaxFlow(t, `
const fs = require('fs');
[1].forEach((relativePath) => {
  let outputPath = relativePath;
  Promise.resolve().then((content) => {
    fs.writeFile(outputPath, content, () => {});
  });
});
`, `fs.writeFile(* as $path,)`, map[string][]string{
		"path": {"FreeValue-outputPath"},
	}, ssaapi.WithLanguage(ssaconfig.JS))
}

// TestClosureFreeValue_ThreeLevel_MultipleVars 验证中间层定义了多个 local 变量时，
// 每个变量都能在第三层 closure 中被正确捕获。
func TestClosureFreeValue_ThreeLevel_MultipleVars(t *testing.T) {
	t.Parallel()

	// a 和 b 都在第二层定义，第三层应该都能捕获
	ssatest.CheckSyntaxFlow(t, `
const fs = require('fs');
const path = require('path');
[1].forEach((relativePath) => {
  const a = relativePath;
  const b = path.basename(relativePath);
  Promise.resolve().then((content) => {
    fs.writeFile(a, content, () => {});
    fs.writeFile(b, content, () => {});
  });
});
`, `fs.writeFile(* as $path,)`, map[string][]string{
		"path": {"FreeValue-a", "FreeValue-b"},
	}, ssaapi.WithLanguage(ssaconfig.JS))
}

// TestClosureFreeValue_FourLevel 验证四层嵌套（比三层更深）时，local 变量仍能正确捕获。
// 第二层定义 x，第四层（隔两层）使用 x。
func TestClosureFreeValue_FourLevel(t *testing.T) {
	t.Parallel()

	ssatest.CheckSyntaxFlow(t, `
const fs = require('fs');
[1].forEach((relativePath) => {
  const x = relativePath;
  Promise.resolve().then((_) => {
    Promise.resolve().then((content) => {
      fs.writeFile(x, content, () => {});
    });
  });
});
`, `fs.writeFile(* as $path,)`, map[string][]string{
		"path": {"FreeValue-x"},
	}, ssaapi.WithLanguage(ssaconfig.JS))
}

// TestClosureFreeValue_JSZip_LoadAsync 验证 JSZip.loadAsync().then().forEach() 三层
// Promise 链中，relativePath → sanitizedName（local const）→ 内层 .then() 的完整链路。
// 这是触发此 bug 的原始业务场景（Zip Slip 安全规则检测）。
func TestClosureFreeValue_JSZip_LoadAsync(t *testing.T) {
	t.Parallel()

	// sanitizedName 在 forEach 回调（第二层）定义，在 zipEntry.async().then()（第三层）使用
	ssatest.CheckSyntaxFlow(t, `
const JSZip = require('jszip');
const fs = require('fs');

fs.readFile('test.zip', (err, data) => {
  JSZip.loadAsync(data).then((zip) => {
    zip.forEach((relativePath, zipEntry) => {
      const sanitizedName = relativePath;
      zipEntry.async('nodebuffer').then((content) => {
        fs.writeFile(sanitizedName, content, (err) => {});
      });
    });
  });
});
`, `fs.writeFile(* as $path,)`, map[string][]string{
		"path": {"FreeValue-sanitizedName"},
	}, ssaapi.WithLanguage(ssaconfig.JS))
}

// TestClosureFreeValue_RegularFunction_ThreeLevel 验证普通 function 关键字三层嵌套时，
// 最内层能否正确捕获外层变量。
//
// 注意：CheckPrintlnValue 验证的是 println 参数在 SSA 中的直接表示，
// 不做常量追踪。因此期望值是 SSA Value 的字符串形式：
//   - 当内层 closure 捕获外层变量时，SSA 生成 FreeValue-xxx（正确）
//   - 若捕获失败，则会生成 Undefined-xxx（说明闭包捕获有问题）
//   - 直接形参为 Parameter-z
func TestClosureFreeValue_RegularFunction_ThreeLevel(t *testing.T) {
	t.Parallel()

	t.Run("regular function three level - outer param captured", func(t *testing.T) {
		// 验证：inner 能捕获 outer 的 localX（FreeValue）和 middle 的 localY（FreeValue）
		// 若捕获失败会变成 Undefined-localX / Undefined-localY
		ssatest.CheckPrintlnValue(`
function outer(x) {
  const localX = x;
  function middle(y) {
    const localY = y;
    function inner(z) {
      println(localX);
      println(localY);
      println(z);
    }
    inner(333333);
  }
  middle(222222);
}
outer(111111);
`, []string{"FreeValue-localX", "FreeValue-localY", "Parameter-z"}, t)
	})

	t.Run("regular function three level - outer var captured", func(t *testing.T) {
		// 全局 var + 两层嵌套函数，最内层捕获所有外层变量
		ssatest.CheckPrintlnValue(`
var outerVal = 111111;
function level1() {
  var midVal = 222222;
  function level2() {
    var innerVal = 333333;
    function level3() {
      println(outerVal);
      println(midVal);
      println(innerVal);
    }
    level3();
  }
  level2();
}
level1();
`, []string{"FreeValue-outerVal", "FreeValue-midVal", "FreeValue-innerVal"}, t)
	})

	t.Run("regular function three level - mixed arrow and regular", func(t *testing.T) {
		// 外层普通函数 + 中间 arrow function + 内层普通函数
		// arrow function 使用 lazy build，普通 function 立即编译
		// 混合场景下各层变量均应能正确捕获
		ssatest.CheckPrintlnValue(`
function outer(x) {
  const localX = x;
  const middle = (y) => {
    const localY = y;
    function inner(z) {
      println(localX);
      println(localY);
      println(z);
    }
    inner(333333);
  };
  middle(222222);
}
outer(111111);
`, []string{"FreeValue-localX", "FreeValue-localY", "Parameter-z"}, t)
	})

	t.Run("all arrow three level - local const captured", func(t *testing.T) {
		// 全部使用 arrow function（均走 lazy build），中间层定义 local const
		// 这是 build_from_ast.go 修复的核心场景
		ssatest.CheckPrintlnValue(`
const outer = (x) => {
  const localX = x;
  const middle = (y) => {
    const localY = y;
    const inner = (z) => {
      println(localX);
      println(localY);
      println(z);
    };
    inner(333333);
  };
  middle(222222);
};
outer(111111);
`, []string{"FreeValue-localX", "FreeValue-localY", "Parameter-z"}, t)
	})
}

// TestClosureFreeValue_NotAffectTwoLevel 确认修复不影响两层 closure 的正常行为：
// 内层直接使用外层参数，应该仍是 FreeValue-relativePath（而非 Undefined）。
func TestClosureFreeValue_NotAffectTwoLevel(t *testing.T) {
	t.Parallel()

	ssatest.CheckSyntaxFlow(t, `
const JSZip = require('jszip');
const fs = require('fs');

fs.readFile('test.zip', (err, data) => {
  JSZip.loadAsync(data).then((zip) => {
    zip.forEach((relativePath, zipEntry) => {
      if (!zipEntry.dir) {
        fs.writeFile(relativePath, 'content', (err) => {});
      }
    });
  });
});
`, `fs.writeFile(* as $path,)`, map[string][]string{
		"path": {"Parameter-relativePath"},
	}, ssaapi.WithLanguage(ssaconfig.JS))
}
