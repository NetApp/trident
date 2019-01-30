.. _security_recommendations:

*************************
Security Recommendations
*************************

Run Trident in its own namespace
---------------------------------

It is important to prevent applications, application admins, users, and management applications from accessing Trident object definitions or the pods to ensure reliable storage and block potential malicious activity. To separate out the other applications and users from Trident, always install Trident in its own Kubernetes namespace. In our :ref:`Installing Trident docs <Install Trident>` we call this namespace `trident`. Putting Trident in its own namespace assures that only the Kubernetes administrative personnel have access to the Trident pod and the artifacts (such as backend and CHAP secrets if applicable) stored in Trident's etcd datastore. Allow only administrators access to the trident namespace and thus access to `tridentctl` application.

etcd container
--------------

Trident's state is stored in etcd. Etcd contains details regarding the backends, storage classes, and volumes the Trident provisioner creates. Trident's default install method installs etcd as a container in the Trident pod. For security reasons, the etcd container located within the Trident pod is not accessible via its REST interface outside the pod, and only the trident container can access the etcd container. If you choose to use an :ref:`external etcd <Using an external etcd cluster>`, authenticate Trident with the etcd cluster using certificates. This also ensures all the communication between the Trident and etcd is encrypted.

CHAP authentication
-------------------

NetApp recommends deploying bi-directional CHAP to ensure authentication between a host and the SolidFire backend. Trident uses a secret object that includes two CHAP passwords per SolidFire tenant. Kubernetes manages mapping of Kubernetes tenant to SF tenant. Upon volume creation time, Trident makes an API call to the SolidFire system to retrieve the secrets if the secret for that tenant doesn’t already exist. Trident then passes the secrets on to Kubernetes. The kubelet located on each node accesses the secrets via the Kubernetes API and uses them to run/enable CHAP between each node accessing the volume and the SolidFire system where the volumes are located.

Since Trident v18.10, SolidFire defaults to use CHAP if the Kubernetes version is >= 1.7 and a Trident access group doesn't exist. Setting `AccessGroup` or `UseCHAP` in the backend configuration file overrides this behavior. CHAP is guaranteed by setting ``UseCHAP`` to ``true`` in the backend.json file.


Storage backend credentials
---------------------------

Delete backend config files
^^^^^^^^^^^^^^^^^^^^^^^^^^^

Deleting the backend config files helps prevent unauthorized users from accessing backend usernames, passwords and other credentials. After the backends are created using the `tridentctl create backend` command, make sure that the backend config files are deleted from the node where Trident was installed. Keep a copy of the file in a secure location if the backend file will be updated in the future (e.g. to change passwords).. 

Encrypt Trident* etcd Volume
^^^^^^^^^^^^^^^^^^^^^^^^^^^

Since Trident stores all its backend credentials in its etcd datastore, it may be possible for unauthorized users to access this information either from etcdctl or directly from the hardware hosting the etcd volume. Therefore, as an additional security measure, enable encryption on the Trident volume.

The appropriate encryption license must be enabled on the backends to encrypt Tridents volume.  

**ONTAP Backend**


	Prior to Trident installation, edit the :ref:`temporary backend.json <Configure the installer>` file to include :ref:`encryption <Backend configuration options>`. When the volume is created for Tridents use, that volume will then be encrypted upon trident installation. 

Alternatively, encrypt the trident volume using the ONTAP CLI command ``volume encryption conversion start -vserver SVM_name -volume volume_name``. Verify the status of the conversion operation using the command  ``volume encryption conversion show``. Please note that you cannot use `volume encryption conversion start` ONTAP CLI command to start encryption on a SnapLock or FlexGroup volume. For more information on how setup NetApp Volume Encryption, refer to the `ONTAP NetApp Encryption Power Guide <https://library.netapp.com/ecm/ecm_download_file/ecmlp2572742>`_.

**Element Backend**


On the Solidfire backend, enable encryption for the cluster.
