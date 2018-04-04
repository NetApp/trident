Trident for Docker
------------------

Trident for Docker provides direct integration with the Docker ecosystem for NetApp's ONTAP, SolidFire, and E-Series
storage platforms. It supports the provisioning and management of storage resources from the storage platform to Docker
hosts, with a robust framework for adding additional platforms in the future.

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
