################################
Azure NetApp Files Configuration
################################

.. note::
  The Azure NetApp Files service does not support volumes less than 100 GB in size. To make it easier to deploy
  applications, Trident automatically creates 100 GB volumes if a smaller volume is requested.

Preparation
-----------

To configure and use an `Azure NetApp Files`_ backend, you will need:

.. _Azure NetApp Files: https://azure.microsoft.com/en-us/services/netapp/

* ``subscriptionID`` from an Azure subscription with Azure NetApp Files enabled
* ``tenantID``, ``clientID``, and ``clientSecret`` from an `App Registration`_ in Azure Active Directory with
  sufficient permissions to the Azure NetApp Files service
* Azure ``location`` that contains at least one `delegated subnet`_

.. _App Registration: https://docs.microsoft.com/en-us/azure/active-directory/develop/howto-create-service-principal-portal
.. _delegated subnet: https://docs.microsoft.com/en-us/azure/azure-netapp-files/azure-netapp-files-delegate-subnet

If you're using Azure NetApp Files for the first time or in a new location, some initial configuration is required that
the `quickstart guide`_ will walk you through.

.. _quickstart guide: https://docs.microsoft.com/en-us/azure/azure-netapp-files/azure-netapp-files-quickstart-set-up-account-create-volumes

Backend configuration options
-----------------------------

================== =============================================================== ================================================
Parameter          Description                                                     Default
================== =============================================================== ================================================
version            Always 1
storageDriverName  "azure-netapp-files"
backendName        Custom name for the storage backend                             Driver name + "_" + random characters
subscriptionID     The subscription ID from your Azure subscription
tenantID           The tenant ID from an App Registration
clientID           The client ID from an App Registration
clientSecret       The client secret from an App Registration
serviceLevel       One of "Standard", "Premium" or "Ultra"                         "" (random)
location           Name of the Azure location new volumes will be created in       "" (random)
virtualNetwork     Name of a virtual network with a delegated subnet               "" (random)
subnet             Name of a subnet delegated to ``Microsoft.Netapp/volumes``      "" (random)
nfsMountOptions    Fine-grained control of NFS mount options                       "-o nfsvers=3"
limitVolumeSize    Fail provisioning if requested volume size is above this value  "" (not enforced by default)
================== =============================================================== ================================================

You can control how each volume is provisioned by default using these options in a special section of the configuration.
For an example, see the configuration examples below.

================ =============================================================== ================================================
Parameter        Description                                                     Default
================ =============================================================== ================================================
exportRule       The export rule(s) for new volumes                              "0.0.0.0/0"
size             The default size of new volumes                                 "100G"
================ =============================================================== ================================================

The ``exportRule`` value must be a comma-separated list of any combination of IPv4 addresses or IPv4 subnets in CIDR
notation.

Example Configurations
----------------------

**Example 1 - Minimal backend configuration for azure-netapp-files**

This is the absolute minimum backend configuration. With this Trident will discover all of your NetApp accounts, capacity pools, and subnets delegated to ANF in every location worldwide, and place new volumes on one of them randomly.

This configuration is useful when you're just getting started with ANF and trying things out, but in practice you're going to want to provide additional scoping for the volumes you provision in order to make sure that they have the characteristics you want and end up on a network that's close to the compute that's using it. See the subsequent examples for more details.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "azure-netapp-files",
        "subscriptionID": "9f87c765-4774-fake-ae98-a721add45451",
        "tenantID": "68e4f836-edc1-fake-bff9-b2d865ee56cf",
        "clientID": "dd043f63-bf8e-fake-8076-8de91e5713aa",
        "clientSecret": "SECRET"
    }

**Example 2 - Single location and specific service level for azure-netapp-files**

This backend configuration will place volumes in Azure's "eastus" location in a "Premium" capacity pool. Trident automatically discovers all of the subnets delegated to ANF in that location and will place a new volume on one of them randomly.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "azure-netapp-files",
        "subscriptionID": "9f87c765-4774-fake-ae98-a721add45451",
        "tenantID": "68e4f836-edc1-fake-bff9-b2d865ee56cf",
        "clientID": "dd043f63-bf8e-fake-8076-8de91e5713aa",
        "clientSecret": "SECRET",
        "location": "eastus",
        "serviceLevel": "Premium"
    }


**Example 3 - Advanced configuration for azure-netapp-files**

This backend configuration further reduces the scope of volume placement to a single subnet, and also modifies some volume provisioning defaults.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "azure-netapp-files",
        "subscriptionID": "9f87c765-4774-fake-ae98-a721add45451",
        "tenantID": "68e4f836-edc1-fake-bff9-b2d865ee56cf",
        "clientID": "dd043f63-bf8e-fake-8076-8de91e5713aa",
        "clientSecret": "SECRET",
        "location": "eastus",
        "serviceLevel": "Premium",
        "virtualNetwork": "my-virtual-network",
        "subnet": "my-subnet",
        "nfsMountOptions": "vers=3,proto=tcp,timeo=600",
        "limitVolumeSize": "500Gi",
        "defaults": {
            "exportRule": "10.0.0.0/24,10.0.1.0/24,10.0.2.100",
            "size": "200Gi"
        }
    }


**Example 4 - Virtual storage pools with azure-netapp-files**

This backend configuration defines multiple :ref:`pools of storage <Virtual Storage Pools>` in a single file. This is useful when you have multiple capacity pools supporting different service levels and you want to create storage classes in Kubernetes that represent those.

This is just scratching the surface of the power of virtual storage pools and their labels.

.. code-block:: json

    {
        "version": 1,
        "storageDriverName": "azure-netapp-files",
        "subscriptionID": "9f87c765-4774-fake-ae98-a721add45451",
        "tenantID": "68e4f836-edc1-fake-bff9-b2d865ee56cf",
        "clientID": "dd043f63-bf8e-fake-8076-8de91e5713aa",
        "clientSecret": "SECRET",
        "nfsMountOptions": "vers=3,proto=tcp,timeo=600",
        "labels": {
            "cloud": "azure"
        },
        "location": "eastus",

        "storage": [
            {
                "labels": {
                    "performance": "gold"
                },
                "serviceLevel": "Ultra"
            },
            {
                "labels": {
                    "performance": "silver"
                },
                "serviceLevel": "Premium"
            },
            {
                "labels": {
                    "performance": "bronze"
                },
                "serviceLevel": "Standard",
            }
        ]
    }
