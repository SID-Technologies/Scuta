package output

import (
	"bytes"
	"io"
	"testing"
)

func TestProgressReader(t *testing.T) {
	data := []byte("hello world, this is test data for progress reader")
	reader := bytes.NewReader(data)

	pr := NewProgressReader(reader, int64(len(data)))

	// Read all bytes
	result, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(result, data) {
		t.Errorf("read data mismatch: got %q, want %q", result, data)
	}

	if pr.BytesRead() != int64(len(data)) {
		t.Errorf("BytesRead() = %d, want %d", pr.BytesRead(), len(data))
	}
}

func TestProgressReaderPartialReads(t *testing.T) {
	data := []byte("abcdefghij") // 10 bytes
	reader := bytes.NewReader(data)

	pr := NewProgressReader(reader, int64(len(data)))

	// Read in small chunks
	buf := make([]byte, 3)
	totalRead := 0

	for {
		n, err := pr.Read(buf)
		totalRead += n
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}

	if int64(totalRead) != pr.BytesRead() {
		t.Errorf("totalRead=%d != BytesRead()=%d", totalRead, pr.BytesRead())
	}

	if pr.BytesRead() != int64(len(data)) {
		t.Errorf("BytesRead() = %d, want %d", pr.BytesRead(), len(data))
	}
}

func TestProgressReaderEmptyInput(t *testing.T) {
	reader := bytes.NewReader(nil)

	pr := NewProgressReader(reader, 0)

	result, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d bytes", len(result))
	}

	if pr.BytesRead() != 0 {
		t.Errorf("BytesRead() = %d, want 0", pr.BytesRead())
	}
}
