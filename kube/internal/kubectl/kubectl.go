// Package kubectl runs the Kubernetes client embedded in the fat runtime.
package kubectl

import (
	"strings"

	"github.com/eclipse-iofog/edgelet-kube/internal/bundle"
	"github.com/eclipse-iofog/edgelet-kube/internal/version"
	"k8s.io/kubectl/pkg/cmd"
)

// Run executes kubectl with args. Runtime flags that belong to this process
// are removed before the client sees them.
func Run(args []string) error {
	version.Pin()
	command := cmd.NewDefaultKubectlCommand()
	command.SetArgs(withoutRuntimeFlags(args))
	return command.Execute()
}

func withoutRuntimeFlags(args []string) []string {
	flag := "--" + bundle.DataDirFlag
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == flag:
			i++
		case strings.HasPrefix(arg, flag+"="):
		default:
			out = append(out, arg)
		}
	}
	return out
}
