package minirehs

// compositeDB 是"主后端 + 可选 stdlib 兜底子集"的组合容器, 实现 compiledDB.
// 当前纵切中兜底子集为空 (regexp2-only pattern 已由引擎内 verifier 直接承载),
// 但保留该结构以便未来把少数特殊 pattern 拆到独立子引擎且对调用方透明.
//
// 去重策略: 主后端与兜底集合通常按 pattern 划分, 不重叠; 为稳健起见对 (id,from,to)
// 三元组去重, 避免重复上报.
//
// 关键词: CompositeDatabase, 主后端, 兜底子集, 去重
type compositeDB struct {
	primary  compiledDB
	fallback compiledDB // 可为 nil
}

func newCompositeDB(primary compiledDB, fallback compiledDB) *compositeDB {
	return &compositeDB{primary: primary, fallback: fallback}
}

func (c *compositeDB) numAlwaysOn() int {
	n := c.primary.numAlwaysOn()
	if c.fallback != nil {
		n += c.fallback.numAlwaysOn()
	}
	return n
}

func (c *compositeDB) close() error {
	if c.fallback != nil {
		_ = c.fallback.close()
	}
	return c.primary.close()
}

func (c *compositeDB) scan(data []byte, sc *scratch, handler MatchHandler) (bool, error) {
	if c.fallback == nil {
		// 无兜底子集时不需要去重, 直接透传主后端 (热路径零额外开销).
		return c.primary.scan(data, sc, handler)
	}

	seen := make(map[matchKey]struct{})
	wrap := func(m Match) bool {
		k := matchKey{id: m.ID, from: m.From, to: m.To}
		if _, dup := seen[k]; dup {
			return true
		}
		seen[k] = struct{}{}
		return handler(m)
	}

	stopped, err := c.primary.scan(data, sc, wrap)
	if err != nil || stopped {
		return stopped, err
	}
	return c.fallback.scan(data, sc, wrap)
}

type matchKey struct {
	id   PatternID
	from int
	to   int
}
