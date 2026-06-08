//go:build windows

package version

import "os/exec"

func setDetachedProcAttr(_ *exec.Cmd) {}
