package collab

import (
	"testing"

	"gateway/backend/internal/ot/delta"
)

func TestPieceTable_BasicString(t *testing.T) {
	pt := NewPieceTable("Hello world")
	if got := pt.String(); got != "Hello world" {
		t.Fatalf("String() = %q, want %q", got, "Hello world")
	}
	if gotLen := pt.Len(); gotLen != len([]rune("Hello world")) {
		t.Fatalf("Len() = %d, want %d", gotLen, len([]rune("Hello world")))
	}
}

func TestPieceTable_InsertMiddle(t *testing.T) {
	pt := NewPieceTable("Hello world")

	d := delta.Delta{
		{Kind: delta.KindRetain, Count: 5},               // 跳过 "Hello"
		{Kind: delta.KindInsert, Text: " collaborative"}, // 在 pos=5 插入
	}

	if err := pt.Apply(d); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	want := "Hello collaborative world"
	if got := pt.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestPieceTable_DeleteMiddle(t *testing.T) {
	pt := NewPieceTable("Hello collaborative world")

	// "Hello collaborative world"
	//  01234 5            18 ...
	//  保留 "Hello"，然后删 " collaborative"
	d := delta.Delta{
		{Kind: delta.KindRetain, Count: 5},  // "Hello"
		{Kind: delta.KindDelete, Count: 14}, // " collaborative" 长度
	}

	if err := pt.Apply(d); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	want := "Hello world"
	if got := pt.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
