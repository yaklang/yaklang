`git` 库提供 Git 仓库操作能力，覆盖克隆、拉取、检出、分支/提交遍历、Blame 以及把任意提交/时间范围"快照"成可遍历文件系统的能力，常用于代码审计、供应链分析与 .git 泄露利用。

典型使用场景：

- 仓库操作：`git.Clone` / `git.Pull` / `git.Fetch` / `git.Checkout`，配合 `git.auth` / `git.withPrivateKey` / `git.depth` / `git.branch` 等选项。
- 历史分析：`git.IterateCommit` 遍历提交，`git.Blame` / `git.BlameCommit` 行级追溯，`git.Branch` / `git.HeadHash` / `git.ParentHash` / `git.RevParse` 查询引用。
- 快照为文件系统：`git.FileSystemFromCommit` / `git.FileSystemFromCommitRange` / `git.FileSystemFromDate` / `git.FileSystemCurrentWeek` 等把某次提交/时间窗的代码变成可遍历 FS。
- 安全利用：`git.GitHack` 从泄露的 .git 目录还原源码。

与相邻库的关系：`git` 产出的文件系统常交给 `ssa`/`syntaxflow`（代码分析）、`diff`（版本比对）、`filesys`（遍历）做后续审计。
