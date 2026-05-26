## edgelet config

Update agent configuration

### Synopsis

Update agent configuration via flags.

Each setting has a long --kebab-case flag and a short --alias flag (see flag help below).
Use config cert to install a controller CA certificate, or config switch to change profile.

```
edgelet config [flags]
```

### Examples

```
# Long flags
  edgelet config --controller-url http://localhost:51121/api/v3
  edgelet config --change-frequency-seconds 10 --status-frequency-seconds 10
  edgelet config --disk-limit-gib 20 --memory-limit-mib 512

# Short alias flags
  edgelet config --a http://localhost:51121/api/v3 --cf 10 --sf 10

# Install controller CA certificate (base64-encoded PEM string)
  edgelet config cert <base64-encoded-cert-string>

# Switch configuration profile
  edgelet config switch prod

# Structured output (global flags before subcommand)
  edgelet -o json config --cf 10
```

### Options

```
      --arch string                    fog type/arch. Alias: --ft
      --available-disk-threshold int   available disk threshold. Alias: --dt
      --change-frequency-seconds int   change polling frequency (seconds). Alias: --cf
      --container-engine string        container engine (docker|podman|edgelet). Alias: --ce
      --controller-cert string         controller CA certificate file path. Alias: --ac
      --controller-url string          controller URL. Alias: --a
      --cpu-limit-percent float        CPU limit (%). Alias: --p
      --dev-mode                       developer mode. Alias: --dev
      --device-scan-frequency int      device scan frequency (seconds). Alias: --sd
      --disk-directory string          disk directory. Alias: --dl
      --disk-limit-gib float           disk usage limit (GiB). Alias: --d
      --docker-pruning-frequency int   prune frequency. Alias: --pf
      --docker-url string              runtime socket URL. Alias: --c
      --edge-guard-frequency int       edge guard frequency. Alias: --egf
      --gps-coordinates string         GPS coordinates lat,lon. Alias: --gpsc
      --gps-device string              GPS device. Alias: --gpsd
      --gps-mode string                GPS mode (auto|dynamic|manual|off). Alias: --gps
      --gps-scan-frequency int         GPS scan frequency. Alias: --gpsf
  -h, --help                           help for config
      --log-disk-directory string      log directory. Alias: --ld
      --log-disk-limit-gib float       log disk limit (GiB). Alias: --l
      --log-file-count int             log file count. Alias: --lc
      --log-level string               log level (DEBUG|INFO|WARN|ERROR). Alias: --ll
      --memory-limit-mib float         memory limit (MiB). Alias: --m
      --network-interface string       network interface. Alias: --n
      --secure-mode                    secure mode. Alias: --sec
      --status-frequency-seconds int   status frequency (seconds). Alias: --sf
      --timezone string                timezone. Alias: --tz
      --upgrade-scan-frequency int     upgrade scan frequency. Alias: --uf
      --watchdog-enabled               watchdog enable. Alias: --idc
```

### Options inherited from parent commands

```
      --debug            Debug logging
      --no-color         Disable color and interactive UX
  -o, --output string    Output format: human, json, yaml (default "human")
      --quiet            Suppress interactive progress output
      --socket string    Edgelet API unix socket path
      --timeout string   Request timeout
      --verbose          Verbose logging
```

### SEE ALSO

* [edgelet](edgelet.md)	 - Local CLI for the Edgelet daemon
* [edgelet config cert](edgelet_config_cert.md)	 - Install controller CA certificate
* [edgelet config switch](edgelet_config_switch.md)	 - Switch configuration profile


