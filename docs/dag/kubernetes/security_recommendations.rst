.. _security_recommendations:

*************************
Security Recommendations
*************************

Run Trident in its own namespace
---------------------------------

It is important to prevent applications, application admins, users, and management applications from accessing Trident object definitions or the pods to ensure reliable storage and block potential malicious activity. To separate out the other applications and users from Trident, always install Trident in its own Kubernetes namespace. In our :ref:`Installing Trident docs <Install Trident>` we call this namespace `trident`. Putting Trident in its own namespace assures that only the Kubernetes administrative personnel have access to the Trident pod and the artifacts (such as backend and CHAP secrets if applicable) stored in Trident's etcd datastore. Allow only administrators access to the trident namespace and thus access to `tridentctl` application.

CHAP authentication
-------------------

NetApp recommends deploying bi-directional CHAP to ensure authentication between a host and the HCI/SolidFire backend. Trident uses a secret object that includes two CHAP passwords per tenant. Kubernetes manages the mapping of Kubernetes tenant to SF tenant. Upon volume creation time, Trident makes an API call to the HCI/SolidFire system to retrieve the secrets if the secret for that tenant doesn’t already exist. Trident then passes the secrets on to Kubernetes. The kubelet located on each node accesses the secrets via the Kubernetes API and uses them to run/enable CHAP between each node accessing the volume and the HCI/SolidFire system where the volumes are located.

Since Trident v18.10, the ``solidfire-san`` driver defaults to use CHAP if the Kubernetes version is >= 1.7 and a Trident access group doesn't exist. Setting `AccessGroup` or `UseCHAP` in the backend configuration file overrides this behavior. CHAP is guaranteed by setting ``UseCHAP`` to ``true`` in the backend.json file.

