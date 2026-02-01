module github.com/eclipse-iofog/agent-go

go 1.24.3

require (
	github.com/Azure/go-amqp v1.5.1
	github.com/docker/docker v24.0.7+incompatible
	github.com/docker/go-connections v0.6.0
	github.com/fsnotify/fsnotify v1.9.0
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/gorilla/websocket v1.5.3
	github.com/shirou/gopsutil/v4 v4.25.12
	github.com/sirupsen/logrus v1.9.3
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/yusufpapurcu/wmi v1.2.4
	golang.org/x/crypto v0.47.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Microsoft/go-winio v0.4.21 // indirect
	github.com/docker/distribution v2.8.3+incompatible // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	gotest.tools/v3 v3.5.2 // indirect
)

// Fix Docker distribution compatibility issue
replace github.com/docker/distribution => github.com/docker/distribution v2.8.2+incompatible
