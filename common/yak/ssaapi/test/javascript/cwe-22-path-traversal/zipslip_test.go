package cwe22pathtraversal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

// loadZipSlipRule 从内置 embed FS 按路径读取 js-zipslip.sf 规则内容。
// 若规则文件不在当前构建的 embed FS 中（如 irify_exclude 构建模式），则跳过测试。
func loadZipSlipRule(t *testing.T) string {
	t.Helper()
	content, ok := sfbuildin.GetEmbedRuleContent("ecmascript/cwe-22-path-traversal/js-zipslip.sf")
	if !ok {
		t.Skip("ecmascript/cwe-22-path-traversal/js-zipslip.sf 不在当前构建的 embed FS 中，跳过测试")
	}
	require.NotEmpty(t, content, "js-zipslip.sf 内容为空")
	return content
}

// runOnFile 用单文件 VirtualFS 执行规则，返回 (totalAlerts, highAlerts)。
func runOnFile(t *testing.T, ruleContent, filename, code string) (total, high int) {
	t.Helper()
	vfs := filesys.NewVirtualFs()
	vfs.AddFile(filename, code)

	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		require.Greater(t, len(programs), 0, "SSA 编译应至少产生一个程序")
		result, err := programs[0].SyntaxFlowWithError(ruleContent)
		require.NoError(t, err, "规则执行不应报错")
		for _, varName := range result.GetAlertVariables() {
			vals := result.GetValues(varName)
			total += len(vals)
			if info, ok := result.GetAlertInfo(varName); ok {
				if info.Severity == "high" || info.Severity == "h" {
					high += len(vals)
				}
			}
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JS))
	return
}

func TestZipSlip_UnzipBad(t *testing.T) {
	rule := loadZipSlipRule(t)
	total, high := runOnFile(t, rule, "unzip-bad.js", `
const fs = require('fs');
const unzip = require('unzip');

fs.createReadStream('archive.zip')
  .pipe(unzip.Parse())
  .on('entry', entry => {
    const fileName = entry.path;
    // BAD: This could write any file on the filesystem.
    entry.pipe(fs.createWriteStream(fileName));
  });
`)
	assert.Greater(t, total, 0, "应触发告警（漏报）")
	assert.Greater(t, high, 0, "应触发 high 告警")
}

func TestZipSlip_UnzipGood(t *testing.T) {
	rule := loadZipSlipRule(t)
	total, _ := runOnFile(t, rule, "unzip-good.js", `
const fs = require('fs');
const unzip = require('unzip');

fs.createReadStream('archive.zip')
  .pipe(unzip.Parse())
  .on('entry', entry => {
    const fileName = entry.path;
    // GOOD: ensures the path is safe to write to.
    if (fileName.indexOf('..') == -1) {
      entry.pipe(fs.createWriteStream(fileName));
    } else {
      console.log('skipping bad path', fileName);
    }
  });
`)
	assert.Equal(t, 0, total, "indexOf('..')  守卫不应误报")
}

func TestZipSlip_AdmZipBad(t *testing.T) {
	rule := loadZipSlipRule(t)
	total, high := runOnFile(t, rule, "admzip-bad.js", `
const fs = require('fs');
const AdmZip = require('adm-zip');

const zip = new AdmZip('test.zip');
const zipEntry = zip.getEntry('file');

if (zipEntry) {
  const entryName = zipEntry.entryName;
  fs.writeFile(entryName, entryName, (err) => {
    if (err) {
      console.error('Error writing to file:', err);
    } else {
      console.log('Entry name written to entryName.txt');
    }
  });
} else {
  console.log('Entry not found.');
}
`)
	assert.Greater(t, total, 0, "应触发告警（漏报）")
	assert.Greater(t, high, 0, "entryName 直接流向 writeFile，应触发 high 告警")
}

func TestZipSlip_AdmZipGood(t *testing.T) {
	rule := loadZipSlipRule(t)
	total, _ := runOnFile(t, rule, "admzip-good.js", `
const fs = require('fs');
const path = require('path');
const AdmZip = require('adm-zip');

const zip = new AdmZip('test.zip');
const zipEntry = zip.getEntry('file');

if (zipEntry) {
  const entryName = zipEntry.entryName;
  // GOOD: path.basename strips directory components.
  const sanitizedName = path.basename(entryName);
  fs.writeFile(sanitizedName, zipEntry.getData(), (err) => {
    if (err) {
      console.error('Error writing to file:', err);
    } else {
      console.log('Entry ' + sanitizedName + ' written successfully');
    }
  });
} else {
  console.log('Entry not found.');
}
`)
	assert.Equal(t, 0, total, "path.basename 净化后不应误报")
}

func TestZipSlip_JSZipBad(t *testing.T) {
	rule := loadZipSlipRule(t)
	total, high := runOnFile(t, rule, "jszip-bad.js", `
const JSZip = require('jszip');
const fs = require('fs');
const path = require('path');

fs.readFile('test.zip', (err, data) => {
  if (err) {
    console.error('读取 zip 文件时出错:', err);
    return;
  }

  JSZip.loadAsync(data)
    .then((zip) => {
      zip.forEach((relativePath, zipEntry) => {
        const sanitizedName = relativePath; // 漏洞：直接使用来自压缩包的路径

        if (!zipEntry.dir) {
          // 第三层 .then()：sanitizedName 跨闭包成为 FreeValue，其 default 指向 relativePath
          zipEntry.async('nodebuffer').then((content) => {
            fs.writeFile(sanitizedName, content, (err) => {
              if (err) {
                console.error('写入文件 ' + sanitizedName + ' 时出错:', err);
              } else {
                console.log('文件 ' + sanitizedName + ' 提取成功');
              }
            });
          });
        } else {
          console.log('跳过目录: ' + sanitizedName);
        }
      });
    })
    .catch((err) => {
      console.error('解压 zip 文件时出错:', err);
    });
});
`)
	assert.Greater(t, total, 0, "应触发告警（漏报）")
	assert.Greater(t, high, 0, "应触发告警（漏报）")
}

func TestZipSlip_JSZipGood(t *testing.T) {
	rule := loadZipSlipRule(t)
	total, _ := runOnFile(t, rule, "jszip-good.js", `
const JSZip = require('jszip');
const fs = require('fs');
const path = require('path');

// 读取 zip 文件（你可以将此替换为文件缓冲区或文件流）
fs.readFile('test.zip', (err, data) => {
  if (err) {
    console.error('读取 zip 文件时出错:', err);
    return;
  }

  // 创建 JSZip 实例并加载 zip 数据
  JSZip.loadAsync(data)
    .then((zip) => {
      // 使用 zip.forEach 遍历 zip 文件中的每个条目
      zip.forEach((relativePath, zipEntry) => {
        // 使用 path.basename 确保文件名不包含目录遍历
        const sanitizedName = path.basename(relativePath);

        // 判断当前条目是否是文件（而不是目录）
        if (!zipEntry.dir) {
          // 使用 zipEntry.async 获取文件内容并将其写入本地文件系统
          zipEntry.async('nodebuffer').then((content) => {
            fs.writeFile(sanitizedName, content, (err) => {
              if (err) {
                console.error(err);
              } else {
                console.log("文件 ${sanitizedName} 提取成功");
              }
            });
          });
        } else {
          console.log("跳过目录: ${sanitizedName}");
        }
      });
    })
    .catch((err) => {
      console.error('解压 zip 文件时出错:', err);
    });
});
`)
	assert.Equal(t, total, 0, "不应触发告警（误报）")
}
