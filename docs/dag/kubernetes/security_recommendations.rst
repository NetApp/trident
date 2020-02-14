.. _security_recommendations:

*************************
Security Recommendations
*************************

Run Trident in its own namespace
---------------------------------

It is important to prevent applications, application admins, users, and
management applications from accessing Trident object definitions or the
pods to ensure reliable storage and block potential malicious activity.
To separate out the other applications and users from Trident, always
install Trident in its own Kubernetes namespace. In our
:ref:`Installing Trident docs <3: Install Trident>` we call this namespace
`trident`. Putting Trident in its own namespace assures that only the
Kubernetes administrative personnel have access to the Trident pod and
the artifacts (such as backend and CHAP secrets if applicable) stored
in the namespaced CRD objects. Allow only administrators access to the
Trident namespace and thus access to `tridentctl` application.

CHAP authentication
-------------------

.. note::

   Trident will only use CHAP when installed as a CSI Provisioner. 

NetApp recommends deploying bi-directional CHAP to ensure authentication
between a host and the HCI/SolidFire backend. Trident uses a secret
object that includes two CHAP passwords per tenant. When Trident is installed
as a CSI Provisioner, it manages the CHAP secrets and stores them in a
``tridentvolume`` CR object for the respective PV. When a PV is created,
CSI Trident uses the CHAP secrets to initiate an iSCSI session and communicate with
the HCI/SolidFire system over CHAP.

In the non-CSI frontend, the attachment of volumes as devices on the worker
nodes is handled by Kubernetes. Upon volume creation time, Trident makes an API call to
the HCI/SolidFire system to retrieve the secrets if the secret for that tenant
doesn’t already exist. Trident then passes the secrets on to Kubernetes. The kubelet
located on each node accesses the secrets via the Kubernetes API and uses them to
run/enable CHAP between each node accessing the volume and the HCI/SolidFire system
where the volumes are located.
