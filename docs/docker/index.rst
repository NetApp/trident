Trident for Docker
------------------

Trident for Docker provides direct integration with the Docker ecosystem for NetApp's `ONTAP`_ and `Element`_
storage platforms, plus our
`Azure NetApp Files`_ service in Azure,
`Cloud Volumes Service for AWS`_, and
`Cloud Volumes Service for GCP`_.

 It supports the provisioning and management of storage
resources from the storage platform to Docker hosts, with a robust framework for adding additional platforms in the
future.

Multiple instances of Trident can run concurrently on the same host. This allows simultaneous connections to multiple
storage systems and storage types, with the ablity to customize the storage used for the Docker volume(s).

.. toctree::
    :maxdepth: 2
    :caption: Contents:

    deploying
    install/index
    use/index
    known-issues
    troubleshooting


.. _Azure NetApp Files: https://cloud.netapp.com/azure-netapp-files
.. _ONTAP: https://www.netapp.com/us/products/data-management-software/ontap.aspx
.. _Element: https://www.netapp.com/us/products/data-management-software/element-os.aspx
.. _Cloud Volumes Service for AWS: https://cloud.netapp.com/cloud-volumes-service-for-aws?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident
.. _Cloud Volumes Service for GCP: https://cloud.netapp.com/cloud-volumes-service-for-gcp?utm_source=NetAppTrident_ReadTheDocs&utm_campaign=Trident