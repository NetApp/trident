module github.com/netapp/trident

go 1.23.0

require (
	cloud.google.com/go/netapp v1.4.0 // https://pkg.go.dev/cloud.google.com/go/netapp
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.14.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/netapp/armnetapp/v7 v7.2.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph v0.9.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armfeatures v1.2.0
	github.com/RoaringBitmap/roaring/v2 v2.3.5
	github.com/aws/aws-sdk-go-v2 v1.32.2
	github.com/aws/aws-sdk-go-v2/config v1.27.43
	github.com/aws/aws-sdk-go-v2/credentials v1.17.41
	github.com/aws/aws-sdk-go-v2/service/fsx v1.49.2
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.34.2
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/container-storage-interface/spec v1.8.0
	github.com/docker/go-plugins-helpers v0.0.0-20240701071450-45e2431495c8
	github.com/dustin/go-humanize v1.0.2-0.20231009183035-961771c7ab99
	github.com/elastic/go-sysinfo v1.14.2
	github.com/evanphx/json-patch/v5 v5.9.0
	github.com/ghodss/yaml v1.0.1-0.20220118164431-d8423dcdf344 // 1/18/2022
	github.com/go-openapi/errors v0.22.0
	github.com/go-openapi/runtime v0.28.0
	github.com/go-openapi/strfmt v0.23.0
	github.com/go-openapi/swag v0.23.0
	github.com/go-openapi/validate v0.24.0
	github.com/golang/protobuf v1.5.4
	github.com/google/go-cmp v0.6.0
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/hpe-storage/common-host-libs v4.7.1+incompatible
	github.com/jarcoal/httpmock v1.3.1
	github.com/kr/secureheader v0.2.0
	github.com/kubernetes-csi/csi-lib-utils v0.16.0
	github.com/kubernetes-csi/csi-proxy/client v1.1.3
	github.com/kubernetes-csi/external-snapshotter/client/v8 v8.0.0
	github.com/mattermost/xml-roundtrip-validator v0.1.1-0.20230502164821-3079e7b80fca
	github.com/mitchellh/copystructure v1.2.0
	github.com/mitchellh/hashstructure/v2 v2.0.2
	github.com/olekukonko/tablewriter v0.0.6-0.20230925090304-df64c4bbad77
	github.com/openshift/api v0.0.0-20241009131553-a1523024209f
	github.com/prometheus/client_golang v1.20.4
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/afero v1.11.0
	github.com/spf13/cobra v1.8.1
	github.com/stretchr/testify v1.9.0
	github.com/vishvananda/netlink v1.1.0
	github.com/zcalusic/sysinfo v1.1.2
	go.uber.org/mock v0.4.0
	go.uber.org/multierr v1.11.0 // github.com/uber-go/multierr
	golang.org/x/crypto v0.28.0 // github.com/golang/crypto
	golang.org/x/net v0.30.0 // github.com/golang/net
	golang.org/x/oauth2 v0.23.0 // github.com/golang/oauth2
	golang.org/x/sys v0.26.0 // github.com/golang/sys
	golang.org/x/text v0.19.0 // github.com/golang/text
	golang.org/x/time v0.7.0 // github.com/golang/time
	google.golang.org/api v0.200.0
	google.golang.org/grpc v1.67.1 // github.com/grpc/grpc-go
	google.golang.org/protobuf v1.35.1 // github.com/protocolbuffers/protobuf-go
	k8s.io/api v0.31.1 // github.com/kubernetes/api
	k8s.io/apiextensions-apiserver v0.31.1 // github.com/kubernetes/apiextensions-apiserver
	k8s.io/apimachinery v0.31.1 // github.com/kubernetes/apimachinery
	k8s.io/client-go v0.31.1 // github.com/kubernetes/client-go
	k8s.io/mount-utils v0.31.1 // github.com/kubernetes/mount-utils
	k8s.io/utils v0.0.0-20240921022957-49e7df575cb6 // github.com/kubernetes/utils
	sigs.k8s.io/cloud-provider-azure/pkg/azclient v0.0.50 // github.com/kubernetes-sigs/cloud-provider-azure
)

require (
	cloud.google.com/go v0.115.1 // indirect
	cloud.google.com/go/auth v0.9.8 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.4 // indirect
	cloud.google.com/go/compute/metadata v0.5.2 // indirect
	cloud.google.com/go/longrunning v0.6.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2 v2.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v4 v4.8.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault v1.4.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources v1.2.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage v1.6.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets v1.1.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.0.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.24.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.28.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.32.2 // indirect
	github.com/aws/smithy-go v1.22.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.12.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-systemd v0.0.0-20190321100706-95778dfbb74e // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/elastic/go-windows v1.0.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.23.0 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/jsonreference v0.21.0 // indirect
	github.com/go-openapi/loads v0.22.0 // indirect
	github.com/go-openapi/spec v0.21.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/googleapis/gax-go/v2 v2.13.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-runewidth v0.0.10 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/spdystream v0.4.0 // indirect
	github.com/moby/sys/mountinfo v0.7.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mschoch/smat v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/opencontainers/runc v1.1.13 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20220909204839-494a5a6aca78 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/rivo/uniseg v0.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.mongodb.org/mongo-driver v1.14.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.54.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.54.0 // indirect
	go.opentelemetry.io/otel v1.29.0 // indirect
	go.opentelemetry.io/otel/metric v1.29.0 // indirect
	go.opentelemetry.io/otel/trace v1.29.0 // indirect
	golang.org/x/mod v0.20.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/term v0.25.0 // indirect
	golang.org/x/tools v0.24.0 // indirect
	google.golang.org/genproto v0.0.0-20241007155032-5fefd90f89a9 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240930140551-af27646dc61f // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241007155032-5fefd90f89a9 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v0.0.0-20181124034731-591f970eefbb // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20240228011516-70dd3763d340 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.4.0 // indirect
)
