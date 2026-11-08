module github.com/eclipse-iofog/edgelet-kube

go 1.26.10

require (
	github.com/eclipse-iofog/edgelet v0.0.0
	github.com/klauspost/compress v1.20.3
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/sirupsen/logrus v1.10.2 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace github.com/eclipse-iofog/edgelet => ../
