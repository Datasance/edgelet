package control

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/eclipse-iofog/edgelet-kube/internal/config"
	"github.com/eclipse-iofog/edgelet-kube/internal/datastore"
	"github.com/eclipse-iofog/edgelet-kube/internal/network"
	"github.com/eclipse-iofog/edgelet-kube/internal/paths"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Config selects the API processes started after the datastore is open.
type Config struct {
	// Server is the loaded server configuration.
	Server config.Server
	// Store is the open datastore. The API processes use its endpoint.
	Store *datastore.Store
	// CNIConfDir is the conflist directory. Empty uses the installed directory.
	CNIConfDir string
	// CNIBinDir is the CNI plugin directory in the extracted bundle.
	// Empty uses the directory of this executable.
	CNIBinDir string
	// PodNetwork writes the conflist. Nil writes the bridge conflist.
	// The function does not wait on the apiserver.
	PodNetwork func(dir, podCIDR string) error
}

// Runtime is the running API processes.
type Runtime struct {
	// Kubeconfig is the admin client file.
	Kubeconfig string

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Stop asks the API processes to exit and waits for them to return.
func (r *Runtime) Stop() {
	if r == nil || r.cancel == nil {
		return
	}
	r.cancel()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
	}
	r.cancel = nil
}

// Start runs kube-apiserver against the open datastore, writes the bridge
// conflist, then starts kube-controller-manager, kube-scheduler, kubelet,
// and kube-proxy. Image import has already finished. The conflist write does
// not wait on the apiserver and finishes before kubelet creates a sandbox.
func Start(ctx context.Context, cfg Config) (*Runtime, error) {
	if err := CheckSELinux(cfg.Server.SELinux); err != nil {
		return nil, err
	}
	if err := requireServer(cfg.Server); err != nil {
		return nil, err
	}
	if cfg.Store == nil || cfg.Store.Endpoint == "" {
		return nil, fmt.Errorf("datastore endpoint is empty")
	}
	encPath, err := encryptionConfig(cfg.Server)
	if err != nil {
		return nil, err
	}
	advertise, err := advertiseAddress(cfg.Server)
	if err != nil {
		return nil, err
	}
	paths, err := ensureCerts(cfg.Server)
	if err != nil {
		return nil, err
	}
	kArgs, err := kubeletArgs(cfg.Server, advertise, paths, cfg.CNIConfDir, cfg.CNIBinDir)
	if err != nil {
		return nil, err
	}
	pArgs, err := kubeProxyArgs(cfg.Server, advertise, paths)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	rt := &Runtime{Kubeconfig: paths.AdminKubeconfig, cancel: cancel}
	errCh := make(chan error, 8)
	stop := func(err error) (*Runtime, error) {
		cancel()
		rt.Stop()
		return nil, err
	}
	rt.launch(ctx, "kube-apiserver", errCh, func(ctx context.Context) error {
		return runAPIServer(ctx, apiServerArgs(cfg.Server, cfg.Store.Endpoint, advertise, encPath, paths))
	})
	write := cfg.PodNetwork
	if write == nil {
		write = network.Write
	}
	if err := write(flagValue(kArgs, "cni-conf-dir"), cfg.Server.NodePodCIDR); err != nil {
		return stop(err)
	}
	if err := waitReady(ctx, paths.AdminKubeconfig, errCh); err != nil {
		return stop(err)
	}
	controllerPort, schedulerPort := componentPorts(cfg.Server.HTTPSListenPort, 0, 0)
	controllerArgs, err := controllerManagerArgs(cfg.Server, cfg.Store.LeaderElect, controllerPort, paths)
	if err != nil {
		return stop(err)
	}
	rt.launch(ctx, "kube-controller-manager", errCh, func(ctx context.Context) error {
		return runControllerManager(ctx, controllerArgs)
	})
	rt.launch(ctx, "kube-scheduler", errCh, func(ctx context.Context) error {
		return runScheduler(ctx, schedulerArgs(cfg.Server, cfg.Store.LeaderElect, schedulerPort, paths))
	})
	if err := applyBundledPath(cfg.Server.PreferBundledBin, flagValue(kArgs, "cni-bin-dir")); err != nil {
		return stop(err)
	}
	rt.launch(ctx, "kubelet", errCh, func(ctx context.Context) error {
		return startKubelet(ctx, kArgs)
	})
	rt.launch(ctx, "kube-proxy", errCh, func(ctx context.Context) error {
		return startKubeProxy(ctx, pArgs)
	})
	if err := waitPort(ctx, controllerPort, errCh); err != nil {
		return stop(fmt.Errorf("kube-controller-manager: %w", err))
	}
	if err := waitPort(ctx, schedulerPort, errCh); err != nil {
		return stop(fmt.Errorf("kube-scheduler: %w", err))
	}
	return rt, nil
}

func (r *Runtime) launch(ctx context.Context, name string, errCh chan<- error, run func(context.Context) error) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer func() {
			if rec := recover(); rec != nil {
				report(errCh, name, fmt.Errorf("%v", rec))
			}
		}()
		err := run(ctx)
		if err != nil && ctx.Err() == nil {
			report(errCh, name, err)
		}
	}()
}

// encryptionConfig is the secrets encryption file when startup is allowed
// and the file exists. An empty path omits the apiserver flag.
func encryptionConfig(srv config.Server) (string, error) {
	enc, err := datastore.PrepareEncryption(srv.DataDir, srv.SecretsEncryption)
	if err != nil {
		return "", err
	}
	return enc.ProviderConfig, nil
}

// PreferBundledBin sets PATH before containerd starts.
// The bridge plugin is executed by that child and locates iptables from the child's PATH.
// When prefer is true, the bundled xtables-legacy directory comes first.
func PreferBundledBin(prefer bool) error {
	return applyBundledPath(prefer, "")
}

// applyBundledPath sets PATH for kubelet, kube-proxy, and the containerd child.
// prefer puts the bundle aux directory first, so iptables resolves to xtables-legacy.
func applyBundledPath(prefer bool, cniBinDir string) error {
	_, bin, err := cniDirs("", cniBinDir)
	if err != nil {
		return err
	}
	path := (paths.Layout{BundledBin: filepath.Join(bin, "aux")}).BinPath(prefer, os.Getenv("PATH"))
	return os.Setenv("PATH", path)
}

func requireServer(srv config.Server) error {
	if srv.ClusterCIDR == "" || srv.ServiceCIDR == "" || srv.ClusterDomain == "" || srv.NodePodCIDR == "" {
		return fmt.Errorf("cluster network is not configured")
	}
	if srv.BindAddress == "" || srv.HTTPSListenPort == 0 {
		return fmt.Errorf("API bind address is not configured")
	}
	return nil
}

func waitReady(ctx context.Context, kubeconfig string, errCh <-chan error) error {
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		last = readyz(ctx, kubeconfig)
		if last == nil {
			return nil
		}
		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("kube-apiserver did not become ready: %w", last)
		case <-ticker.C:
		}
	}
}

func waitPort(ctx context.Context, port int, errCh <-chan error) error {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		last = err
		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("port %d did not open: %w", port, last)
		case <-ticker.C:
		}
	}
}

func readyz(ctx context.Context, kubeconfig string) error {
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	cfg.Timeout = 5 * time.Second
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	body, err := client.Discovery().RESTClient().Get().AbsPath("/readyz").DoRaw(ctx)
	if err != nil {
		return err
	}
	if string(body) != "ok" && len(body) == 0 {
		return fmt.Errorf("readyz returned an empty body")
	}
	return nil
}
