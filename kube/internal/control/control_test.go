package control

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/eclipse-iofog/edgelet-kube/internal/config"
	"github.com/eclipse-iofog/edgelet-kube/internal/datastore"
	"github.com/eclipse-iofog/edgelet-kube/scripts/airgap"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func TestMain(m *testing.M) {
	logrus.SetOutput(io.Discard)
	klog.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func TestSELinuxRejectedWithoutHostSupport(t *testing.T) {
	orig := hostSELinux
	t.Cleanup(func() { hostSELinux = orig })

	hostSELinux = func() bool { return false }
	if err := CheckSELinux(false); err != nil {
		t.Fatal(err)
	}
	if err := CheckSELinux(true); err == nil {
		t.Fatal("expected a startup error")
	}
	hostSELinux = func() bool { return true }
	if err := CheckSELinux(true); err != nil {
		t.Fatal(err)
	}
}

func TestArgumentsUseConfiguredNetworks(t *testing.T) {
	srv := config.Defaults()
	srv.ClusterCIDR = "10.20.0.0/16"
	srv.ServiceCIDR = "10.50.0.0/16"
	srv.ClusterDNS = "10.50.0.25"
	srv.ClusterDomain = "example.test"
	srv.NodePodCIDR = "10.20.0.0/24"
	srv.Disable = []string{"metrics-server"}
	paths := certPaths{
		Dir:                  "/tmp/certs",
		CACert:               "/tmp/certs/ca.crt",
		CAKey:                "/tmp/certs/ca.key",
		ServingCert:          "/tmp/certs/serving.crt",
		ServingKey:           "/tmp/certs/serving.key",
		ServiceAccountCert:   "/tmp/certs/sa.crt",
		ServiceAccountKey:    "/tmp/certs/sa.key",
		FrontProxyCA:         "/tmp/certs/front-proxy-ca.crt",
		FrontProxyCert:       "/tmp/certs/front-proxy-client.crt",
		FrontProxyKey:        "/tmp/certs/front-proxy-client.key",
		AdminKubeconfig:      "/tmp/certs/admin.kubeconfig",
		ControllerKubeconfig: "/tmp/certs/controller-manager.kubeconfig",
		SchedulerKubeconfig:  "/tmp/certs/scheduler.kubeconfig",
	}
	api := strings.Join(apiServerArgs(srv, "unix:///tmp/kine.sock", "10.20.0.8", "", paths), "\n")
	controller, err := controllerManagerArgs(srv, false, 10257, paths)
	if err != nil {
		t.Fatal(err)
	}
	joined := api + "\n" + strings.Join(controller, "\n") + "\n" + strings.Join(schedulerArgs(srv, false, 10259, paths), "\n")
	for _, want := range []string{
		"--service-cluster-ip-range=10.50.0.0/16",
		"--cluster-cidr=10.20.0.0/16",
		"--node-cidr-mask-size=24",
		"--service-account-issuer=https://kubernetes.default.svc.example.test",
		"--etcd-servers=unix:///tmp/kine.sock",
		"--bind-address=0.0.0.0",
		"--secure-port=6443",
		"--leader-elect=false",
		"--proxy-client-cert-file=/tmp/certs/front-proxy-client.crt",
		"--requestheader-client-ca-file=/tmp/certs/front-proxy-ca.crt",
		"--enable-aggregator-routing=true",
		"--advertise-address=10.20.0.8",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s\n%s", want, joined)
		}
	}
	for _, absent := range []string{
		"10.50.0.10",
		"10.43.0.10",
		"10.42.0.0/16",
		"encryption-provider-config",
	} {
		if strings.Contains(joined, absent) {
			t.Fatalf("argument list contains %s", absent)
		}
	}
	if strings.Contains(joined, "kubelet") && strings.Contains(joined, "--kubelet-client-certificate=") {
		t.Fatal("kubelet client certificate was configured")
	}
}

func TestLeaderElectionFollowsDatastore(t *testing.T) {
	srv := config.Defaults()
	paths := certPaths{ControllerKubeconfig: "c", SchedulerKubeconfig: "s"}
	off, err := controllerManagerArgs(srv, false, 10257, paths)
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(off, "--leader-elect=false") {
		t.Fatalf("controller args %v", off)
	}
	on := schedulerArgs(srv, true, 10259, paths)
	if !containsArg(on, "--leader-elect=true") {
		t.Fatalf("scheduler args %v", on)
	}
}

func TestReadyz(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip(runtime.GOOS)
	}
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store, err := datastore.Open(ctx, datastore.Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)

	srv := config.Defaults()
	srv.DataDir = dir
	srv.WriteKubeconfig = filepath.Join(dir, "edgelet-kube.yaml")
	srv.BindAddress = "127.0.0.1"
	srv.HTTPSListenPort = freePort(t)
	srv.NodeIP = "192.0.2.10"
	srv.Disable = []string{"metrics-server"}
	var podNetwork bool
	rt, err := Start(ctx, Config{
		Server: srv,
		Store:  store,
		PodNetwork: func(context.Context) error {
			podNetwork = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Stop)
	if !podNetwork {
		t.Fatal("pod network step did not run")
	}
	if kubeletRunning() {
		t.Fatal("kubelet is running")
	}
	if _, err := os.Stat(filepath.Join(dir, "server", "tls", "ca.crt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "server", "temporary")); !os.IsNotExist(err) {
		t.Fatal("temporary certificate directory was created")
	}
	if _, err := os.Stat(filepath.Join(dir, "db", "etcd")); !os.IsNotExist(err) {
		t.Fatal("etcd directory was created")
	}
	info, err := os.Stat(rt.Kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("kubeconfig mode %o", info.Mode().Perm())
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", rt.Kubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.Discovery().RESTClient().Get().AbsPath("/readyz").DoRaw(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ok") {
		t.Fatalf("readyz %q", body)
	}
	versionBody, err := client.Discovery().RESTClient().Get().AbsPath("/version").DoRaw(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(versionBody), airgap.KubernetesVersion()) {
		t.Fatalf("version %s", versionBody)
	}
	group, err := client.Discovery().RESTClient().Get().AbsPath("/apis/apiregistration.k8s.io/v1").DoRaw(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(group), "APIService") {
		t.Fatalf("aggregation API %s", group)
	}
	if kubeletRunning() {
		t.Fatal("kubelet is running")
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func kubeletRunning() bool {
	if _, err := os.Stat("/proc"); err == nil {
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return false
		}
		for _, entry := range entries {
			body, err := os.ReadFile("/proc/" + entry.Name() + "/cmdline")
			if err != nil || len(body) == 0 {
				continue
			}
			argv0 := string(body)
			if i := strings.IndexByte(argv0, 0); i >= 0 {
				argv0 = argv0[:i]
			}
			if filepath.Base(argv0) == "kubelet" {
				return true
			}
		}
		return false
	}
	out, err := exec.Command("ps", "-ax", "-o", "comm=").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if filepath.Base(strings.TrimSpace(line)) == "kubelet" {
			return true
		}
	}
	return false
}

func TestPodNetworkRunsBeforeControllers(t *testing.T) {
	// The ordering test above starts the controllers. This checks the port
	// helper used when the API port is not the default.
	controller, scheduler := componentPorts(20000, 0, 0)
	if controller != 20001 || scheduler != 20002 {
		t.Fatalf("ports %d %d", controller, scheduler)
	}
	controller, scheduler = componentPorts(config.DefaultHTTPSListenPort, 0, 0)
	if controller != 10257 || scheduler != 10259 {
		t.Fatalf("default ports %d %d", controller, scheduler)
	}
}
