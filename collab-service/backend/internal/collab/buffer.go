package collab

import (
	"collabServer/backend/internal/ot/delta"
)

// 抽象文档内容缓冲区接口
type Buffer interface {
	Len() int
	Apply(d delta.Delta) error
	String() string
}

/*
结构示例

初始文档内容 `"Hello world"`：

- original buffer 内容：`"Hello world"`
- add buffer 为空 (`""`)
- piece 表：


[ (orig, offset=0, length=11) ]  // 整个文档


在位置 5 插入 `" collaborative"`：
- 在 **add buffer** 末尾追加 `" collaborative"`：
  - add buffer = `" collaborative"`
- piece 表从一条拆成三条：


[
  (orig, offset=0, length=5),       // "Hello"
  (add,  offset=0, length=13),      // " collaborative"
  (orig, offset=5, length=6),       // " world"
]
*/
