module github.com/netapp/trident

go 1.14

require (
	github.com/Azure/azure-sdk-for-go v41.2.0+incompatible
	github.com/Azure/go-autorest/autorest v0.10.0
	github.com/Azure/go-autorest/autorest/azure/auth v0.4.2
	github.com/Azure/go-autorest/autorest/date v0.2.0
	github.com/Azure/go-autorest/autorest/to v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/RoaringBitmap/roaring v0.4.23
	github.com/cenkalti/backoff/v4 v4.0.2
	github.com/container-storage-interface/spec v1.2.0
	github.com/coreos/etcd v3.3.20+incompatible
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-plugins-helpers v0.0.0-20200102110956-c9a8a2d92ccc
	github.com/dustin/go-humanize v1.0.0
	github.com/evanphx/json-patch v4.2.0+incompatible
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32 // 2/12/2019
	github.com/go-logfmt/logfmt v0.5.0
	github.com/golang/protobuf v1.3.5
        github.com/google/go-cmp v0.4.0
        github.com/google/uuid v1.1.1
        github.com/gorilla/mux v1.7.4
        github.com/mitchellh/copystructure v1.0.0
        github.com/mitchellh/hashstructure v1.0.0
        github.com/olekukonko/tablewriter v0.0.4
        github.com/prometheus/client_golang v1.5.1
        github.com/rs/xid v1.2.1
        github.com/sirupsen/logrus v1.5.0
	github.com/spf13/cobra v1.0.0
	github.com/stretchr/testify v1.5.1
	golang.org/x/crypto v0.0.0-20200406173513-056763e48d71 // github.com/golang/crypto
        golang.org/x/net v0.0.0-20191112182307-2180aed22343 // indirect
        golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // github.com/golang/oauth2
        golang.org/x/sys v0.0.0-20200413165638-669c56c373c4 // github.com/golang/sys
        google.golang.org/grpc v1.26.0 // github.com/grpc/grpc-go
        k8s.io/api v0.18.0 // github.com/kubernetes/api
        k8s.io/apiextensions-apiserver v0.18.0 // github.com/kubernetes/apiextensions-apiserver
	k8s.io/apimachinery v0.18.0 // github.com/kubernetes/apimachinery
	k8s.io/client-go v0.18.0 // github.com/kubernetes/client-go
)
