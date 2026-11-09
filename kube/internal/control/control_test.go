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
	"github.com/eclipse-iofog/edgelet-kube/internal/network"
	"github.com/eclipse-iofog/edgelet-kube/internal/stage"
	"github.com/eclipse-iofog/edgelet-kube/scripts/airgap"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func TestMain(m *testing.M) {
	logrus.SetOutput(io.Discard)
	klog.SetOutput(io.Discard)
	startKubelet = func(ctx context.Context, _ []string) error {
		<-ctx.Done()
		return nil
	}
	startKubeProxy = func(ctx context.Context, _ []string) error {
		<-ctx.Done()
		return nil
	}
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
	srv.ClusterCIDR = "10.20.0.0/16"
	srv.ServiceCIDR = "10.50.0.0/16"
	srv.ClusterDNS = "10.50.0.25"
	srv.ClusterDomain = "example.test"
	srv.NodePodCIDR = "10.20.0.0/24"
	srv.DefaultLocalStoragePath = filepath.Join(dir, "volumes")
	extra := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: caller-addon\n  namespace: kube-system\ndata:\n  ok: \"true\"\n")
	cniConf := filepath.Join(dir, "net.d")
	cniBin := filepath.Join(dir, "bin")
	origKubelet := startKubelet
	origProxy := startKubeProxy
	t.Cleanup(func() {
		startKubelet = origKubelet
		startKubeProxy = origProxy
	})
	kubeletStarted := make(chan struct{})
	proxyStarted := make(chan struct{})
	var kubeletArgsSeen []string
	var proxyArgsSeen []string
	startKubelet = func(ctx context.Context, args []string) error {
		if _, err := os.Stat(filepath.Join(cniConf, network.FileName)); err != nil {
			t.Errorf("conflist missing when kubelet started: %v", err)
		}
		kubeletArgsSeen = append([]string(nil), args...)
		close(kubeletStarted)
		<-ctx.Done()
		return nil
	}
	startKubeProxy = func(ctx context.Context, args []string) error {
		proxyArgsSeen = append([]string(nil), args...)
		close(proxyStarted)
		<-ctx.Done()
		return nil
	}
	rt, err := Start(ctx, Config{
		Server:     srv,
		Store:      store,
		CNIConfDir: cniConf,
		CNIBinDir:  cniBin,
		Manifests:  extra,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Stop)
	select {
	case <-kubeletStarted:
	case <-ctx.Done():
		t.Fatal("kubelet was not started")
	}
	select {
	case <-proxyStarted:
	case <-ctx.Done():
		t.Fatal("kube-proxy was not started")
	}
	body, err := os.ReadFile(filepath.Join(cniConf, network.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"hairpinMode":true`) {
		t.Fatalf("conflist %s", body)
	}
	entries, err := os.ReadDir(cniConf)
	if err != nil {
		t.Fatal(err)
	}
	var conflists int
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".conflist") || strings.HasSuffix(name, ".conf") {
			conflists++
		}
	}
	if conflists != 1 {
		t.Fatalf("conflists %d", conflists)
	}
	seen := strings.Join(kubeletArgsSeen, "\n") + "\n" + strings.Join(proxyArgsSeen, "\n")
	for _, want := range []string{
		"--cni-conf-dir=" + cniConf,
		"--cni-bin-dir=" + cniBin,
		"--pod-infra-container-image=" + config.DefaultPauseImage,
		"--proxy-mode=iptables",
		"--cluster-cidr=" + srv.ClusterCIDR,
		"--cgroup-driver=systemd",
	} {
		if !strings.Contains(seen, want) {
			t.Fatalf("missing %s\n%s", want, seen)
		}
	}
	if strings.Contains(seen, "/opt/cni/bin") || strings.Contains(seen, "register-with-taints") || strings.Contains(seen, "cgroupfs") || strings.Contains(seen, "protect-kernel-defaults") {
		t.Fatalf("args %s", seen)
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
	readyBody, err := client.Discovery().RESTClient().Get().AbsPath("/readyz").DoRaw(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readyBody), "ok") {
		t.Fatalf("readyz %q", readyBody)
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

	addons := kubeClient(t, rt.Kubeconfig)
	if _, err := addons.kube.CoreV1().ConfigMaps("kube-system").Get(ctx, "caller-addon", metav1.GetOptions{}); err != nil {
		t.Fatal(err)
	}
	assertAddons(t, addons, srv, true, true, true)
	assertMetricsTop(t, addons, rt.Kubeconfig)
	for _, name := range []string{"coredns", "local-storage", "metrics-server"} {
		disabled := srv
		disabled.Disable = []string{name}
		if err := stage.Apply(ctx, rt.Kubeconfig, disabled, extra); err != nil {
			t.Fatal(err)
		}
		assertAddons(t, addons, srv, name != "coredns", name != "local-storage", name != "metrics-server")
		if _, err := addons.kube.CoreV1().ConfigMaps("kube-system").Get(ctx, "caller-addon", metav1.GetOptions{}); err != nil {
			t.Fatal(err)
		}
		if name == "metrics-server" {
			still, err := addons.kube.Discovery().RESTClient().Get().AbsPath("/apis/apiregistration.k8s.io/v1").DoRaw(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(still), "APIService") {
				t.Fatalf("aggregation API %s", still)
			}
		}
		if err := stage.Apply(ctx, rt.Kubeconfig, srv, extra); err != nil {
			t.Fatal(err)
		}
		assertAddons(t, addons, srv, true, true, true)
	}
}

func TestKubeletAndProxyUseNodeNetwork(t *testing.T) {
	srv := config.Defaults()
	srv.NodeName = "node-a"
	srv.NodeIP = "192.0.2.10"
	srv.DataDir = t.TempDir()
	srv.ClusterCIDR = "10.20.0.0/16"
	srv.ServiceCIDR = "10.50.0.0/16"
	srv.ClusterDNS = "10.50.0.10"
	srv.ClusterDomain = "example.test"
	srv.NodePodCIDR = "10.20.0.0/24"
	srv.PauseImage = "registry.example/pause:test"
	srv.NodeLabel = []string{"disk=ssd"}
	srv.ResolvConf = "/etc/resolv.conf"
	certs := certPaths{
		KubeletKubeconfig:   filepath.Join(srv.DataDir, "kubelet.kubeconfig"),
		KubeProxyKubeconfig: filepath.Join(srv.DataDir, "kube-proxy.kubeconfig"),
	}
	cniConf := filepath.Join(srv.DataDir, "net.d")
	cniBin := filepath.Join(srv.DataDir, "bin")
	kubelet, err := kubeletArgs(srv, srv.NodeIP, certs, cniConf, cniBin)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := kubeProxyArgs(srv, srv.NodeIP, certs)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(kubelet, "\n") + "\n" + strings.Join(proxy, "\n")
	for _, want := range []string{
		"--cni-conf-dir=" + cniConf,
		"--cni-bin-dir=" + cniBin,
		"--pod-infra-container-image=registry.example/pause:test",
		"--container-runtime-endpoint=unix:///run/edgelet-kube/containerd.sock",
		"--cluster-dns=10.50.0.10",
		"--cluster-domain=example.test",
		"--node-ip=192.0.2.10",
		"--hostname-override=node-a",
		"--node-labels=disk=ssd",
		"--resolv-conf=/etc/resolv.conf",
		"--proxy-mode=iptables",
		"--cluster-cidr=10.20.0.0/16",
		"--cgroup-driver=systemd",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s\n%s", want, joined)
		}
	}
	for _, absent := range []string{
		"/opt/cni/bin",
		"register-with-taints",
		"control-plane",
		"cgroupfs",
		"protect-kernel-defaults",
		"disable-kube-proxy",
		"--token",
		"flannel",
	} {
		if strings.Contains(joined, absent) {
			t.Fatalf("argument list contains %s\n%s", absent, joined)
		}
	}
	loaded, err := loadKubelet(kubelet)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CgroupDriver != "systemd" || loaded.ProtectKernelDefaults {
		t.Fatalf("driver %s protect %v", loaded.CgroupDriver, loaded.ProtectKernelDefaults)
	}
	if _, err := loadKubeProxy(proxy); err != nil {
		t.Fatal(err)
	}

	srv.NodeTaint = []string{"dedicated=lab:NoSchedule"}
	withTaint, err := kubeletArgs(srv, srv.NodeIP, certs, cniConf, cniBin)
	if err != nil {
		t.Fatal(err)
	}
	taintArgs := strings.Join(withTaint, "\n")
	if !strings.Contains(taintArgs, "--register-with-taints=dedicated=lab:NoSchedule") {
		t.Fatalf("taint args %s", taintArgs)
	}
	if strings.Contains(taintArgs, "control-plane") {
		t.Fatalf("taint args %s", taintArgs)
	}
	if _, err := kubeletArgs(srv, srv.NodeIP, certs, cniConf, "/opt/cni/bin"); err == nil {
		t.Fatal("accepted the host cni directory")
	}
}

func TestKubeletProtectsKernelDefaults(t *testing.T) {
	srv := config.Defaults()
	srv.NodeName = "node-a"
	srv.NodeIP = "192.0.2.10"
	srv.DataDir = t.TempDir()
	srv.ProtectKernelDefaults = true
	srv.KubeletArg = []string{"protect-kernel-defaults=false", "v=2"}
	certs := certPaths{KubeletKubeconfig: filepath.Join(srv.DataDir, "kubelet.kubeconfig")}
	args, err := kubeletArgs(srv, srv.NodeIP, certs, filepath.Join(srv.DataDir, "net.d"), filepath.Join(srv.DataDir, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\n")
	if !strings.Contains(joined, "--cgroup-driver=systemd") || !strings.Contains(joined, "--protect-kernel-defaults=true") {
		t.Fatalf("args %s", joined)
	}
	if strings.Contains(joined, "cgroupfs") || strings.Contains(joined, "--protect-kernel-defaults=false") {
		t.Fatalf("args %s", joined)
	}
	if !strings.Contains(joined, "--v=2") {
		t.Fatalf("args %s", joined)
	}
	loaded, err := loadKubelet(args)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CgroupDriver != "systemd" || !loaded.ProtectKernelDefaults {
		t.Fatalf("driver %s protect %v", loaded.CgroupDriver, loaded.ProtectKernelDefaults)
	}

	srv.ProtectKernelDefaults = false
	srv.KubeletArg = []string{"cgroup-driver=cgroupfs"}
	if _, err := kubeletArgs(srv, srv.NodeIP, certs, filepath.Join(srv.DataDir, "net.d"), filepath.Join(srv.DataDir, "bin")); err == nil {
		t.Fatal("accepted a cgroupfs driver")
	}
}

func TestPreferBundledIptables(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	aux := filepath.Join(bin, "aux")
	host := filepath.Join(root, "host")
	if err := os.MkdirAll(aux, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(host, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aux, "xtables-legacy-multi"), []byte("iptables"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("xtables-legacy-multi", filepath.Join(aux, "iptables")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host, "iptables"), []byte("host"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", host)

	srv := config.Defaults()
	if !srv.PreferBundledBin {
		t.Fatal("prefer-bundled-bin default")
	}
	if err := applyBundledPath(srv.PreferBundledBin, bin); err != nil {
		t.Fatal(err)
	}
	got, err := exec.LookPath("iptables")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(aux, "iptables") {
		t.Fatalf("iptables %s", got)
	}
	target, err := os.Readlink(got)
	if err != nil {
		t.Fatal(err)
	}
	if target != "xtables-legacy-multi" {
		t.Fatalf("iptables -> %s", target)
	}
	if !strings.HasPrefix(os.Getenv("PATH"), aux+string(os.PathListSeparator)) {
		t.Fatalf("PATH %s", os.Getenv("PATH"))
	}

	if err := applyBundledPath(false, bin); err != nil {
		t.Fatal(err)
	}
	got, err = exec.LookPath("iptables")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(host, "iptables") {
		t.Fatalf("host iptables %s", got)
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
