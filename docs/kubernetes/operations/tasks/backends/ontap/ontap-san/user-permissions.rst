.. _ontap-san-user-permissions:

################
User permissions
################

Trident expects to be run as either an ONTAP or SVM administrator, typically
using the ``admin`` cluster user or a ``vsadmin`` SVM user, or a user with a
different name that has the same role. For Amazon FSx for ONTAP deployments, Trident expects to be run as either an ONTAP or SVM administrator, using the cluster ``fsxadmin`` user or a ``vsadmin`` SVM user, or a user with a different name that has the same role. The ``fsxadmin`` user is a limited replacement for the cluster admin user.

.. note::
        If you use the ``limitAggregateUsage`` parameter, cluster admin permissions are required. When using Amazon FSx for ONTAP with Trident, the ``limitAggregateUsage`` parameter will not work with the ``vsadmin`` and ``fsxadmin`` user accounts. The configuration operation will fail if you specify this parameter.

While it is possible to create a more restrictive role within ONTAP that a
Trident driver can use, we don't recommend it. Most new releases of Trident
will call additional APIs that would have to be accounted for, making upgrades
difficult and error-prone.
