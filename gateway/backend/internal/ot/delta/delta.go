package delta

type Kind string

const (
	KindRetain Kind = "retain"
	KindInsert Kind = "insert"
	KindDelete Kind = "delete"
)

type Op struct {
	Kind  Kind           `json:"kind"`            // "retain" / "insert" / "delete"
	Count int            `json:"count,omitempty"` // retain/delete 的长度
	Text  string         `json:"text,omitempty"`  // insert 的文本
	Attrs map[string]any `json:"attrs,omitempty"` // 样式属性（粗体/颜色等）
}

type Delta []Op

// "ops":[{"retain":5},{"insert":"Hello"}]
