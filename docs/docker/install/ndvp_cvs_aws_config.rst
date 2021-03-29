Cloud Volumes Service (CVS) on AWS Configuration
================================================

.. note::

   The NetApp Cloud Volumes Service for AWS does not support volumes less than 100 GB in size. To
   make it easier to deploy applications, Trident automatically creates 100 GB volumes if a
   smaller volume is requested. Future releases of the Cloud Volumes Service may remove this restriction.

In addition to the global configuration values above, when using CVS on AWS, these options are available.  The
required values are all available in the CVS web user interface.

+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| Option                | Description                                                              | Example                                      |
+=======================+==========================================================================+==============================================+
| ``apiRegion``         | CVS account region (required)                                            | "us-east-1"                                  |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``apiURL``            | CVS account API URL (required)                                           | "https://cds-aws-bundles.netapp.com:8080/v1" |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``apiKey``            | CVS account API key (required)                                           |                                              |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``secretKey``         | CVS account secret key (required)                                        |                                              |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``proxyURL``          | Proxy URL if proxy server required to connect to CVS account             | "http://proxy-server-hostname/"              |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``nfsMountOptions``   | NFS mount options; defaults to "-o nfsvers=3"                            | "vers=3,proto=tcp,timeo=600"                 |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+
| ``serviceLevel``      | Performance level (standard, premium, extreme), defaults to "standard"   | "premium"                                    |
+-----------------------+--------------------------------------------------------------------------+----------------------------------------------+

The required values ``apiRegion``, ``apiURL``, ``apiKey``, and ``secretKey`` may be found in the CVS web portal in
Account settings / API access.

The proxyURL config option must be used if a proxy server is needed to communicate with AWS. The proxy server may either
be an HTTP proxy or an HTTPS proxy. In case of an HTTPS proxy, certificate validation is skipped to allow the usage of
self-signed certificates in the proxy server. Proxy servers with authentication enabled are not supported.

Also, when using CVS on AWS, these default volume option settings are available.

+-----------------------+--------------------------------------------------------------------------+--------------------------+
| Defaults Option       | Description                                                              | Example                  |
+=======================+==========================================================================+==========================+
| ``exportRule``        | NFS access list (addresses and/or CIDR subnets), defaults to "0.0.0.0/0" | "10.0.1.0/24,10.0.2.100" |
+-----------------------+--------------------------------------------------------------------------+--------------------------+
| ``snapshotDir``       | Controls visibility of the .snapshot directory                           | "false"                  |
+-----------------------+--------------------------------------------------------------------------+--------------------------+
| ``snapshotReserve``   | Snapshot reserve percentage, default is "" to accept CVS default of 0    | "10"                     |
+-----------------------+--------------------------------------------------------------------------+--------------------------+
| ``size``              | Volume size, defaults to "100GB"                                         | "500G"                   |
+-----------------------+--------------------------------------------------------------------------+--------------------------+

Example CVS on AWS Config File
------------------------------

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "aws-cvs",
        "apiRegion": "us-east-1",
        "apiURL": "https://cds-aws-bundles.netapp.com:8080/v1",
        "apiKey":    "znHczZsrrtHisIsAbOguSaPIKeyAZNchRAGzlzZE",
        "secretKey": "rR0rUmWXfNioN1KhtHisiSAnoTherboGuskey6pU",
        "region": "us-east-1",
        "proxyURL": "http://proxy-server-hostname/",
        "serviceLevel": "premium",
        "limitVolumeSize": "200Gi",
        "defaults": {
            "snapshotDir": "true",
            "snapshotReserve": "5",
            "exportRule": "10.0.0.0/24,10.0.1.0/24,10.0.2.100",
            "size": "100Gi"
        }
    }
