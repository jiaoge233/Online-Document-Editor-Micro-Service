package collab

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"collabServer/backend/internal/ot/delta"
)

type WALEntry struct {
	Type         string      `json:"type"`
	OperationID  string      `json:"operationId"`
	DocID        string      `json:"docId"`
	AuthorID     uint64      `json:"authorId"`
	BaseRevision uint64      `json:"baseRevision"`
	ClientID     string      `json:"clientId"`
	ClientSeq    uint64      `json:"clientSeq"`
	Ops          delta.Delta `json:"ops"`
	ReceivedAt   time.Time   `json:"receivedAt"`
}

type WAL interface {
	Append(ctx context.Context, entry WALEntry) error
	Replay(ctx context.Context, handler func(context.Context, WALEntry) error) error
	Close() error
}

type FileWAL struct {
	mu   sync.Mutex
	path string
	file *os.File
}

func NewFileWAL(path string) (*FileWAL, error) {
	if path == "" {
		return nil, errors.New("wal path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	return &FileWAL{
		path: path,
		file: file,
	}, nil
}

func (w *FileWAL) Append(ctx context.Context, entry WALEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.file.Write(b); err != nil {
		return err
	}
	return w.file.Sync()
}

func (w *FileWAL) Replay(ctx context.Context, handler func(context.Context, WALEntry) error) error {
	f, err := os.Open(w.path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	lineNo := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		lineNo++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry WALEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return fmt.Errorf("decode wal line %d: %w", lineNo, err)
		}
		if err := handler(ctx, entry); err != nil {
			return fmt.Errorf("replay wal line %d: %w", lineNo, err)
		}
	}

	return scanner.Err()
}

func (w *FileWAL) Close() error {
	if w == nil || w.file == nil {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
