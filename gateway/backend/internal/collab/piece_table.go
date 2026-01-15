package collab

import "gateway/backend/internal/ot/delta"

type bufferKind int

const (
	//iota：在 const (...) 里从 0 开始自动递增，这里就是：bufOriginal = 0, bufAdd = 1
	bufOriginal bufferKind = iota
	bufAdd
)

type piece struct {
	buf    bufferKind
	offset int
	length int
}

type PieceTable struct {
	original []rune
	add      []rune
	pieces   []piece
}

func NewPieceTable(initial string) *PieceTable {
	r := []rune(initial)
	return &PieceTable{
		original: r,
		pieces:   []piece{{buf: bufOriginal, offset: 0, length: len(r)}},
	}
}

func (pt *PieceTable) Len() int {
	n := 0
	for _, p := range pt.pieces {
		n += p.length
	}
	return n
}

func (pt *PieceTable) String() string {
	var res string
	for _, p := range pt.pieces {
		switch p.buf {
		case bufOriginal:
			res += string(pt.original[p.offset : p.offset+p.length])
		case bufAdd:
			res += string(pt.add[p.offset : p.offset+p.length])
		}
	}
	return res
}

func (pt *PieceTable) Apply(d delta.Delta) error {
	pos := 0
	//retain: 沿 piece 列表向前走，对应“移动 pos”；
	//insert: 在当前 pos 调用 insert 流程；
	//delete: 在当前 pos 调用 delete 流程（通过调整/合并 piece）。
	for _, op := range d {
		switch op.Kind {
		case delta.KindRetain:
			pos += op.Count

		case delta.KindInsert:
			d_rune := []rune(op.Text)
			start := len(pt.add)
			pt.add = append(pt.add, d_rune...)
			length := len(d_rune)

			idx, offset := pt.locate(pos)
			new_piece := piece{buf: bufAdd, offset: start, length: length}

			if idx < len(pt.pieces) {
				cur := pt.pieces[idx]
				left_piece := piece{buf: cur.buf, offset: pt.pieces[idx].offset, length: offset}
				right_piece := piece{buf: cur.buf, offset: pt.pieces[idx].offset + offset, length: pt.pieces[idx].length - offset}

				newPieces := make([]piece, 0, len(pt.pieces)+1)

				if left_piece.length > 0 {
					newPieces = append(newPieces, left_piece)
				}
				newPieces = append(newPieces, new_piece)
				if right_piece.length > 0 {
					newPieces = append(newPieces, right_piece)
				}

				newPieces = append(newPieces, pt.pieces[idx+1:]...)
				pt.pieces = newPieces
			} else {
				pt.pieces = append(pt.pieces, new_piece)
			}

			pos += length

		case delta.KindDelete:
			remain := op.Count
			idx, offset := pt.locate(pos)

			for remain > 0 && idx < len(pt.pieces) {
				cur := &pt.pieces[idx]
				// 这个 piece 里还剩多少可删
				can := cur.length - offset
				if can <= 0 {
					idx++
					offset = 0
					continue
				}

				// 本轮实际要删多少
				take := remain
				if take > can {
					take = can
				}

				// 整个 piece 都删掉
				if offset == 0 && take == cur.length {
					pt.pieces = append(pt.pieces[:idx], pt.pieces[idx+1:]...)
					// idx 不动（现在这个位置是删完后的下一个 piece）
					offset = 0
				} else {
					// 只删中间一段：从 offset 开始删 take 个
					// 拆成 左 / 右 两段
					leftLen := offset
					rightLen := cur.length - offset - take

					// 构造一个临时切片 newPieces，把当前 cur 替换掉
					newPieces := make([]piece, 0, len(pt.pieces)+1)
					newPieces = append(newPieces, pt.pieces[:idx]...)
					if leftLen > 0 {
						newPieces = append(newPieces, piece{
							buf:    cur.buf,
							offset: cur.offset,
							length: leftLen,
						})
					}
					if rightLen > 0 {
						newPieces = append(newPieces, piece{
							buf:    cur.buf,
							offset: cur.offset + offset + take,
							length: rightLen,
						})
					}
					newPieces = append(newPieces, pt.pieces[idx+1:]...)
					pt.pieces = newPieces
				}

				remain -= take
			}
		}
	}
	return nil
}

// 根据逻辑位置 pos，找到对应的 piece 下标 idx 和在该 piece 内的偏移 offset
func (pt *PieceTable) locate(pos int) (idx int, offset int) {
	cur := 0
	for i, p := range pt.pieces {
		if pos < cur+p.length {
			return i, pos - cur
		}
		cur += p.length
	}
	return len(pt.pieces), 0
}
