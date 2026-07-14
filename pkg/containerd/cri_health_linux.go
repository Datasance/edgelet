//go:build linux && cgo

package containerd

import (
	"context"
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/constants"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const criHealthProbeTimeout = 2 * time.Second

// criRuntimeUsable reports whether the CRI plugin can serve runtime API calls.
// Socket Version() alone is insufficient during data-plane restart (shim load errors).
// Uses a direct gRPC probe to avoid importing pkg/engine/edgelet/cri (import cycle via volumemount/statusreporter).
func criRuntimeUsable() bool {
	socketPath := strings.TrimPrefix(constants.EdgeletContainerdSocket, "unix://")
	conn, err := grpc.NewClient("unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return false
	}
	defer func() {
		_ = conn.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), criHealthProbeTimeout)
	defer cancel()

	runtime := runtimeapi.NewRuntimeServiceClient(conn)
	_, err = runtime.ListContainers(ctx, &runtimeapi.ListContainersRequest{})
	return err == nil
}
