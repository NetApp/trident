.. _ontap-san-user-permissions:

################
User permissions
################

Trident expects to be run as either an ONTAP or SVM administrator, typically
using the ``admin`` cluster user or a ``vsadmin`` SVM user, or a user with a
different name that has the same role.

.. note::
        If you use the "limitAggregateUsage" option, cluster admin permissions are required.

While it is possible to create a more restrictive role within ONTAP that a
Trident driver can use, we don't recommend it. Most new releases of Trident
will call additional APIs that would have to be accounted for, making upgrades
difficult and error-prone.
