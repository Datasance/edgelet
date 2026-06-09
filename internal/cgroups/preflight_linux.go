//go:build linux && !cgo

package cgroups

// DetectPreflight inspects the host for init start_pre hooks (no cgroup mutation).
// Implementation is procfs/cgroupfs-only so the thin CLI does not link containerd/cgroups.
func DetectPreflight() (*CgroupPolicy, error) {
	mode, mount, hybridWarning := detectModeHost()
	nested := detectNestedHost()
	machineRoot := DetectMachineRoot()
	delegatedList := checkDelegatedControllersForMode(mount, mode)
	delegated := delegatedSet(delegatedList)

	driver, systemdCgroup := selectDriverHost(mode, nested, delegated)

	agentPath, containerdPath := cgroupPaths(mode, driver)

	return &CgroupPolicy{
		Mode:                 mode,
		Driver:               driver,
		Nested:               nested,
		MachineRoot:          machineRoot,
		SystemdCgroup:        systemdCgroup,
		UnifiedMountpoint:    mount,
		AgentCgroupPath:      agentPath,
		ContainerdCgroupPath: containerdPath,
		DelegatedControllers: delegatedList,
		HybridWarning:        hybridWarning,
	}, nil
}
