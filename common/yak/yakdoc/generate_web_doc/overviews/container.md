`container` 库提供常用数据结构容器，补充内置 `list`/`map` 之外的集合类型，用于需要去重、链表操作的算法场景。

典型使用场景：

- 集合：`container.NewSet`（线程安全集合）、`container.NewUnsafeSet`（非线程安全、更快）用于元素去重与集合运算。
- 链表：`container.NewLinkedList` 创建双向链表，适合频繁头尾插入/删除的场景。

与相邻库的关系：`container` 是纯数据结构工具，无副作用，可在任意脚本中替代手写去重/链表逻辑，常与 `x`（funk 工具）等数据处理库配合。
