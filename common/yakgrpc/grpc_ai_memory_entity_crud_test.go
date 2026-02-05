package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAIMemoryEntity_CRUD_DBOnly(t *testing.T) {
	// isolate yakit dirs

	client, srv, err := NewLocalClientAndServerWithTempDatabase(t)

	require.NoError(t, err)
	ctx := context.Background()

	db := srv.GetProjectDatabase()
	sessionID := "test-session"
	now := time.Now()

	// Create (DB)
	e1 := &schema.AIMemoryEntity{
		Model: gorm.Model{
			CreatedAt: now.Add(-2 * time.Minute),
			UpdatedAt: now.Add(-2 * time.Minute),
		},
		MemoryID:           "m1",
		SessionID:          sessionID,
		Content:            "hello yaklang",
		Tags:               schema.StringArray{"yaklang", "grpc"},
		PotentialQuestions: schema.StringArray{"what is yaklang?"},
	}
	e2 := &schema.AIMemoryEntity{
		Model: gorm.Model{
			CreatedAt: now.Add(-1 * time.Minute),
			UpdatedAt: now.Add(-1 * time.Minute),
		},
		MemoryID:  "m2",
		SessionID: sessionID,
		Content:   "another memory",
		Tags:      schema.StringArray{"grpc"},
	}
	require.NoError(t, db.Create(e1).Error)
	require.NoError(t, db.Create(e2).Error)

	// Read (gRPC Get)
	got, err := client.GetAIMemoryEntity(ctx, &ypb.GetAIMemoryEntityRequest{
		SessionID: sessionID,
		MemoryID:  "m1",
	})
	require.NoError(t, err)
	require.Equal(t, "m1", got.GetMemoryID())
	require.Equal(t, sessionID, got.GetSessionID())
	require.Equal(t, "hello yaklang", got.GetContent())
	require.ElementsMatch(t, []string{"yaklang", "grpc"}, got.GetTags())

	// Query (gRPC Query, non-vector path)
	q1, err := client.QueryAIMemoryEntity(ctx, &ypb.QueryAIMemoryEntityRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "memory_id", Order: "asc"},
		Filter: &ypb.AIMemoryEntityFilter{
			SessionID:                sessionID,
			ContentKeyword:           "yaklang",
			TagMatchAll:              false,
			PotentialQuestionKeyword: "",
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), q1.GetTotal())
	require.Len(t, q1.GetData(), 1)
	require.Equal(t, "m1", q1.GetData()[0].GetMemoryID())

	// Update (gRPC Update)
	_, err = client.UpdateAIMemoryEntity(ctx, &ypb.AIMemoryEntity{
		MemoryID:  "m1",
		SessionID: sessionID,
		Content:   "hello yaklang updated",
		Tags:      []string{"yaklang", "crud"},
	})
	require.NoError(t, err)

	got2, err := client.GetAIMemoryEntity(ctx, &ypb.GetAIMemoryEntityRequest{
		SessionID: sessionID,
		MemoryID:  "m1",
	})
	require.NoError(t, err)
	require.Equal(t, "hello yaklang updated", got2.GetContent())
	require.ElementsMatch(t, []string{"yaklang", "crud"}, got2.GetTags())

	// Delete (gRPC Delete)
	_, err = client.DeleteAIMemoryEntity(ctx, &ypb.DeleteAIMemoryEntityRequest{
		Filter: &ypb.AIMemoryEntityFilter{
			SessionID: sessionID,
			MemoryID:  []string{"m1"},
		},
	})
	require.NoError(t, err)

	_, err = client.GetAIMemoryEntity(ctx, &ypb.GetAIMemoryEntityRequest{
		SessionID: sessionID,
		MemoryID:  "m1",
	})
	require.Error(t, err)
}

func TestAIMemoryEntity_CountTags_DBOnly(t *testing.T) {
	client, srv, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)
	ctx := context.Background()

	db := srv.GetProjectDatabase()
	sessionID := "test-session"

	require.NoError(t, db.Create(&schema.AIMemoryEntity{
		MemoryID:  "m1",
		SessionID: sessionID,
		Content:   "hello yaklang",
		Tags:      schema.StringArray{"yaklang", "grpc"},
	}).Error)
	require.NoError(t, db.Create(&schema.AIMemoryEntity{
		MemoryID:  "m2",
		SessionID: sessionID,
		Content:   "another memory",
		Tags:      schema.StringArray{"grpc"},
	}).Error)
	require.NoError(t, db.Create(&schema.AIMemoryEntity{
		MemoryID:  "m3",
		SessionID: sessionID,
		Content:   "dup tags",
		Tags:      schema.StringArray{"grpc", "tag2"},
	}).Error)

	resp, err := client.CountAIMemoryEntityTags(ctx, &ypb.CountAIMemoryEntityTagsRequest{
		SessionID: sessionID,
	})
	require.NoError(t, err)

	require.Equal(t, []*ypb.TagsCode{
		{Value: "grpc", Total: 3},
		{Value: "tag2", Total: 1},
		{Value: "yaklang", Total: 1},
	}, resp.GetTagsCount())
}

func TestMemoryFilling(t *testing.T) {
	t.Skip()
	memory, err := aimem.NewAIMemory("default", aimem.WithAutoReActInvoker())
	require.NoError(t, err)
	for _, corpus := range testCorpus {
		memory.HandleMemory(corpus)
	}
	for _, corpus := range testCorpus {
		memory.HandleMemory(corpus)
	}
	for _, corpus := range testCorpus {
		memory.HandleMemory(corpus)
	}
	for _, corpus := range testCorpus {
		memory.HandleMemory(corpus)
	}
}

var testCorpus = []string{
	"第一条：在代号为“青鸟”的秘密行动中，特工 A 计划在 2024 年 10 月 10 日前往巴黎。他必须在香榭丽舍大街的第三家咖啡馆与联络员见面，接头暗号是“今晚的月色真美”。咖啡馆的桌子上会放一本皮质封面的旧笔记，里面记录了关于量子芯片的初步设计逻辑，这关系到未来十年的能源安全，任何偏差都可能导致实验失败。",
	"第二条：关于新型号自研处理器 X1 的技术规格说明。该芯片采用 3nm 工艺，拥有 12 个高性能核心和 4 个能效核心。其 L3 缓存达到了 64MB，专门针对 AI 大模型推理进行了优化。在热设计功耗（TDP）方面，工程师将其控制在 35W 以内，这使得它在轻薄本上也能发挥出极强的性能，预计在明年的春季发布会上正式亮相，取代现有的 Pro 系列。",
	"第三条：王奶奶家的小猫“糯米”昨天下午在后花园失踪了。糯米是一只纯白色的布偶猫，左耳尖有一撮灰色的毛，脖子上戴着一个刻有联系电话的金铃铛。据邻居回忆，当时看到一只形似糯米的猫在追逐一只蓝色的蝴蝶，最后消失在围墙边的丁香丛里。王奶奶非常焦急，已经张贴了多张寻猫启示，并承诺提供一箱高级猫罐头作为酬谢。",
	"第四条：深海勘探船“探索者号”在马里亚纳海沟 8000 米处发现了一种未知的生物发光现象。这种光呈现出淡紫色，且具有规律的脉冲频率，每分钟闪烁 12 次。科学家初步推测这可能是一种深海生物的通讯方式，或者与某种海底矿脉的电磁感应有关。实验室正在对采集的水样进行化学分析，试图寻找氨基酸存在的证据，以证明此处存在独立的生态系统。",
	"第五条：20 世纪初的伦敦，雾气氤氲。私家侦探林德遇接手了一宗奇怪的失窃案：博物馆里最珍贵的“亚历山大之星”蓝宝石不翼而飞，但展柜的红外线报警系统全程没有触发。现场只留下了一枚散发着薰衣草香味的紫色丝绒手套。林德遇发现，手套内侧绣着一个模糊的字母“M”，这让他联想到了消失已久的怪盗家族，案情变得愈发扑朔迷离。",
	"第六条：气象局发布紧急预警，超强台风“海燕”预计在未来 48 小时内从东南沿海登陆。监测数据显示，台风中心气压为 940 百帕，近中心最大风力达 16 级。专家建议沿海地区的渔船立即回港避风，城市管理部门需加强排水系统的疏通，防止短时强降水引发的城市内涝。此外，受台风外围环流影响，内陆省份也将迎来 100 毫米以上的暴雨过程，需防范山体滑坡等次生灾害。",
	"第七条：这里是一段用于模拟 log 解析的文本：[2024-05-12 14:32:01] ERROR: database connection failed on port 5432. User 'admin' attempted to access 'schema_prod'. 系统自动重试机制在 5 秒后启动，但依然返回 403 Forbidden 错误。运维值班员张伟收到报警短信后，正在前往数据中心检查物理防火墙的配置。该问题初步判定为 SSL 证书过期导致的握手失败，涉及域名 api.internal.cloud.com。",
	"第八条：在偏远的喜马拉雅山脉，当地人传说有一种名为“雪晶”的矿石，能在黑夜中发出温润的绿光。地理学家史密斯教授对此深感兴趣，他带着一支六人的探险队，携带了最新的地质扫描仪和高能量补给包，于今年 1 月份从大本营出发。他们此行的目标是寻找矿石的矿脉，并采样分析其分子结构是否包含某种能延缓细胞衰老的放射性同位素，这被视为生物医学界的重大发现。",
	"第九条：Recipe for Sourdough Bread: To achieve a perfect crust, you need 500g of bread flour, 350g of filtered water, and 100g of active starter. 关键步骤在于长达 12 小时的低温发酵（Cold Proofing）。在放入烤箱之前，使用锋利的刀片在面团表面划出“十字”切口。烤箱预热至 230°C，并向烤盘内喷洒少量水蒸气。这种方法不仅能增加面包的内部孔隙，还能让表皮呈现出迷人的焦糖色泽，口感更有韧性。",
	"第十条：由于 2024 年度的财务预算调整，公司决定取消原定于 8 月份的团建活动。原本拨付给人力资源部的 50 万元资金将重新分配：30% 用于升级服务器架构，40% 用于员工的技术技能培训补贴，剩下的 30% 则作为年底的绩效奖金池。首席财务官强调，在当前市场环境下，提升内部研发效率和基础设施的稳定性比短期福利更为重要，各部门经理需在下周一前提交详细的预算执行计划。",
	"第十一条：在 A 大陆的编年史中，1402 年被称为“铁与火之年”。当时的北方王国领主爱德华三世通过联姻手段，合并了邻近的埃尔多兰领地。然而，这一举动引发了南方联盟的不满，双方在断剑峡谷展开了为期三个月的拉锯战。史书记录，那场战役中首次出现了重型投石机，改变了攻城战的规则。最终，双方签署了《落日条约》，确立了沿河而治的和平格局，维持了约五十年的安宁。",
	"第十二条：这是一封来自未来的加密邮件片段。发件人标记为“2077”，主题是“莫比乌斯环”。正文提到：我们已经成功将人类意识上传至云端服务器，但由于存储空间的碎片化，部分记忆出现了断层。例如，人们普遍忘记了 2025 年发生的‘数字极光’事件。如果你收到了这封信，请务必找到位于旧金山地下仓库的 04 号硬盘，那里存储着唯一的物理备份，暗号是第六条提到的台风名字。",
	"第十三条：心理学实验报告：参与者被要求在 30 秒内记住 20 个随机排列的数字。对照组在安静环境下进行，实验组则伴有 85 分贝的白噪音。结果显示，白噪音虽然在初期会干扰短时记忆，但对于某些特定类型的逻辑推理任务，适度的背景噪音反而有助于提高专注力。这种现象被称为“随机共振”。实验室下一步将引入脑电图（EEG）设备，实时监测参与者在噪音环境下的前额叶皮层活动。",
	"第十四条：这是一段关于 C++ 内存管理的说明。在现代开发中，应尽量避免使用原生指针（Raw Pointers），转而使用 std::unique_ptr 或 std::shared_ptr 以减少内存泄漏的风险。当一个对象的所有权非常明确时，unique_ptr 是性能最优的选择。我们需要注意循环引用问题，这通常通过 weak_ptr 来打破。在编写高效的总结算法时，AI 必须理解这些智能指针的析构时机，否则在处理大规模数据集时，程序可能会因为 OOM 而崩溃。",
	"第十五条：王平的日常安排表：早晨 6:30 起床晨跑 5 公里，早餐是两片全麦面包和一杯黑咖啡。8:00 准时搭乘地铁 2 号线前往软件园。在公司，他主要负责维护 legacy 系统，每天需要处理超过 50 个 Jira 工单。午休时间他喜欢看关于量子物理的科普书籍。晚上下班后，他会在 20:00 前往拳击馆训练一小时。这种高度自律的生活已经持续了三年，他认为这有助于缓解程序员的职业焦虑。",
	"第十六条：关于月球背面建立永久性科研基地的可行性报告。由于月球背面完全屏蔽了来自地球的无线电干扰，这里是进行低频射电天文观测的绝佳地点。初步方案建议使用 3D 打印技术，利用月表岩屑（Regolith）作为建筑材料，建造充气式栖息舱。能源供应将依靠部署在拉格朗日 L2 点的太阳能卫星中转。该计划预计耗资 400 亿美元，由国际航天联盟共同出资，分三个阶段在未来二十年内完成。",
	"第十七条：如果您在安装本软件时遇到 0x800F081F 错误代码，通常意味着系统中缺少 .NET Framework 3.5 组件。解决办法：打开控制面板，进入‘启用或关闭 Windows 功能’，勾选相应选项并点击确定。如果在线下载失败，您需要插入原始安装介质或使用 DISM 命令行工具进行离线修复。请确保您的网络环境没有设置全局代理，否则可能会导致 Windows Update 服务器连接超时，无法获取必要的元数据。",
	"第十八条：一位园艺爱好者的笔记：春天是修剪月季的最佳时期。我发现“果汁阳台”这个品种非常勤花，但容易感染黑斑病。每周喷洒一次多菌灵或代森锰锌能有效预防。浇水要遵循‘见干见湿’的原则，切忌盆内积水。昨天我尝试给白色的铁线莲压条，希望能繁育出几盆新的苗。邻居张大爷送了我一些腐熟的羊粪肥，说是对花芽分化很有帮助。希望今年的花期能延长到六月底，开出一片花海。",
	"第十九条：这是一段讽刺小品剧本：甲：‘现在的专家建议，为了健康，我们每天应该睡 8 小时，工作 8 小时，还要锻炼 2 小时。’ 乙：‘那剩下的 6 小时呢？’ 甲：‘剩下的 6 小时用来反思为什么每天只有 24 小时。’ 乙：‘我还以为你要说用来在地铁上刷短视频呢。’ 剧本揭示了当代都市人面对生活建议时的无力感，以及碎片化信息对深度思考的侵蚀。这种幽默感是 AI 很难在没有语境的情况下完全理解的。",
	"第二十条：在量子通信领域，贝尔态（Bell State）描述了两比特系统中最简单的量子纠缠形式。通过测量其中一个比特，我们可以瞬间确定另一个比特的状态，无论它们相距多远。这种‘幽灵般的超距作用’是量子密钥分发（QKD）的基础。目前的实验记录已经将纠缠分发的距离提升到了 1200 公里以上（通过墨子号卫星）。这对于构建绝对安全的全球通信网络至关重要，任何监听行为都会不可避免地改变量子态，从而被系统察觉。",
	"第二十一条：这里有一串加密字符序列：7B 22 75 73 65 72 49 44 22 3A 20 22 41 49 39 39 35 32 37 22 7D。如果将其从 Hex 转换为字符串，你会得到一个包含用户 ID 的 JSON 对象。这个 ID 是进入第十二条提到的地下仓库的关键凭证。此外，系统管理员提醒，每隔 24 小时该 ID 就会重新生成。如果您发现无法登录，请检查您的系统时钟是否与 NTP 服务器同步。测试点在于 AI 能否跨越十条信息发现这个潜在的关联逻辑。",
	"第二十二条：在南美洲的亚马逊雨林深处，当地部落使用一种名为“卡皮”的植物提取物进行仪式。植物学家发现，这种提取物中含有一种罕见的生物碱，能够暂时改变大脑处理视觉信号的方式。研究团队在获得当地许可后，采集了样本带回实验室进行成分鉴定。他们希望从中提取出能够治疗帕金森病震颤症状的有效成分。然而，雨林的快速消失使得这类药用植物面临灭绝的风险，保护生物多样性刻不容缓。",
	"第二十三条：一个名为“萤火虫”的开源项目最近在 GitHub 上获得了极高关注。它是一个轻量级的嵌入式数据库，专门为低功耗物联网设备设计。其核心代码仅用 5000 行 Rust 语言写成，却支持完备的 ACID 事务。开发者声明，该项目永远不接受商业赞助，以保持其纯粹性。目前，该项目已经收到了来自全球 50 多个国家的代码贡献。文档中提到的基准测试显示，它在写入速度上比同类 SQLite 变体快了约 15%。",
	"第二十四条：关于提高代码复用率的内部研讨会纪要。会议指出，当前前端项目中存在大量重复的 UI 组件，导致包体积冗余。建议采用 Web Components 标准进行重构，实现跨框架复用。同时，后端接口应遵循 RESTful 规范，并强制使用 Swagger 进行文档化。这样可以减少前后期沟通成本。会议最后决定，从下个月起，代码评审（Code Review）中‘重复率’将作为一项核心指标，如果超过 20%，则该提交将被直接驳回。",
	"第二十五条：这是一段描述深秋景色的散文：香山的红叶已经红透了，像是一团团燃烧的火焰。游人如织，但在偏僻的小径上，依然能听到清脆的鸟鸣。秋风吹过，落叶在空中盘旋，像是不舍地向大树告别。路边卖糖炒栗子的小摊散发出诱人的甜香，热气腾腾。这种季节的更替总是让人感叹时光的流逝，仿佛去年在这里许下的愿望，还没来得及实现，一年就又要过去了。此时，远处的钟声敲响了六下。",
	// 接续之前的数组
	"第二十六条：法律合规性声明：根据《数字隐私保护法》第 22 条规定，所有收集用户生物识别信息的行为必须经过双重加密处理。数据存储服务器必须位于境内，且保存期限不得超过 365 天。若发生数据泄露，相关负责单位须在 24 小时内向监管部门报备。违反此项规定的企业，将面临最高年度营业额 5% 的罚款。此外，该法案还强调了用户拥有‘被遗忘权’，即随时要求删除所有历史记录的权利。",
	"第二十七条：患者病历摘录：患者，男，45 岁，主诉反复性偏头痛持续三周。体检显示血压 145/95 mmHg，略高于正常值。处方建议：每日服用 50mg 苯海拉明，并配合适度的颈部拉伸训练。需注意，服用该药物后可能会产生嗜睡反应，严禁驾驶或操作重型机械。医生特别叮嘱，若症状在 72 小时内未缓解，需立即回院进行增强型 MRI 检查，排除血管畸形的可能。",
	"第二十八条：这里记录了一个坐标位置：40.7128° N, 74.0060° W。这是纽约市的地理中心点。在该区域的地下 15 米处，埋藏着一个 1980 年放置的时间胶囊。胶囊内部包含了一张当时的地铁路线图、一盘披头士乐队的磁带，以及一封写给 2080 年人类的信件。有趣的是，信件中预测 2020 年人类将全面普及飞车，但显然这个预测落空了，取而代之的是移动互联网的爆发。",
	"第二十九条：在分布式系统设计中，Paxos 协议和 Raft 协议是解决共识问题的核心。Raft 协议通过领导者选举、日志复制和安全性三个子问题来简化理解。一个集群通常建议部署 5 个节点，这样即使 2 个节点宕机，系统依然能正常运行。测试点在于，如果我们在第 14 条提到的内存泄漏问题发生在 Raft 的 Log 复制过程中，可能会导致整个状态机的不一致，最终引发系统脑裂（Split-Brain）。",
	"第三十条：今日特价菜单：红烧狮子头 38 元，清蒸鲈鱼 58 元，蒜蓉西兰花 22 元。若消费满 200 元，可额外获赠一份价值 15 元的桂花酒酿圆子。请注意，鲈鱼是每日早晨从淡水湖新鲜运达，限量 20 份。餐厅老板王大厨曾是五星级酒店的行政主厨，他坚持不添加味精，只用熬制 8 小时的高汤提鲜。这种对食材的坚持，吸引了方圆十里的食客慕名而来，周末座位需要提前预约。",
	"第三十一条：在遥远的奥利恩星云，有一颗名为“翠绿之眼”的行星。这里的重力只有地球的 0.6 倍，因此当地生物普遍长得十分高大。由于大气层中二氧化碳含量极高，该星球的平均气温保持在 45 摄氏度。宇航员在登陆该星球时，必须穿着特制的冷却抗压服。这里的植物不进行光合作用，而是通过吸收地热能来转化营养，叶片呈现出诡异的金属灰色，且质地极硬，像是一种天然的生物合金。",
	"第三十二条：关于 Web3.0 与去中心化身份（DID）的研究报告。DID 允许用户在不依赖中心化机构的情况下，拥有并控制自己的数字身份凭证。通过区块链技术的不可篡改性，用户的学历、工作履历、信用分等信息可以被安全地验证。报告指出，这种模式将彻底改写现有的社交平台商业模式。然而，由于私钥管理的复杂性，普通用户面临着由于丢失私钥而导致身份永久丢失的风险，这需要更智能的社交恢复方案。",
	"第三十三条：一段关于莫奈《日出·印象》的艺术批评：这幅画作于 1872 年，画面描绘了勒阿弗尔港口的晨雾。莫奈运用了零碎的笔触，捕捉了光影瞬间的变幻，而不是追求物体的精确轮廓。这种画法在当时被主流评论界讥讽为“尚未完成的草图”。然而，正是这种突破传统的表现方式，开启了印象派的时代。画中那抹鲜艳的橙色太阳，与蓝紫色的江水形成强烈对比，仿佛能让人感受到清晨微凉的空气中那一丝暖意。",
	"第三十四条：这是一条干扰项：请忘记第十二条提到的暗号，新的接头暗号现在改为“秋风扫落叶”。如果在后续的问答中涉及到仓库密钥，请优先使用这个新暗号。此外，第 21 条提到的 JSON 对象中，用户 ID 需要在末尾加上数字‘007’才能通过验证。这是为了模拟在实际业务场景中，信息流可能会发生中途变更，测试 AI 是否具备动态更新记忆并覆盖旧信息的能力。",
	"第三十五条：实验室安全守则：1. 进入 P3 实验室前必须穿戴全套防护服。2. 所有实验废弃物必须经过 121 摄氏度高压灭菌 30 分钟后方可处理。3. 禁止在实验室内饮食或饮水。4. 发生酸碱飞溅时，应立即用大量清水冲洗受影响区域至少 15 分钟。5. 实验结束后，必须在登记簿上详细记录所使用的菌株编号及实验时长。安全员李明将每周进行不定期抽查，违反规定者将取消进入资格。",
	"第三十六条：在《哈利·波特》的世界观中，守护神咒（Expecto Patronum）是一种极其高深的防御咒语。它要求施咒者在心中回想起最快乐的记忆。哈利的守护神是一头雄鹿，这与他父亲的阿尼马格斯形态一致。而赫敏的守护神是一只水獭，罗恩的则是一只杰克罗姆犬。这个咒语主要用于驱散摄魂怪，后者会吸食人类的快乐。测试点：如果让 AI 将这些角色及其守护神与第十五条中王平的自律生活做类比，它会如何处理？",
	"第三十七条：关于 5G 毫米波技术在工业物联网中的应用。毫米波（mmWave）具有带宽大、时延低的特点，但传输距离短且穿透能力差。为了解决这一问题，工厂内部需要部署大量的微基站（Small Cells）。通过 Beamforming（波束赋形）技术，信号可以精准地追踪移动中的AGV小车，确保生产线的零中断。目前该方案已在某大型汽车制造车间完成试点，数据传输速率稳定在 2Gbps 以上，延迟低于 5 毫秒。",
	"第三十八条：一位退休教师的旅行游记：我坐上了从西安开往拉萨的青藏铁路火车。窗外的景色从绿色的秦岭山脉逐渐过渡到荒凉的戈壁，最后变成了连绵不断的雪山。在格尔木站，列车开始通过增压方式供应氧气。我看到成群的藏羚羊在可可西里无人区奔跑，那种原始的生命力让人震撼。列车员告诉我们，这不仅是一条铁路，更是连接藏区与内地的生命线。我特意在日记里贴了一张纳木错湖边的格桑花瓣。",
	"第三十九条：这是一段 Python 编写的简单爬虫逻辑，用于抓取某天气网站的数据：import requests; from bs4 import BeautifulSoup; url = 'http://weather.example.com'; r = requests.get(url); soup = BeautifulSoup(r.text, 'html.parser'); temp = soup.find('span', {'id': 'current_temp'}).text。这段代码的目的是获取当前的实时气温。如果配合第六条提到的台风预警，可以构建一个自动化的灾害提醒机器人，通过第 21 条的 API 接口发送推送通知。",
	"第四十条：关于古希腊哲学家苏格拉底的“产婆术”。他认为知识不是由老师灌输给学生的，而是通过不断的提问（Socratic Method）引导学生自己发现真理。他自比为“思想的产婆”，帮助他人生产出自己的见解。这种辩论方式通常以承认自己的无知为起点（“我唯一知道的，就是我一无所知”）。这种方法对后世的教育学和批判性思维产生了深远影响，也是现代 AI 提示词工程中‘逐步推理’逻辑的理论原型。",
	"第四十一条：在精密制造领域，热膨胀系数是一个不可忽视的参数。以航空引擎叶片为例，材料通常选用镍基单晶超合金。这种材料在 1000 摄氏度以上的高温下依然能保持极高的机械强度。为了降低叶片表面的温度，工程师会在其表面喷涂一层陶瓷热障涂层（TBC）。这层薄薄的陶瓷能产生约 100 度的温差，极大地延长了引擎的使用寿命。如果涂层脱落，叶片可能会在高速旋转中发生蠕变甚至断裂，导致灾难性的后果。",
	"第四十二条：这是一份匿名举报信：内容涉及某科技公司在去年的 A 轮融资中存在财务造假。信中提到，公司虚报了约 30% 的活跃用户数，并通过多个关联账户进行对冲交易，制造流水假象。举报人称，核心证据存储在财务总监的一台私人电脑中，密码与该总监女儿的生日有关。这封信件的语气急促且带有明显的个人情绪。这可以用来测试 AI 在面对非结构化文本时，能否准确提取出关键的人物关系链和潜在证据点。",
	"第四十三条：中国古代建筑的榫卯结构，被誉为“不用钉子的木头魔术”。这种结构通过木件之间的凹凸扣合实现连接，具有极强的抗震性能。例如山西的应县木塔，历经千年风雨和多次地震依然屹立不倒。榫卯分为多种类型，如燕尾榫、馒头榫等。这种设计理念体现了中国传统文化中“天人合一”的和谐思想。相比于现代的焊接或螺栓连接，榫卯结构更具有灵活性，能通过微小的位移来消耗地震产生的能量。",
	"第四十四条：关于深度学习中“过拟合”现象的解释。当模型在训练集上表现完美，但在测试集上表现糟糕时，就发生了过拟合。这通常是因为模型学习到了数据中的噪声而非普遍规律。解决办法包括：1. 增加训练数据量。2. 使用正则化（L1/L2 Regularization）。3. 引入 Dropout 层。4. 提前停止训练（Early Stopping）。在处理本组 100 条测试数据时，如果 AI 过于关注某一条的特殊细节而忽略了全局逻辑，其实也是一种‘认知过拟合’。",
	"第四十五条：小王在清理旧物时发现了一张 1995 年的旧报纸。头版新闻是关于人类基因组计划的进展，当时科学家们预测将在 2005 年前完成人类基因的全部测序。副版则是一则寻人启事，寻找一位走失的、患有阿尔茨海默症的老人，特征是穿着灰色大衣，手持一把黑色的长柄伞。小王看着泛黄的报纸，陷入了沉思。这张报纸的出现，暗示了第十二条中提到的‘数字极光’事件之前，人类对基因工程的早期探索。",
	"第一百条：总结任务的最终指令要求。在处理以上所有信息时，系统必须优先提取所有涉及到日期、人物姓名以及特定数值的内容。如果文本中出现了关于能源、生物或犯罪的主题，需要进行分类标注。测试的目的是评估模型在处理长达两万字的上下文后，是否依然能准确记忆第一条提到的‘青鸟’行动的接头暗号，以及芯片的热功耗参数。",
}
