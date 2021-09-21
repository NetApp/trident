module github.com/netapp/trident

go 1.16

require (
	github.com/Azure/azure-sdk-for-go v55.7.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.19
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.8
	github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/to v0.4.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.3.1 // indirect
	github.com/Microsoft/go-winio v0.5.0 // indirect
	github.com/RoaringBitmap/roaring v0.9.2
	github.com/cenkalti/backoff/v4 v4.1.1
	github.com/container-storage-interface/spec v1.5.0
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-plugins-helpers v0.0.0-20210623094020-7ef169fb8b8e
	github.com/dustin/go-humanize v1.0.0
	github.com/evanphx/json-patch/v5 v5.5.0
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32 // 2/12/2019
	github.com/go-openapi/errors v0.20.0
	github.com/go-openapi/runtime v0.19.29
	github.com/go-openapi/strfmt v0.20.1
	github.com/go-openapi/swag v0.19.15
	github.com/go-openapi/validate v0.20.2
	github.com/golang/mock v1.6.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.0.0
	github.com/mattermost/xml-roundtrip-validator v0.1.0
	github.com/mitchellh/copystructure v1.2.0
	github.com/mitchellh/hashstructure/v2 v2.0.2
	github.com/olekukonko/tablewriter v0.0.5
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.26.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.0
	github.com/zcalusic/sysinfo v0.0.0-20210609180555-aff387a52b3a
	go.uber.org/multierr v1.6.0
	golang.org/x/crypto v0.0.0-20210711020723-a769d52b0f97 // github.com/golang/crypto
	golang.org/x/oauth2 v0.0.0-20210628180205-a41e5a781914 // github.com/golang/oauth2
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // github.com/golang/sys
	google.golang.org/grpc v1.39.0 // github.com/grpc/grpc-go
	k8s.io/api v0.21.3 // github.com/kubernetes/api
	k8s.io/apiextensions-apiserver v0.21.3 // github.com/kubernetes/apiextensions-apiserver
	k8s.io/apimachinery v0.21.3 // github.com/kubernetes/apimachinery
	k8s.io/client-go v0.21.3 // github.com/kubernetes/client-go
)
