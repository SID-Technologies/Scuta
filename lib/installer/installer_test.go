package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarGz(t *testing.T) {
	// Create a temp tar.gz with a fake binary
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	// Create the archive
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Add a file
	content := []byte("#!/bin/sh\necho hello")
	hdr := &tar.Header{
		Name: "mytool",
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()
	f.Close()

	// Extract
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := extractTarGz(archivePath, extractDir); err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}

	// Verify file exists
	extractedPath := filepath.Join(extractDir, "mytool")
	info, err := os.Stat(extractedPath)
	if err != nil {
		t.Fatalf("extracted file not found: %v", err)
	}

	if info.Size() != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), info.Size())
	}
}

func TestExtractTarGzNested(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Add a directory
	dirHdr := &tar.Header{
		Name:     "subdir/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	}
	if err := tw.WriteHeader(dirHdr); err != nil {
		t.Fatal(err)
	}

	// Add a file inside the directory
	content := []byte("binary content")
	hdr := &tar.Header{
		Name: "subdir/mytool",
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()
	f.Close()

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := extractTarGz(archivePath, extractDir); err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}

	extractedPath := filepath.Join(extractDir, "subdir", "mytool")
	if _, err := os.Stat(extractedPath); err != nil {
		t.Fatalf("nested extracted file not found: %v", err)
	}
}

func TestExtractTarGzPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "evil.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	content := []byte("malicious")
	hdr := &tar.Header{
		Name: "../../escape",
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()
	f.Close()

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err = extractTarGz(archivePath, extractDir)
	if err == nil {
		t.Error("expected error for path traversal in tar, got nil")
	}
}

func TestExtractTarGzSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "symlink.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     "evil-link",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}

	tw.Close()
	gw.Close()
	f.Close()

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err = extractTarGz(archivePath, extractDir)
	if err == nil {
		t.Error("expected error for symlink in tar, got nil")
	}
}

func TestExtractZipPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "evil.zip")

	// Create a zip with a path traversal entry
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)

	// The zip.Writer normalizes paths, so we use a header with the raw name
	header := &zip.FileHeader{
		Name:   "../../escape",
		Method: zip.Store,
	}
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("malicious")); err != nil {
		t.Fatal(err)
	}

	zw.Close()
	f.Close()

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	err = extractZip(archivePath, extractDir)
	if err == nil {
		t.Error("expected error for path traversal in zip, got nil")
	}
}

func TestFindBinary(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake binary at root level
	binaryPath := filepath.Join(tmpDir, "mytool")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := findBinary(tmpDir, "mytool")
	if err != nil {
		t.Fatalf("findBinary failed: %v", err)
	}

	if found != binaryPath {
		t.Errorf("expected %q, got %q", binaryPath, found)
	}
}

func TestFindBinaryNested(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a nested binary
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	binaryPath := filepath.Join(subDir, "mytool")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := findBinary(tmpDir, "mytool")
	if err != nil {
		t.Fatalf("findBinary failed: %v", err)
	}

	if found != binaryPath {
		t.Errorf("expected %q, got %q", binaryPath, found)
	}
}

func TestFindBinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := findBinary(tmpDir, "nonexistent")
	if err == nil {
		t.Error("expected error for missing binary, got nil")
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "source")
	dst := filepath.Join(tmpDir, "dest")

	content := []byte("hello world")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(content) {
		t.Errorf("expected %q, got %q", content, got)
	}
}

func TestIsSafePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		safe bool
	}{
		{"normal file", "mytool", true},
		{"nested file", "subdir/mytool", true},
		{"traversal", "../../escape", false},
		{"deep traversal", "foo/../../..", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafePath("/tmp/extract", tt.path); got != tt.safe {
				t.Errorf("isSafePath(%q) = %v, want %v", tt.path, got, tt.safe)
			}
		})
	}
}

func TestValidateToolName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "pilum", false},
		{"valid with dash", "my-tool", false},
		{"valid with underscore", "my_tool", false},
		{"traversal", "../escape", true},
		{"path separator", "foo/bar", true},
		{"dot", ".", true},
		{"dotdot", "..", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateToolName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateToolName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
