module github.com/netapp/trident

go 1.15

require (
	github.com/Azure/azure-sdk-for-go v50.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.15
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.5
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
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.4
	github.com/google/uuid v1.1.4
	github.com/gorilla/mux v1.8.0
	github.com/mitchellh/copystructure v1.0.0
	github.com/mitchellh/hashstructure/v2 v2.0.1
	github.com/olekukonko/tablewriter v0.0.4
	github.com/prometheus/client_golang v1.9.0
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v1.1.1
	github.com/stretchr/testify v1.6.1
	github.com/vishvananda/netlink v1.1.0
	github.com/zcalusic/sysinfo v0.0.0-20201228191146-22fe1fd10b0f
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad // github.com/golang/crypto
	golang.org/x/oauth2 v0.0.0-20210112200429-01de73cf58bd // github.com/golang/oauth2
	golang.org/x/sys v0.0.0-20210112091331-59c308dcf3cc // github.com/golang/sys
	google.golang.org/grpc v1.34.1 // github.com/grpc/grpc-go
	k8s.io/api v0.20.1 // github.com/kubernetes/api
	k8s.io/apiextensions-apiserver v0.20.1 // github.com/kubernetes/apiextensions-apiserver
	k8s.io/apimachinery v0.20.1 // github.com/kubernetes/apimachinery
	k8s.io/client-go v0.20.1 // github.com/kubernetes/client-go
)
