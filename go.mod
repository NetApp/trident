module github.com/netapp/trident

go 1.14

require (
	github.com/Azure/azure-sdk-for-go v52.6.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.18
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.7
	github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Microsoft/go-winio v0.4.16 // indirect
	github.com/RoaringBitmap/roaring v0.5.5
	github.com/cenkalti/backoff/v4 v4.1.0
	github.com/container-storage-interface/spec v1.3.0
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-plugins-helpers v0.0.0-20200102110956-c9a8a2d92ccc
	github.com/dustin/go-humanize v1.0.0
	github.com/evanphx/json-patch/v5 v5.1.0
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32 // 2/12/2019
	github.com/go-logfmt/logfmt v0.5.0
	github.com/golang/protobuf v1.5.1
	github.com/google/go-cmp v0.5.5
	github.com/google/uuid v1.2.0
	github.com/gorilla/mux v1.8.0
	github.com/mitchellh/copystructure v1.1.1
	github.com/mitchellh/hashstructure/v2 v2.0.1
	github.com/olekukonko/tablewriter v0.0.5
	github.com/prometheus/client_golang v1.10.0
	github.com/prometheus/common v0.19.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.0
	github.com/zcalusic/sysinfo v0.0.0-20210226105846-b810d137e525
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2 // github.com/golang/crypto
	golang.org/x/oauth2 v0.0.0-20210323180902-22b0adad7558 // github.com/golang/oauth2
	golang.org/x/sys v0.0.0-20210326220804-49726bf1d181 // github.com/golang/sys
	google.golang.org/grpc v1.36.1 // github.com/grpc/grpc-go
	k8s.io/api v0.20.5 // github.com/kubernetes/api
	k8s.io/apiextensions-apiserver v0.20.5 // github.com/kubernetes/apiextensions-apiserver
	k8s.io/apimachinery v0.20.5 // github.com/kubernetes/apimachinery
	k8s.io/client-go v0.20.5 // github.com/kubernetes/client-go
)
