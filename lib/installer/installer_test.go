package installer

import (
	"archive/tar"
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
