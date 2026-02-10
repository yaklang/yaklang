# SQLite FTS5（bizhelper）

本目录提供一套基于 SQLite FTS5 的封装，用于给“现有表”配套增加 **FTS5 虚拟索引表**，并提供：

- 自动建表
- 数据迁移/重建
- 触发器同步（insert/update/delete）
- `MATCH` 搜索 + `bm25()` 排序（返回原表结构体）
- 索引清理（删除触发器与 FTS 表）
- Yield（分页流式返回）

> 注意：这里**不会**帮你“开启 FTS5”，默认你的 SQLite 构建已内置/启用 FTS5。

## 目标能力

- 新建 FTS5 虚拟表
- 从原表迁移/重建数据
- 绑定触发器（insert/update/delete 同步）
- BM25 + MATCH 搜索（返回值为原表结构体）
- 清理索引（drop triggers + drop fts table）
- Yield（按页拉取并流式输出）

实现文件：`common/utils/bizhelper/sqlite_fts5.go`

## 配置：`SQLiteFTS5Config`

最常用字段：

- `BaseModel any`：可选；用于从 gorm scope 推导 `BaseTable`（推荐）
- `BaseTable string`：原表表名（与 `BaseModel` 二选一即可）
- `FTSTable string`：FTS5 虚拟表表名（例如：`your_table_fts`）
- `RowIDColumn string`：原表主键列，映射到 FTS 的 `rowid`（默认 `id`）
- `Columns []string`：需要做全文索引的列名列表（例如：`[]string{"title","body"}`）
- `Tokenize string`：FTS5 tokenize（默认 `unicode61`）

content 模式（可选）：

- **默认行为（重要）**：当 `ContentTable == ""` 时，内部会创建 **contentless FTS**（`content=''`），避免存储重复内容，并提升不同 SQLite/FTS5 构建下的兼容性。
- 外部 content（external content）：当 `ContentTable != ""` 时，创建 `content='...'` 的外部内容索引表。

- `ContentTable string`：开启 external content 模式（FTS5 `content='...'`）
- `ContentRowID string`：external content 的 `content_rowid`（默认等于 `RowIDColumn`）

## API

### 1) 新建虚拟表

```go
err := bizhelper.SQLiteFTS5CreateVirtualTable(db, cfg)
```

会执行类似：

- `CREATE VIRTUAL TABLE IF NOT EXISTS <FTSTable> USING fts5(...)`

### 2) 迁移/重建数据

```go
err := bizhelper.SQLiteFTS5MigrateData(db, cfg)
```

- external content：使用 `INSERT INTO fts(fts) VALUES('rebuild')` 重建
- 非 external content（contentless）：先尝试 `INSERT INTO fts(fts) VALUES('delete-all')`，如果 SQLite/FTS5 构建不支持则回退到 `DELETE FROM <FTSTable>`，再批量 `INSERT ... SELECT` 导入数据

### 3) 绑定触发器（同步）

```go
err := bizhelper.SQLiteFTS5BindTriggers(db, cfg)
```

为原表创建/重建 3 个 trigger（名字带前缀 `<BaseTable>_<FTSTable>_fts5_*`）：

- `AFTER INSERT`：向 FTS 插入一条索引行
- `AFTER UPDATE`：先向 FTS 写入 `delete`（含 old 值），再插入 new 值
- `AFTER DELETE`：向 FTS 写入 `delete`（含 old 值）

### 4) 一键初始化（推荐）

```go
err := bizhelper.SQLiteFTS5Setup(db, cfg)
```

内部事务顺序：

1. `SQLiteFTS5CreateVirtualTable`
2. `SQLiteFTS5MigrateData`
3. `SQLiteFTS5BindTriggers`

### 5) BM25 + MATCH 搜索（返回原表结构体）

```go
rows, err := bizhelper.SQLiteFTS5BM25Match[YourModel](db, cfg, []string{"query"}, 20, 0)
```

- 使用 `JOIN <FTSTable> ON <FTSTable>.rowid = <BaseTable>.<RowIDColumn>`
- 使用 `WHERE <FTSTable> MATCH ?`
- 使用 `ORDER BY bm25(<FTSTable>)`
- 返回 `[]YourModel`（即原表结构体切片）
- **多关键词默认 OR**：传 `matches []string`，内部用 `OR` 连接（例如：`[]string{"yaklang","fts5"}` → `(yaklang) OR (fts5)`）。如需 AND/短语/字段查询，请把完整 FTS5 表达式作为**单个** query string 传入（例如：`[]string{` + "`yaklang AND fts5`" + `}`、`[]string{` + "`\"hello world\"`" + `}`、`[]string{` + "`title:yaklang`" + `}`）。

也可使用扫描到任意容器（例如 `[]struct`）的版本：

```go
var out []YourModel
err := bizhelper.SQLiteFTS5BM25MatchInto(db, cfg, []string{"query"}, &out, 20, 0)
```

#### 保留原表过滤（重要）

`SQLiteFTS5BM25Match/Into` 会保留调用方在 `db` 上事先链的过滤条件（`Where/Joins/...`），也就是说你可以先对原表做过滤，再叠加 FTS 的 `MATCH + bm25`：

```go
filtered := db.Model(&schema.Doc{}).Where("collection_uuid = ?", uuid)
rows, err := bizhelper.SQLiteFTS5BM25Match[schema.Doc](filtered, cfg, []string{"yaklang"}, 50, 0)
```

> 注意：当你自己写 `Where("content LIKE ?")` 之类条件时，如果 join 后出现字段歧义，请显式写表前缀或用别名（例如：`rag_vector_document_v1.content`）。

### 6) Yield（分页流式返回）

```go
ch := bizhelper.SQLiteFTS5BM25MatchYield[schema.Doc](
  ctx,
  db.Model(&schema.Doc{}).Where("collection_uuid = ?", uuid),
  cfg,
  []string{"yaklang"},
  bizhelper.WithYieldModel_PageSize(100),
  bizhelper.WithYieldModel_Limit(1000),
)
for doc := range ch {
  _ = doc
}
```

### 7) 清理索引（drop triggers + drop fts table）

当原表被 drop 后，FTS 虚拟表可能仍然存在（例如你是手动 drop 某张表，或者在某些迁移路径下留下了“孤儿 FTS 表”）。这时可以调用清理：

```go
_ = bizhelper.SQLiteFTS5Drop(db, cfg)
```

`SQLiteFTS5Drop` 是幂等的：

- triggers 已不存在也不会报错（`DROP TRIGGER IF EXISTS ...`）
- FTS 表已不存在也不会报错（`DROP TABLE IF EXISTS ...`）

## 最小示例

```go
cfg := &bizhelper.SQLiteFTS5Config{
  BaseModel: &schema.Doc{},       // 推荐：自动推导 BaseTable
  FTSTable:  "docs_fts",
  Columns:   []string{"title", "body"},
  // RowIDColumn: "id",            // 默认 id
  // Tokenize: "unicode61",        // 默认 unicode61
}

if err := bizhelper.SQLiteFTS5Setup(db, cfg); err != nil {
  return err
}

docs, err := bizhelper.SQLiteFTS5BM25Match[schema.Doc](db, cfg, []string{`yaklang`}, 50, 0)
```

## 单例默认配置（推荐用法）

你可以为某张表维护一个默认 `*SQLiteFTS5Config` 单例，然后在各处复用：

```go
var DefaultDocFTS5 = &bizhelper.SQLiteFTS5Config{
  BaseModel: &schema.Doc{},
  FTSTable:  "docs_fts",
  Columns:   []string{"title", "body"},
}

_ = bizhelper.SQLiteFTS5Setup(db, DefaultDocFTS5)
docs, _ := bizhelper.SQLiteFTS5BM25Match[schema.Doc](db, DefaultDocFTS5, []string{`yaklang`}, 50, 0)
```

如果某次调用需要临时覆盖（不修改默认配置），可以额外传入 options（可选）：

```go
docs, _ := bizhelper.SQLiteFTS5BM25Match[schema.Doc](
  db,
  DefaultDocFTS5,
  []string{`yaklang`},
  50,
  0,
  bizhelper.WithSQLiteFTS5FTSTable("docs_fts_shadow"),
)
```

## 注意事项 / 约束

- `RowIDColumn` 必须是稳定的整数主键（常见为 `id INTEGER PRIMARY KEY`），用于 `base.id = fts.rowid` 的 join。
- `MATCH` 的语法是 FTS 的查询语法（支持 `AND/OR/NEAR`、短语等）。如果你接收用户输入，建议在上层做必要的过滤/转义策略。
- `Tokenize` 的具体行为取决于 SQLite/FTS5 的构建与 tokenizer（例如 `trigram`）。当使用 `trigram` 时，`MATCH` 查询的表达式需要能产生有效 token（例如 `北京大*`，而 `北京*` 可能无法命中）。
- 触发器会带来写入开销；如果你有批量导入场景，可以：
  1) 先建表（不绑 trigger）→ 2) 导入数据 → 3) `SQLiteFTS5MigrateData` → 4) 再 `SQLiteFTS5BindTriggers`。
