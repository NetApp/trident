#######
trident
#######

Trident exposes several command-line options. Normally the defaults will suffice, but you may want to modify them in
your deployment. They are:

Logging
"""""""

* ``-debug``: Optional; enables debugging output.
* ``-loglevel <level>``: Optional; sets the logging level (debug, info, warn, error, fatal). Defaults to info.

Persistence
"""""""""""

* ``-etcd_v3 <address> or -etcd_v2 <address>``: Required; use this to specify the etcd deployment that Trident should use.
* ``-etcd_v3_cert <file>``: Optional, etcdV3 client certificate.
* ``-etcd_v3_cacert <file>``: Optional, etcdV3 client CA certificate.
* ``-etcd_v3_key <file>``: Optional, etcdV3 client private key.
* ``-no_persistence``: Optional, does not persist any metadata at all.
* ``-passthrough``: Optional, uses backend as the sole source of truth.

Kubernetes
""""""""""

* ``-k8s_pod``: Optional; however, either this or -k8s_api_server must be set to enable Kubernetes support. Setting this will cause Trident to use its containing pod's Kubernetes service account credentials to contact the API server. This only works when Trident runs as a pod in a Kubernetes cluster with service accounts enabled.
* ``-k8s_api_server <insecure-address:insecure-port>``: Optional; however, either this or -k8s_pod must be used to enable Kubernetes support. When specified, Trident will connect to the Kubernetes API server using the provided insecure address and port. This allows Trident to be deployed outside of a pod; however, it only supports insecure connections to the API server. To connect securely, deploy Trident in a pod with the -k8s_pod option.
* ``-k8s_config_path <file>``: Optional; path to a KubeConfig file.

Docker
""""""

* ``-volume_driver <name>``: Optional; driver name used when registering the Docker plugin. Defaults to 'netapp'.
* ``-driver_port <port-number>``: Optional; listen on this port rather than a UNIX domain socket.
* ``-config <file>``: Path to a backend configuration file.

REST
""""

* ``-address <ip-or-host>``: Optional; specifies the address on which Trident's REST server should listen. Defaults to localhost. When listening on localhost and running inside a Kubernetes pod, the REST interface will not be directly accessible from outside the pod. Use -address "" to make the REST interface accessible from the pod IP address.
* ``-port <port-number>``: Optional; specifies the port on which Trident's REST server should listen. Defaults to 8000.
* ``-rest``: Optional; enable the REST interface. Defaults to true.