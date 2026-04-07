package collab

import (
	"context"
	"path/filepath"
	"testing"

	"collabServer/backend/internal/ot/delta"
)

func TestFileWALReplay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	walPath := filepath.Join(t.TempDir(), "collab.wal")

	wal, err := NewFileWAL(walPath)
	if err != nil {
		t.Fatalf("NewFileWAL() error = %v", err)
	}
	defer wal.Close()

	want := []WALEntry{
		{
			Type:         "op_submit",
			OperationID:  "o-1",
			DocID:        "doc-1",
			AuthorID:     1001,
			BaseRevision: 0,
			ClientID:     "c-1",
			ClientSeq:    1,
			Ops: delta.Delta{
				{Kind: delta.KindInsert, Text: "Hello"},
			},
		},
		{
			Type:         "op_submit",
			OperationID:  "o-2",
			DocID:        "doc-1",
			AuthorID:     1001,
			BaseRevision: 1,
			ClientID:     "c-1",
			ClientSeq:    2,
			Ops: delta.Delta{
				{Kind: delta.KindRetain, Count: 5},
				{Kind: delta.KindInsert, Text: " world"},
			},
		},
	}

	for _, entry := range want {
		if err := wal.Append(ctx, entry); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	var got []WALEntry
	if err := wal.Replay(ctx, func(_ context.Context, entry WALEntry) error {
		got = append(got, entry)
		return nil
	}); err != nil {
		t.Fatalf("Replay() error = %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("Replay() got %d entries, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i].OperationID != want[i].OperationID {
			t.Fatalf("entry[%d].OperationID = %q, want %q", i, got[i].OperationID, want[i].OperationID)
		}
		if got[i].DocID != want[i].DocID {
			t.Fatalf("entry[%d].DocID = %q, want %q", i, got[i].DocID, want[i].DocID)
		}
		if got[i].BaseRevision != want[i].BaseRevision {
			t.Fatalf("entry[%d].BaseRevision = %d, want %d", i, got[i].BaseRevision, want[i].BaseRevision)
		}
		if len(got[i].Ops) != len(want[i].Ops) {
			t.Fatalf("entry[%d].Ops len = %d, want %d", i, len(got[i].Ops), len(want[i].Ops))
		}
	}
}

func TestRecoverFromWALRestoresDocumentContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	walPath := filepath.Join(t.TempDir(), "collab.wal")

	wal, err := NewFileWAL(walPath)
	if err != nil {
		t.Fatalf("NewFileWAL() error = %v", err)
	}

	svc := NewInMemoryService(nil, nil, nil, "", nil, nil, 0, wal)

	if _, err := svc.Submit(ctx, "doc-1", 1001, 0, "c-1", 1, delta.Delta{
		{Kind: delta.KindInsert, Text: "Hello"},
	}); err != nil {
		t.Fatalf("Submit(insert Hello) error = %v", err)
	}

	if _, err := svc.Submit(ctx, "doc-1", 1001, 1, "c-1", 2, delta.Delta{
		{Kind: delta.KindRetain, Count: 5},
		{Kind: delta.KindInsert, Text: " world"},
	}); err != nil {
		t.Fatalf("Submit(insert world) error = %v", err)
	}

	if err := wal.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	replayWAL, err := NewFileWAL(walPath)
	if err != nil {
		t.Fatalf("NewFileWAL(replay) error = %v", err)
	}
	defer replayWAL.Close()

	recovered := NewInMemoryService(nil, nil, nil, "", nil, nil, 0, replayWAL)
	if err := recovered.RecoverFromWAL(ctx); err != nil {
		t.Fatalf("RecoverFromWAL() error = %v", err)
	}

	content, revision, err := recovered.LoadDocumentContent(ctx, "doc-1")
	if err != nil {
		t.Fatalf("LoadDocumentContent() error = %v", err)
	}
	if content != "Hello world" {
		t.Fatalf("LoadDocumentContent() content = %q, want %q", content, "Hello world")
	}
	if revision != 2 {
		t.Fatalf("LoadDocumentContent() revision = %d, want %d", revision, 2)
	}
}
