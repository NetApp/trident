Cloud Volumes Service (CVS) on GCP Configuration
================================================

.. note::

   The NetApp Cloud Volumes Service for GCP does not support CVS-Performance volumes less than 100 GiB in size, or CVS
   volumes less than 1 TiB in size. To make it easier to deploy applications, Trident automatically creates volumes of
   the minimum size if a too-small volume is requested. Future releases of the Cloud Volumes Service may remove this
   restriction.

In addition to the global configuration values above, when using CVS on GCP, these options are available.

+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| Option                | Description                                                              | Example                                      |
+=======================+==========================================================================+==============================================+
| ``projectNumber``     | GCP project number (required)                                            | "123456789012"                               |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``hostProjectNumber`` | GCP shared VPC host project number (required if using a shared VPC)      | "098765432109"                               |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``apiRegion``         | CVS account region (required)                                            | "us-west2"                                   |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``apiKey``            | API key for GCP service account with CVS admin role (required)           | (contents of private key file)               |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``proxyURL``          | Proxy URL if proxy server required to connect to CVS account             | "http://proxy-server-hostname/"              |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``nfsMountOptions``   | NFS mount options; defaults to "-o nfsvers=3"                            | "nfsvers=3,proto=tcp,timeo=600"              |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``network``           | GCP network used for CVS volumes, defaults to "default"                  | "default"                                    |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``serviceLevel``      | Performance level (standard, premium, extreme), defaults to "standard"   | "premium"                                    |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+

The required value ``projectNumber`` may be found in the GCP web portal's Home screen.  The ``apiRegion`` is the
GCP region where this backend will provision volumes. The ``apiKey`` is the JSON-formatted contents of a GCP
service account's private key file (copied verbatim into the backend config file).  The service account must have
the ``netappcloudvolumes.admin`` role.

If using a shared VPC network, both ``projectNumber`` and ``hostProjectNumber`` must be specified.  In that case,
``projectNumber`` is the service project, and ``hostProjectNumber`` is the host project.

The proxyURL config option must be used if a proxy server is needed to communicate with AWS. The proxy server may either
be an HTTP proxy or an HTTPS proxy. In case of an HTTPS proxy, certificate validation is skipped to allow the usage of
self-signed certificates in the proxy server. Proxy servers with authentication enabled are not supported.

Also, when using CVS on GCP, these default volume option settings are available.

+-----------------------+--------------------------------------------------------------------------+--------------------------+
| Defaults Option       | Description                                                              | Example                  |
+=======================+==========================================================================+==========================+
| ``exportRule``        | NFS access list (addresses and/or CIDR subnets), defaults to "0.0.0.0/0" | "10.0.1.0/24,10.0.2.100" |
+-----------------------+--------------------------------------------------------------------------+--------------------------+
| ``snapshotDir``       | Controls visibility of the .snapshot directory                           | "false"                  |
+-----------------------+--------------------------------------------------------------------------+--------------------------+
| ``snapshotReserve``   | Snapshot reserve percentage, default is "" to accept CVS default of 0    | "10"                     |
+-----------------------+--------------------------------------------------------------------------+--------------------------+
| ``size``              | Volume size, defaults to "1TiB"                                          | "10T"                    |
+-----------------------+--------------------------------------------------------------------------+--------------------------+

Example CVS on GCP Config File
------------------------------

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
        "proxyURL": "http://proxy-server-hostname/"
    }
