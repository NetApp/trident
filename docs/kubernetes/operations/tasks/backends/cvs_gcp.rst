#############################
Cloud Volumes Service for GCP
#############################

.. note::
  The NetApp Cloud Volumes Service for GCP does not support CVS-Performance volumes less than 100 GiB in size, or CVS
  volumes less than 300 GiB in size. To make it easier to deploy applications, Trident automatically creates volumes of
  the minimum size if a too-small volume is requested. Future releases of the Cloud Volumes Service may remove this
  restriction.


Preparation
-----------

To create and use a Cloud Volumes Service (CVS) for GCP backend, you will need:

* An `GCP account configured with NetApp CVS`_
* Project number of your GCP account
* GCP service account with the ``netappcloudvolumes.admin`` role
* API key file for your CVS service account

Trident now includes support for smaller volumes with the default CVS service type on
GCP (https://cloud.google.com/architecture/partners/netapp-cloud-volumes/service-types).
For backends created with ``storageClass=software``, volumes will now have a
minimum provisioning size of 300 GiB. CVS currently provides this feature under
Controlled Availability and does **not provide** technical support.
Users must sign up for access to sub-1TiB
volumes `here <https://docs.google.com/forms/d/e/1FAIpQLSc7_euiPtlV8bhsKWvwBl3gm9KUL4kOhD7lnbHC3LlQ7m02Dw/viewform>`_.
NetApp recommends customers consume sub-1TiB volumes for **non-production** workloads.

.. warning::

 When deploying backends using the default CVS service type [``storageClass=software``],
 users **must obtain access** to the sub-1TiB volumes feature on GCP for the Project Number(s)
 and Project ID(s) in question. This is necessary for Trident to provision sub-1TiB volumes.
 If not, volume creations **will fail** for PVCs that are <600 GiB. Obtain access to sub-1TiB
 volumes using `this <https://docs.google.com/forms/d/e/1FAIpQLSc7_euiPtlV8bhsKWvwBl3gm9KUL4kOhD7lnbHC3LlQ7m02Dw/viewform>`_
 form.

Backend configuration options
-----------------------------

========================= ================================================================= =================================================
Parameter                 Description                                                       Default
========================= ================================================================= =================================================
version                   Always 1
storageDriverName         "gcp-cvs"
backendName               Custom name for the storage backend                               Driver name + "_" + part of API key
storageClass              Type of storage. Choose from ``hardware`` [CVS-Performance        "hardware"
                          service type] or ``software`` [CVS service type]
projectNumber             GCP account project number
hostProjectNumber         GCP shared VPC host project number
apiRegion                 CVS account region
apiKey                    API key for GCP service account with CVS admin role
proxyURL                  Proxy URL if proxy server required to connect to CVS Account
nfsMountOptions           Fine-grained control of NFS mount options                         "nfsvers=3"
limitVolumeSize           Fail provisioning if requested volume size is above this value    "" (not enforced by default)
network                   GCP network used for CVS volumes                                  "default"
serviceLevel              The CVS service level for new volumes                             "standard"
debugTraceFlags           Debug flags to use when troubleshooting.
                          E.g.: {"api":false, "method":true}                                null
========================= ================================================================= =================================================

The ``apiRegion`` represents the GCP region where Trident creates CVS volumes.
Trident can mount and attach volumes on Kubernetes nodes that belong to the same
GCP region. When creating a backend, it is important to ensure ``apiRegion``
matches the region Kubernetes nodes are deployed in. When creating cross-region
Kubernetes clusters, CVS volumes that are created in a given ``apiRegion`` can
only be used in workloads scheduled on nodes that are in the same GCP region.

.. warning::

  Do not use ``debugTraceFlags`` unless you are troubleshooting and require a
  detailed log dump.

The required value ``projectNumber`` may be found in the GCP web portal's Home screen.  The ``apiRegion`` is the
GCP region where this backend will provision volumes. The ``apiKey`` is the JSON-formatted contents of a GCP
service account's private key file (copied verbatim into the backend config file).  The service account must have
the ``netappcloudvolumes.admin`` role.

If using a shared VPC network, both ``projectNumber`` and ``hostProjectNumber`` must be specified.  In that case,
``projectNumber`` is the service project, and ``hostProjectNumber`` is the host project.

The ``storageClass`` is an optional parameter that can be used to choose the
desired `CVS service type <https://cloud.google.com/solutions/partners/netapp-cloud-volumes/service-types?hl=en_US>`_.
Users can choose from the base CVS service type[``storageClass=software``] or the CVS-Performance service
type [``storageClass=hardware``], which Trident uses by default. Make sure you specify an ``apiRegion`` that
provides the respective CVS ``storageClass`` in your backend definition.

The proxyURL config option must be used if a proxy server is needed to communicate with GCP. The proxy server may either
be an HTTP proxy or an HTTPS proxy. In case of an HTTPS proxy, certificate validation is skipped to allow the usage of
self-signed certificates in the proxy server. Proxy servers with authentication enabled are not supported.

Each backend provisions volumes in a single GCP region. To create volumes in other regions, you can define additional
backends.

The serviceLevel values for CVS on GCP are ``standard``, ``premium``, and ``extreme``.

You can control how each volume is provisioned by default using these options in a special section of the configuration.
For an example, see the configuration examples below.

========================= =============================================================== ================================================
Parameter                 Description                                                     Default
========================= =============================================================== ================================================
exportRule                The export rule(s) for new volumes                              "0.0.0.0/0"
snapshotDir               Controls visibility of the .snapshot directory                  "false"
snapshotReserve           Percentage of volume reserved for snapshots                     "" (accept CVS default of 0)
size                      The size of new volumes                                         "100Gi"
========================= =============================================================== ================================================

The ``exportRule`` value must be a comma-separated list of any combination of
IPv4 addresses or IPv4 subnets in CIDR notation.

.. note::

  For all volumes created on a GCP backend, Trident will copy all labels present
  on a :ref:`storage pool <gcp-virtual-storage-pool>` to the storage volume at
  the time it is provisioned. Storage admins can define labels per storage pool
  and group all volumes created per storage pool. This provides a convenient way
  of differentiating volumes based on a set of customizable labels that are
  provided in the backend configuration.

Example configurations
----------------------

**Example 1 - Minimal backend configuration for gcp-cvs driver**

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "gcp-cvs",
        "projectNumber": "012345678901",
        "apiRegion": "us-west2",
        "apiKey": {
            "type": "service_account",
            "project_id": "my-gcp-project",
            "private_key_id": "1234567890123456789012345678901234567890",
            "private_key": "-----BEGIN PRIVATE KEY-----\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nXsYg6gyxy4zq7OlwWgLwGa==\n-----END PRIVATE KEY-----\n",
            "client_email": "cloudvolumes-admin-sa@my-gcp-project.iam.gserviceaccount.com",
            "client_id": "123456789012345678901",
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token",
            "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
            "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/cloudvolumes-admin-sa%40my-gcp-project.iam.gserviceaccount.com"
        }
    }

**Example 2 - Backend configuration for gcp-cvs driver with the base CVS service type**

This example shows a backend definition that uses the base CVS service type, which
is meant for general-purpose workloads and provides light/moderate performance,
coupled with high zonal availability.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "gcp-cvs",
        "projectNumber": "012345678901",
        "storageClass": "software",
        "apiRegion": "us-east4",
        "apiKey": {
            "type": "service_account",
            "project_id": "my-gcp-project",
            "private_key_id": "1234567890123456789012345678901234567890",
            "private_key": "-----BEGIN PRIVATE KEY-----\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nXsYg6gyxy4zq7OlwWgLwGa==\n-----END PRIVATE KEY-----\n",
            "client_email": "cloudvolumes-admin-sa@my-gcp-project.iam.gserviceaccount.com",
            "client_id": "123456789012345678901",
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token",
            "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
            "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/cloudvolumes-admin-sa%40my-gcp-project.iam.gserviceaccount.com"
        }
    }

**Example 3 -  Backend configuration for gcp-cvs driver with single service level**

This example shows a backend file that applies the same aspects to all Trident created storage in the GCP us-west2
region. This example also shows the usage of proxyURL config option in a backend file.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "gcp-cvs",
        "projectNumber": "012345678901",
        "apiRegion": "us-west2",
        "apiKey": {
            "type": "service_account",
            "project_id": "my-gcp-project",
            "private_key_id": "1234567890123456789012345678901234567890",
            "private_key": "-----BEGIN PRIVATE KEY-----\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nXsYg6gyxy4zq7OlwWgLwGa==\n-----END PRIVATE KEY-----\n",
            "client_email": "cloudvolumes-admin-sa@my-gcp-project.iam.gserviceaccount.com",
            "client_id": "123456789012345678901",
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token",
            "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
            "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/cloudvolumes-admin-sa%40my-gcp-project.iam.gserviceaccount.com"
        },
        "proxyURL": "http://proxy-server-hostname/",
        "nfsMountOptions": "nfsvers=3,proto=tcp,timeo=600",
        "limitVolumeSize": "10Ti",
        "serviceLevel": "premium",
        "defaults": {
            "snapshotDir": "true",
            "snapshotReserve": "5",
            "exportRule": "10.0.0.0/24,10.0.1.0/24,10.0.2.100",
            "size": "5Ti"
        }
    }

.. _gcp-virtual-storage-pool:

**Example 4 - Backend and storage class configuration for gcp-cvs driver with virtual storage pools**

This example shows the backend definition file configured with :ref:`Virtual Storage Pools <Virtual Storage Pools>`
along with StorageClasses that refer back to them.

In the sample backend definition file shown below, specific defaults are set for all storage pools, which set the
``snapshotReserve`` at 5% and the ``exportRule`` to 0.0.0.0/0. The virtual storage pools are defined in the
``storage`` section. In this example, each individual storage pool sets its own ``serviceLevel``, and some pools
overwrite the default values set above.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "gcp-cvs",
        "projectNumber": "012345678901",
        "apiRegion": "us-west2",
        "apiKey": {
            "type": "service_account",
            "project_id": "my-gcp-project",
            "private_key_id": "1234567890123456789012345678901234567890",
            "private_key": "-----BEGIN PRIVATE KEY-----\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nznHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE4jK3bl/qp8B4Kws8zX5ojY9m\nXsYg6gyxy4zq7OlwWgLwGa==\n-----END PRIVATE KEY-----\n",
            "client_email": "cloudvolumes-admin-sa@my-gcp-project.iam.gserviceaccount.com",
            "client_id": "123456789012345678901",
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token",
            "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
            "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/cloudvolumes-admin-sa%40my-gcp-project.iam.gserviceaccount.com"
        },
        "nfsMountOptions": "nfsvers=3,proto=tcp,timeo=600",

        "defaults": {
            "snapshotReserve": "5",
            "exportRule": "0.0.0.0/0"
        },

        "labels": {
            "cloud": "gcp"
        },
        "region": "us-west2",

        "storage": [
            {
                "labels": {
                    "performance": "extreme",
                    "protection": "extra"
                },
                "serviceLevel": "extreme",
                "defaults": {
                    "snapshotDir": "true",
                    "snapshotReserve": "10",
                    "exportRule": "10.0.0.0/24"
                }
            },
            {
                "labels": {
                    "performance": "extreme",
                    "protection": "standard"
                },
                "serviceLevel": "extreme"
            },
            {
                "labels": {
                    "performance": "premium",
                    "protection": "extra"
                },
                "serviceLevel": "premium",
                "defaults": {
                    "snapshotDir": "true",
                    "snapshotReserve": "10"
                }
            },

            {
                "labels": {
                    "performance": "premium",
                    "protection": "standard"
                },
                "serviceLevel": "premium"
            },

            {
                "labels": {
                    "performance": "standard"
                },
                "serviceLevel": "standard"
            }
        ]
    }

The following StorageClass definitions refer to the above Virtual Storage Pools. Using the ``parameters.selector``
field, each StorageClass calls out which virtual pool(s) may be used to host a volume. The volume will have the
aspects defined in the chosen virtual pool.

The first StorageClass (``cvs-extreme-extra-protection``) will map to the first Virtual Storage Pool. This is the
only pool offering extreme performance with a snapshot reserve of 10%. The last StorageClass (``cvs-extra-protection``)
calls out any storage pool which provides a snapshot reserve of 10%. Trident will decide which Virtual Storage Pool is
selected and will ensure the snapshot reserve requirement is met.

.. code-block:: yaml

    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-extreme-extra-protection
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=extreme; protection=extra"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-extreme-standard-protection
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=premium; protection=standard"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-premium-extra-protection
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=premium; protection=extra"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-premium
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=premium; protection=standard"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-standard
    provisioner: netapp.io/trident
    parameters:
      selector: "performance=standard"
    allowVolumeExpansion: true
    ---
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      name: cvs-extra-protection
    provisioner: netapp.io/trident
    parameters:
      selector: "protection=extra"
    allowVolumeExpansion: true

.. _GCP account configured with NetApp CVS: https://cloud.netapp.com/cloud-volumes-service-for-gcp?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
