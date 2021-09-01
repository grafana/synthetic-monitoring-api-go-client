module github.com/grafana/synthetic-monitoring-api-go-client

go 1.17

require (
	github.com/google/go-cmp v0.5.6
	github.com/grafana/synthetic-monitoring-agent v0.1.4
	github.com/stretchr/testify v1.7.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/kr/pretty v0.2.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/net v0.0.0-20210525063256-abc453219eb5 // indirect
	golang.org/x/sys v0.0.0-20210603081109-ebe580a85c40 // indirect
	golang.org/x/text v0.3.6 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	google.golang.org/grpc v1.40.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

// Without the following replace, you get an error like
//
//     k8s.io/client-go@v12.0.0+incompatible: invalid version: +incompatible suffix not allowed: module contains a go.mod file, so semantic import versioning is required
//
// This is telling you that you cannot have a version 12.0.0 and tag
// that as "incompatible", that you should be calling the module
// something like "k8s.io/client-go/v12".
//
// 78d2af792bab is the commit tagged as v12.0.0.

replace k8s.io/client-go => k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
