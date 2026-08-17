package managers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sid-technologies/scuta/lib/audit"
)

func fakeDpkg(statusContent string, verifyOut string, hasDpkg bool) *Dpkg {
	return &Dpkg{
		stat: func(p string) (os.FileInfo, error) {
			if p == dpkgStatusPath && statusContent != "" {
				return fakeInfoFile{}, nil
			}
			return nil, errors.New("not found")
		},
		readFile: func(p string) ([]byte, error) {
			if p == dpkgStatusPath && statusContent != "" {
				return []byte(statusContent), nil
			}
			return nil, errors.New("not found")
		},
		lookPath: func(string) (string, error) {
			if hasDpkg {
				return "/usr/bin/dpkg", nil
			}
			return "", errors.New("not found")
		},
		run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte(verifyOut), nil
		},
	}
}

type fakeInfoFile struct{ os.FileInfo }

const dpkgStatus = "Package: bash\nStatus: install ok installed\n\nPackage: coreutils\nStatus: install ok installed\n"

func TestDpkgDetect(t *testing.T) {
	if !fakeDpkg(dpkgStatus, "", true).Detect() {
		t.Fatal("status file present must detect")
	}
	if fakeDpkg("", "", true).Detect() {
		t.Fatal("no status file must not detect")
	}
}

func TestDpkgAuditCleanSystem(t *testing.T) {
	rep := fakeDpkg(dpkgStatus, "", true).Audit(context.Background())

	if len(rep.Findings) != 1 || rep.Findings[0].Code != audit.CodeInventorySummarized {
		t.Fatalf("findings = %+v", rep.Findings)
	}
	if !strings.Contains(rep.Findings[0].Message, "2 package(s)") {
		t.Fatalf("message = %q", rep.Findings[0].Message)
	}
	if len(rep.Packages) != 0 {
		t.Fatal("dpkg must not list individual packages")
	}
}

func TestDpkgAuditDriftAndConffiles(t *testing.T) {
	verify := "??5??????   /usr/bin/tool\n??5?????? c /etc/tool.conf\n"
	rep := fakeDpkg(dpkgStatus, verify, true).Audit(context.Background())

	var drift, conf *audit.Finding
	for i := range rep.Findings {
		switch rep.Findings[i].Code {
		case audit.CodeBinaryDrift:
			drift = &rep.Findings[i]
		case audit.CodeConfigDrift:
			conf = &rep.Findings[i]
		}
	}

	if drift == nil || drift.Severity != audit.SeverityCritical {
		t.Fatalf("binary drift should be critical: %+v", rep.Findings)
	}
	if !strings.Contains(drift.Message, "/usr/bin/tool") {
		t.Fatalf("drift message = %q", drift.Message)
	}
	if conf == nil || conf.Severity != audit.SeverityInfo {
		t.Fatalf("conffile change should be info: %+v", rep.Findings)
	}
}

func TestDpkgAuditCapsFindings(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < maxDpkgFindings+10; i++ {
		fmt.Fprintf(&sb, "??5??????   /usr/bin/tool%d\n", i)
	}
	rep := fakeDpkg(dpkgStatus, sb.String(), true).Audit(context.Background())

	drifts := 0
	summarized := false
	for _, f := range rep.Findings {
		if f.Code == audit.CodeBinaryDrift {
			drifts++
		}
		if f.Code == audit.CodeInventorySummarized && strings.Contains(f.Message, "further verification") {
			summarized = true
		}
	}
	if drifts != maxDpkgFindings {
		t.Fatalf("drift findings = %d, want %d", drifts, maxDpkgFindings)
	}
	if !summarized {
		t.Fatalf("missing overflow summary: %+v", rep.Findings)
	}
}

func TestDpkgAuditMissingBinary(t *testing.T) {
	rep := fakeDpkg(dpkgStatus, "", false).Audit(context.Background())

	found := false
	for _, f := range rep.Findings {
		if f.Code == audit.CodeManagerUnreadable {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected manager-unreadable: %+v", rep.Findings)
	}
}
