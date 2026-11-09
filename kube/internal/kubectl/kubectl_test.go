package kubectl

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet-kube/scripts/airgap"
)

func TestVersionReportsPin(t *testing.T) {
	stdout, stderr := capture(t, func() {
		if err := Run([]string{"--data-dir", t.TempDir(), "version", "--client=true"}); err != nil {
			t.Fatal(err)
		}
	})
	got := stdout + stderr
	if !strings.Contains(got, airgap.KubernetesVersion()) {
		t.Fatalf("kubectl version\n%s", got)
	}
}

func TestDataDirFlagIsRemoved(t *testing.T) {
	got := withoutRuntimeFlags([]string{"--data-dir", "/var/lib/edgelet-kube", "version", "--client=true"})
	if strings.Join(got, " ") != "version --client=true" {
		t.Fatalf("args %v", got)
	}
	got = withoutRuntimeFlags([]string{"--data-dir=/tmp/work", "get", "nodes"})
	if strings.Join(got, " ") != "get nodes" {
		t.Fatalf("args %v", got)
	}
}

func capture(t *testing.T, fn func()) (string, string) {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW
	fn()
	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	var stdout, stderr bytes.Buffer
	if _, err := io.Copy(&stdout, outR); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(&stderr, errR); err != nil {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String()
}
