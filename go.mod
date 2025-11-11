module github.com/netapp/trident

go 1.25

require (
	cloud.google.com/go/compute v1.49.0
	cloud.google.com/go/netapp v1.10.1 // https://pkg.go.dev/cloud.google.com/go/netapp
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.19.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/netapp/armnetapp/v7 v7.7.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph v0.9.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armfeatures v1.2.0
	github.com/RoaringBitmap/roaring/v2 v2.10.0
	github.com/aws/aws-sdk-go-v2 v1.39.2
	github.com/aws/aws-sdk-go-v2/config v1.31.12
	github.com/aws/aws-sdk-go-v2/credentials v1.18.16
	github.com/aws/aws-sdk-go-v2/service/fsx v1.62.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.39.6
	github.com/brunoga/deep v1.2.5
	github.com/cenkalti/backoff/v4 v4.3.0 //v5.0.2
	github.com/container-storage-interface/spec v1.11.0
	github.com/distribution/reference v0.6.0
	github.com/docker/go-plugins-helpers v0.0.0-20240701071450-45e2431495c8
	github.com/dustin/go-humanize v1.0.2-0.20250512220336-b48bc01a0676
	github.com/elastic/go-sysinfo v1.15.4
	github.com/evanphx/json-patch/v5 v5.9.11
	github.com/ghodss/yaml v1.0.1-0.20220118164431-d8423dcdf344 // 1/18/2022
	github.com/go-openapi/errors v0.22.3
	github.com/go-openapi/runtime v0.29.0
	github.com/go-openapi/strfmt v0.24.0
	github.com/go-openapi/swag v0.25.1
	github.com/go-openapi/validate v0.25.0
	github.com/golang/protobuf v1.5.4
	github.com/google/go-cmp v0.7.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/hpe-storage/common-host-libs v4.7.1+incompatible
	github.com/jarcoal/httpmock v1.4.1
	github.com/kr/secureheader v0.2.0
	github.com/kubernetes-csi/csi-lib-utils v0.22.0
	github.com/kubernetes-csi/csi-proxy/client v1.3.0
	github.com/kubernetes-csi/external-snapshotter/client/v8 v8.2.0
	github.com/mattermost/xml-roundtrip-validator v0.1.0
	github.com/mitchellh/hashstructure/v2 v2.0.2
	github.com/olekukonko/tablewriter v0.0.5
	github.com/openshift/api v0.0.0-20251013165757-fe48e8fd548b
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.23.2
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/afero v1.15.0
	github.com/spf13/cobra v1.10.1
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	github.com/vishvananda/netlink v1.3.1
	github.com/zcalusic/sysinfo v1.1.3
	go.uber.org/mock v0.6.0
	go.uber.org/multierr v1.11.0 // github.com/uber-go/multierr
	golang.org/x/crypto v0.43.0 // github.com/golang/crypto
	golang.org/x/net v0.46.0 // github.com/golang/net
	golang.org/x/oauth2 v0.32.0 // github.com/golang/oauth2
	golang.org/x/sync v0.17.0
	golang.org/x/sys v0.37.0 // github.com/golang/sys
	golang.org/x/text v0.30.0 // github.com/golang/text
	golang.org/x/time v0.14.0 // github.com/golang/time
	google.golang.org/api v0.252.0
	google.golang.org/grpc v1.76.0 // github.com/grpc/grpc-go
	google.golang.org/protobuf v1.36.10 // github.com/protocolbuffers/protobuf-go
	gopkg.in/yaml.v2 v2.4.0 // github.com/go-yaml/yaml
	k8s.io/api v0.34.1 // github.com/kubernetes/api
	k8s.io/apiextensions-apiserver v0.34.1 // github.com/kubernetes/apiextensions-apiserver
	k8s.io/apimachinery v0.34.1 // github.com/kubernetes/apimachinery
	k8s.io/client-go v0.34.1 // github.com/kubernetes/client-go
	k8s.io/mount-utils v0.34.1 // github.com/kubernetes/mount-utils
	k8s.io/utils v0.0.0-20251002143259-bc988d571ff4 // github.com/kubernetes/utils
	sigs.k8s.io/cloud-provider-azure/pkg/azclient v0.9.3 // github.com/kubernetes-sigs/cloud-provider-azure
)

require (
	cloud.google.com/go v0.121.6 // indirect
	cloud.google.com/go/auth v0.17.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/longrunning v0.6.7 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.12.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2 v2.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6 v6.4.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6 v6.6.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault v1.5.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/msi/armmsi v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6 v6.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns v1.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage/v2 v2.0.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets v1.4.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.2.0 // indirect
	github.com/Azure/msi-dataplane v0.4.3 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.9 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.29.6 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.38.6 // indirect
	github.com/aws/smithy-go v1.23.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.20.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/elastic/go-windows v1.0.2 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.24.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.1 // indirect
	github.com/go-openapi/jsonreference v0.21.2 // indirect
	github.com/go-openapi/loads v0.23.1 // indirect
	github.com/go-openapi/spec v0.22.0 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.1 // indirect
	github.com/go-openapi/swag/conv v0.25.1 // indirect
	github.com/go-openapi/swag/fileutils v0.25.1 // indirect
	github.com/go-openapi/swag/jsonname v0.25.1 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.1 // indirect
	github.com/go-openapi/swag/loading v0.25.1 // indirect
	github.com/go-openapi/swag/mangling v0.25.1 // indirect
	github.com/go-openapi/swag/netutils v0.25.1 // indirect
	github.com/go-openapi/swag/stringutils v0.25.1 // indirect
	github.com/go-openapi/swag/typeutils v0.25.1 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.6 // indirect
	github.com/googleapis/gax-go/v2 v2.15.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/moby/spdystream v0.5.0 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.mongodb.org/mongo-driver v1.17.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.61.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.61.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/exp v0.0.0-20250718183923-645b1fa84792 // indirect
	golang.org/x/mod v0.28.0 // indirect
	golang.org/x/term v0.36.0 // indirect
	golang.org/x/tools v0.37.0 // indirect
	google.golang.org/genproto v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250818200422-3122310a409c // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251002232023-7c0ddcbb5797 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v1.0.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250710124328-f3f2b991d03b // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)
