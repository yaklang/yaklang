package coreplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type sfProjectInfoSearcher struct {
	fs    filesys_interface.FileSystem
	local ypb.YakClient
	code  string

	progName string

	t *testing.T
}

func NewSfProjectInfoSearcher(fs filesys_interface.FileSystem, t *testing.T, opt ...ssaapi.Option) *sfProjectInfoSearcher {
	progName := uuid.NewString()
	client, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)
	{

		opt = append(opt,
			ssaapi.WithFileSystem(fs),
			ssaapi.WithProgramName(progName),
		)
		_, err := ssaapi.ParseProject(opt...)
		require.NoError(t, err)
		t.Cleanup(func() {
			log.Infof("delete program: %v", progName)
			// ssadb.DeleteProgram(ssadb.GetDB(), progName)
		})
	}
	_, err = ssaapi.FromDatabase(progName)
	require.NoError(t, err)

	pluginName := "SyntaxFlow 查询项目信息"
	initDB.Do(func() {
		yakit.InitialDatabase()
	})
	codeBytes := GetCorePluginData(pluginName)
	require.NotNilf(t, codeBytes, "无法从bindata获取: %v", pluginName)

	return &sfProjectInfoSearcher{
		fs:       fs,
		local:    client,
		progName: progName,
		code:     string(codeBytes),
		t:        t,
	}
}

func (s *sfProjectInfoSearcher) RunSearchAndCheck(kind string) {
	stream, err := s.local.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       s.code,
		PluginType: "yak",
		ExecParams: []*ypb.KVPair{
			{
				Key:   "progName",
				Value: s.progName,
			},
			{
				Key:   "kind",
				Value: kind,
			},
		},
	})
	require.NoError(s.t, err)
	result := new(msg)

	var process float64
	for {
		exec, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(s.t, err)
		}
		if exec.IsMessage {
			rawMsg := exec.GetMessage()
			fmt.Println("raw msg: ", string(rawMsg))
			json.Unmarshal(rawMsg, &result)

			if result.Type == "progress" {
				process = result.Content.Process
			}
		}
	}
	require.Equal(s.t, 1.0, process)
}

func (s *sfProjectInfoSearcher) Check(t *testing.T, kind string, resultId int, want map[string]string) {
	rets := sendSSAURL(t, s.local, resultId, s.progName, kind)
	spew.Dump(rets)

	got := lo.SliceToMap(rets, func(ret *ypb.YakURLResource) (string, string) {
		if ret.ResourceType != "value" {
			return "", ""
		}
		key := ret.ResourceName
		source, err := getRangeText(ret, s.local)
		require.NoError(t, err)
		return key, source
	})
	spew.Dump("got:", got)
	spew.Dump("want:", want)
	for name, source := range want {
		got, ok := got[name]
		require.True(t, ok, "not found: %v", name)
		require.Equal(t, source, got)
	}
}

func TestSFProjectInfoSearch(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `import java.util.*;
import java.util.stream.Collectors;

/**
 * 数据过滤服务类
 * 包含各种数据过滤功能
 */
public class DataFilterService {

    /**
     * 过滤空字符串
     */
    public List<String> filterEmptyStrings(List<String> data) {
        return data.stream()
                .filter(s -> s != null && !s.trim().isEmpty())
                .collect(Collectors.toList());
    }

    /**
     * 过滤数字数据
     */
    public List<Integer> filterPositiveNumbers(List<Integer> numbers) {
        return numbers.stream()
                .filter(n -> n > 0)
                .collect(Collectors.toList());
    }

    /**
     * 过滤偶数
     */
    public List<Integer> filterEvenNumbers(List<Integer> numbers) {
        return numbers.stream()
                .filter(n -> n % 2 == 0)
                .collect(Collectors.toList());
    }

    /**
     * 过滤用户年龄
     */
    public List<User> filterUsersByAge(List<User> users, int minAge, int maxAge) {
        return users.stream()
                .filter(user -> user.getAge() >= minAge && user.getAge() <= maxAge)
                .collect(Collectors.toList());
    }

    /**
     * 过滤活跃用户
     */
    public List<User> filterActiveUsers(List<User> users) {
        return users.stream()
                .filter(User::isActive)
                .collect(Collectors.toList());
    }

    /**
     * 过滤重复数据
     */
    public <T> List<T> filterDuplicates(List<T> data) {
        return data.stream()
                .distinct()
                .collect(Collectors.toList());
    }

    /**
     * 过滤文件扩展名
     */
    public List<String> filterFilesByExtension(List<String> filenames, String extension) {
        return filenames.stream()
                .filter(filename -> filename.toLowerCase().endsWith("." + extension.toLowerCase()))
                .collect(Collectors.toList());
    }
}

class User {
    private String name;
    private int age;
    private boolean active;

    public User(String name, int age, boolean active) {
        this.name = name;
        this.age = age;
        this.active = active;
    }

    public String getName() { return name; }
    public int getAge() { return age; }
    public boolean isActive() { return active; }
} 
`
	fs.AddFile("demo1.java", code1)
	code2 := `import java.util.*;
import java.util.stream.Collectors;

/**
 * 内容过滤处理器
 * 包含内容处理、过滤和检查功能
 */
public class ContentFilterProcessor {

    private static final Set<String> SENSITIVE_WORDS = new HashSet<>(
        Arrays.asList("敏感词1", "敏感词2", "不当内容", "违规词汇")
    );

    /**
     * 过滤敏感词汇
     */
    public String filterSensitiveContent(String content) {
        if (content == null) return null;
        String filtered = content;
        for (String word : SENSITIVE_WORDS) {
            filtered = filtered.replace(word, "***");
        }
        return filtered;
    }

    /**
     * 检查内容长度
     */
    public boolean checkContentLength(String content, int maxLength) {
        return content != null && content.length() <= maxLength;
    }

    /**
     * 过滤HTML标签
     */
    public String filterHtmlTags(String content) {
        if (content == null) return null;
        return content.replaceAll("<[^>]+>", "");
    }

    /**
     * 检查文件大小
     */
    public boolean checkFileSize(long fileSize, long maxSize) {
        return fileSize > 0 && fileSize <= maxSize;
    }

    /**
     * 过滤特殊字符
     */
    public String filterSpecialCharacters(String input) {
        if (input == null) return null;
        return input.replaceAll("[^a-zA-Z0-9\\u4e00-\\u9fa5\\s]", "");
    }

    /**
     * 检查权限
     */
    public boolean checkUserPermission(String userId, String permission) {
        // 模拟权限检查逻辑
        return userId != null && permission != null && !userId.isEmpty();
    }

    /**
     * 过滤空白行
     */
    public List<String> filterEmptyLines(List<String> lines) {
        return lines.stream()
                .filter(line -> line != null && !line.trim().isEmpty())
                .collect(Collectors.toList());
    }
} 
`
	fs.AddFile("demo2.java", code2)

	s := NewSfProjectInfoSearcher(fs, t, ssaapi.WithLanguage(ssaconfig.JAVA))
	s.RunSearchAndCheck("filterFunc")
}
