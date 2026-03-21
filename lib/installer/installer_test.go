package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
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

func TestBinaryName(t *testing.T) {
	// binaryName should return the tool name as-is on non-Windows,
	// and with .exe on Windows. We test the function directly.
	name := binaryName("pilum")
	if runtime.GOOS == "windows" {
		if name != "pilum.exe" {
			t.Errorf("expected pilum.exe on Windows, got %q", name)
		}
	} else {
		if name != "pilum" {
			t.Errorf("expected pilum on non-Windows, got %q", name)
		}
	}

	// Should not double-add .exe
	if runtime.GOOS == "windows" {
		name = binaryName("pilum.exe")
		if name != "pilum.exe" {
			t.Errorf("expected pilum.exe (no double suffix), got %q", name)
		}
	}
}

func TestFindBinaryWithExtension(t *testing.T) {
	// Test that findBinary can find a file with an extension via nameWithoutExt matching.
	// This works on any OS (the .exe check is the only Windows-specific part).
	tmpDir := t.TempDir()

	binaryPath := filepath.Join(tmpDir, "mytool.exe")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// findBinary strips extensions and matches against the tool name
	found, err := findBinary(tmpDir, "mytool")
	if err != nil {
		t.Fatalf("findBinary failed: %v", err)
	}

	if found != binaryPath {
		t.Errorf("expected %q, got %q", binaryPath, found)
	}
}

func TestFindBinaryPrefixFallback(t *testing.T) {
	// Test that findBinary falls back to prefix matching (e.g., "pilum_v1.0.0_darwin_arm64")
	tmpDir := t.TempDir()

	binaryPath := filepath.Join(tmpDir, "mytool_v1.0.0_darwin_arm64")
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

func TestFindBinaryExactOverPrefix(t *testing.T) {
	// Exact match should be preferred over prefix match
	tmpDir := t.TempDir()

	exactPath := filepath.Join(tmpDir, "mytool")
	if err := os.WriteFile(exactPath, []byte("exact"), 0o755); err != nil {
		t.Fatal(err)
	}
	prefixPath := filepath.Join(tmpDir, "mytool_v1.0.0_darwin_arm64")
	if err := os.WriteFile(prefixPath, []byte("prefix"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := findBinary(tmpDir, "mytool")
	if err != nil {
		t.Fatalf("findBinary failed: %v", err)
	}

	if found != exactPath {
		t.Errorf("expected exact match %q, got %q", exactPath, found)
	}
}

func TestSetSignatureVerification(t *testing.T) {
	inst := New(nil, t.TempDir())

	// Default: no signature verification
	if inst.requireSignature {
		t.Error("expected requireSignature=false by default")
	}
	if inst.signaturePubKey != nil {
		t.Error("expected nil signaturePubKey by default")
	}

	// Set signature verification
	pubKey := []byte("-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----")
	inst.SetSignatureVerification(true, pubKey)

	if !inst.requireSignature {
		t.Error("expected requireSignature=true after SetSignatureVerification")
	}
	if string(inst.signaturePubKey) != string(pubKey) {
		t.Errorf("expected pubKey to be set, got %q", inst.signaturePubKey)
	}
}

func TestSetSignatureVerificationOnNewWithBinDir(t *testing.T) {
	inst := NewWithBinDir(nil, t.TempDir(), "/usr/local/bin")

	// Should start without signature verification
	if inst.requireSignature {
		t.Error("expected requireSignature=false by default for NewWithBinDir")
	}

	inst.SetSignatureVerification(true, []byte("key"))

	if !inst.requireSignature {
		t.Error("expected requireSignature=true after set")
	}
}

func TestNewSetsDefaultBinDir(t *testing.T) {
	tmpDir := t.TempDir()
	inst := New(nil, tmpDir)

	expected := filepath.Join(tmpDir, "bin")
	if inst.binDir != expected {
		t.Errorf("expected binDir %q, got %q", expected, inst.binDir)
	}
}

func TestNewWithBinDirSetsCustomBinDir(t *testing.T) {
	inst := NewWithBinDir(nil, t.TempDir(), "/custom/bin")
	if inst.binDir != "/custom/bin" {
		t.Errorf("expected binDir '/custom/bin', got %q", inst.binDir)
	}
}

func TestParseVersionFromFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		expected string
	}{
		{"standard goreleaser", "pilum_1.2.3_darwin_arm64.tar.gz", "1.2.3"},
		{"with v prefix", "pilum_v2.0.1_linux_amd64.tar.gz", "2.0.1"},
		{"dash separator", "tool-1.0.0-linux-amd64.tar.gz", "1.0.0"},
		{"zip extension", "tool_3.2.1_windows_amd64.zip", "3.2.1"},
		{"no version", "tool_darwin_arm64.tar.gz", "local"},
		{"tgz extension", "pilum_1.0.0_linux_amd64.tgz", "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersionFromFilename(tt.filename)
			if got != tt.expected {
				t.Errorf("parseVersionFromFilename(%q) = %q, want %q", tt.filename, got, tt.expected)
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
