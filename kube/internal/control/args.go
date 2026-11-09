package control

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/eclipse-iofog/edgelet-kube/internal/config"
	"github.com/eclipse-iofog/edgelet-kube/internal/version"
)

func init() {
	version.Pin()
}

func apiServerArgs(srv config.Server, endpoint, advertise, encryptionFile string, paths certPaths) []string {
	issuer := "https://kubernetes.default.svc." + srv.ClusterDomain
	values := map[string]string{
		"etcd-servers":                       endpoint,
		"storage-backend":                    "etcd3",
		"allow-privileged":                   "true",
		"authorization-mode":                 "Node,RBAC",
		"anonymous-auth":                     "false",
		"service-account-signing-key-file":   paths.ServiceAccountKey,
		"service-account-key-file":           paths.ServiceAccountCert,
		"service-account-issuer":             issuer,
		"api-audiences":                      issuer,
		"service-cluster-ip-range":           srv.ServiceCIDR,
		"service-node-port-range":            srv.ServiceNodePortRange,
		"secure-port":                        strconv.Itoa(srv.HTTPSListenPort),
		"bind-address":                       srv.BindAddress,
		"tls-cert-file":                      paths.ServingCert,
		"tls-private-key-file":               paths.ServingKey,
		"client-ca-file":                     paths.CACert,
		"kubelet-certificate-authority":      paths.CACert,
		"requestheader-client-ca-file":       paths.FrontProxyCA,
		"requestheader-allowed-names":        "front-proxy-client",
		"proxy-client-cert-file":             paths.FrontProxyCert,
		"proxy-client-key-file":              paths.FrontProxyKey,
		"requestheader-extra-headers-prefix": "X-Remote-Extra-",
		"requestheader-group-headers":        "X-Remote-Group",
		"requestheader-username-headers":     "X-Remote-User",
		"enable-aggregator-routing":          "true",
		"enable-admission-plugins":           "NodeRestriction",
		"profiling":                          "false",
		"cert-dir":                           paths.Dir,
	}
	if advertise != "" {
		values["advertise-address"] = advertise
	}
	if encryptionFile != "" {
		values["encryption-provider-config"] = encryptionFile
	}
	if srv.DatastoreCAFile != "" {
		values["etcd-cafile"] = srv.DatastoreCAFile
	}
	if srv.DatastoreCertFile != "" {
		values["etcd-certfile"] = srv.DatastoreCertFile
	}
	if srv.DatastoreKeyFile != "" {
		values["etcd-keyfile"] = srv.DatastoreKeyFile
	}
	if srv.V != 0 {
		values["v"] = strconv.Itoa(srv.V)
	}
	if srv.VModule != "" {
		values["vmodule"] = srv.VModule
	}
	return flagArgs(values, srv.KubeAPIServerArg)
}

func controllerManagerArgs(srv config.Server, leader bool, port int, paths certPaths) ([]string, error) {
	mask, err := prefixSize(srv.NodePodCIDR)
	if err != nil {
		return nil, err
	}
	values := map[string]string{
		"kubeconfig":                       paths.ControllerKubeconfig,
		"authorization-kubeconfig":         paths.ControllerKubeconfig,
		"authentication-kubeconfig":        paths.ControllerKubeconfig,
		"service-account-private-key-file": paths.ServiceAccountKey,
		"allocate-node-cidrs":              "true",
		"cluster-cidr":                     srv.ClusterCIDR,
		"node-cidr-mask-size":              mask,
		"service-cluster-ip-range":         srv.ServiceCIDR,
		"root-ca-file":                     paths.CACert,
		"cluster-signing-cert-file":        paths.CACert,
		"cluster-signing-key-file":         paths.CAKey,
		"use-service-account-credentials":  "true",
		"leader-elect":                     strconv.FormatBool(leader),
		"bind-address":                     "127.0.0.1",
		"secure-port":                      strconv.Itoa(port),
		"tls-cert-file":                    paths.ServingCert,
		"tls-private-key-file":             paths.ServingKey,
		"cert-dir":                         paths.Dir,
		"profiling":                        "false",
		"controllers":                      "*,tokencleaner",
	}
	if srv.V != 0 {
		values["v"] = strconv.Itoa(srv.V)
	}
	if srv.VModule != "" {
		values["vmodule"] = srv.VModule
	}
	return flagArgs(values, srv.KubeControllerManagerArg), nil
}

func schedulerArgs(srv config.Server, leader bool, port int, paths certPaths) []string {
	values := map[string]string{
		"kubeconfig":                paths.SchedulerKubeconfig,
		"authorization-kubeconfig":  paths.SchedulerKubeconfig,
		"authentication-kubeconfig": paths.SchedulerKubeconfig,
		"leader-elect":              strconv.FormatBool(leader),
		"bind-address":              "127.0.0.1",
		"secure-port":               strconv.Itoa(port),
		"tls-cert-file":             paths.ServingCert,
		"tls-private-key-file":      paths.ServingKey,
		"cert-dir":                  paths.Dir,
		"profiling":                 "false",
	}
	if srv.V != 0 {
		values["v"] = strconv.Itoa(srv.V)
	}
	if srv.VModule != "" {
		values["vmodule"] = srv.VModule
	}
	return flagArgs(values, srv.KubeSchedulerArg)
}

func prefixSize(cidr string) (string, error) {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("node pod network %q is not a network", cidr)
	}
	ones, _ := network.Mask.Size()
	return strconv.Itoa(ones), nil
}

func flagArgs(values map[string]string, extra []string) []string {
	merged := make(map[string]string, len(values)+len(extra))
	for key, value := range values {
		merged[key] = value
	}
	for _, raw := range extra {
		key, value := splitArg(raw)
		if key == "" {
			continue
		}
		merged[key] = value
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys))
	for _, key := range keys {
		args = append(args, "--"+key+"="+merged[key])
	}
	return args
}

func splitArg(raw string) (string, string) {
	raw = strings.TrimLeft(raw, "-")
	key, value, ok := strings.Cut(raw, "=")
	if key == "" {
		return "", ""
	}
	if !ok {
		return key, "true"
	}
	return key, value
}

func componentPorts(api, controller, scheduler int) (int, int) {
	if controller == 0 {
		if api == config.DefaultHTTPSListenPort {
			controller = 10257
		} else {
			controller = api + 1
		}
	}
	if scheduler == 0 {
		if api == config.DefaultHTTPSListenPort {
			scheduler = 10259
		} else {
			scheduler = api + 2
		}
	}
	return controller, scheduler
}
