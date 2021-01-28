.. _ontap-nas-preparation:

###########
Preparation
###########

For all ONTAP backends, Trident requires at least one
`aggregate assigned to the SVM`_.

.. _aggregate assigned to the SVM: https://library.netapp.com/ecmdocs/ECMP1368404/html/GUID-5255E7D8-F420-4BD3-AEFB-7EF65488C65C.html

Remember that you can also run more than one driver, and create storage
classes that point to one or the other. For example, you could configure a
*Gold* class that uses the ``ontap-nas`` driver and a *Bronze* class that
uses the ``ontap-nas-economy`` one.

All of your Kubernetes worker nodes must have the appropriate NFS tools
installed. See the :ref:`worker configuration guide <NFS>` for more details.

.. _ontap-nas-authentication:

Authentication
--------------

Trident offers two modes of authenticating an ONTAP backend.

-  **Credential-based**: The username and password to an ONTAP user with the
   required :ref:`permissions <ontap-nas-user-permissions>`. It is recommended
   to use a pre-defined security login role, such as ``admin`` or ``vsadmin`` to
   ensure maximum compatibility with ONTAP versions.
-  **Certificate-based**: Trident can also communicate with an ONTAP cluster using
   a certificate installed on the backend. Here, the backend definition must
   contain Base64-encoded values of the client certificate, key, and the trusted
   CA certificate if used (recommended).

Users can also choose to update existing backends, opting to move from
credential-based to certificate-based, and vice-versa. If **both credentials and
certificates are provided**, Trident will default to using certificates while
issuing a warning to remove the credentials from the backend definition.

Credential-based Authentication
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Trident requires the credentials to an SVM-scoped/Cluster-scoped admin to
communicate with the ONTAP backend. It is recommended to make use of standard,
pre-defined roles such as ``admin`` or ``vsadmin``. This ensures forward
compatibility with future ONTAP releases that may expose feature APIs to be used
by future Trident releases. A custom security login role can be created and used
with Trident, but is **not recommended**.

A sample backend definition will look like this:

.. code-block:: json

  {
    "version": 1,
    "backendName": "ExampleBackend",
    "storageDriverName": "ontap-nas",
    "managementLIF": "10.0.0.1",
    "dataLIF": "10.0.0.2",
    "svm": "svm_nfs",
    "username": "vsadmin",
    "password": "secret",
  }

Keep in mind that the backend definition is the only place the credentials are
stored in plaintext. Once the backend is created, **usernames/passwords are encoded
with Base64 and stored as Kubernetes secrets**. The creation/updation of a backend
is the only step that requires knowledge of the credentials. As such, it is an
admin-only operation, to be performed by the Kubernetes/Storage administrator.

Certificated-based Authentication
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Trident ``21.01`` introduces a new feature: authenticating ONTAP backends using
certificates. New and existing backends can use a certificate and communicate with
the ONTAP backend. Three parameters are required in the backend definition.

- ``clientCertificate``: Base64-encoded value of client certificate.
- ``clientPrivateKey``: Base64-encoded value of associated private key.
- ``trustedCACertificate``: Base64-encoded value of trusted CA certificate. If
  using a trusted CA, this parameter must be provided. This can be ignored if
  no trusted CA is used.

A typical workflow involves the following steps:

1. A client certificate and key are generated. When generating, **CN (Common Name) is
   set to the ONTAP user to authenticate as**.

  .. code-block:: bash

    #Generate certificate used to authenticate the client
    openssl req -x509 -nodes -days 1095 -newkey rsa:2048 -keyout k8senv.key -out k8senv.pem -subj "/C=US/ST=NC/L=RTP/O=NetApp/CN=admin"

2. Add trusted CA certificate to the ONTAP cluster. This may be already handled by
   the storage administrator. Ignore if no trusted CA is used.

  .. code-block:: bash

    #Install trusted CA cert in ONTAP cluster.
    security certificate install -type server -cert-name <trusted-ca-cert-name> -vserver <vserver-name>
    ssl modify -vserver <vserver-name> -server-enabled true -client-enabled true -common-name <common-name> -serial <SN-from-trusted-CA-cert> -ca <cert-authority>

3. Install the client certificate and key (from step 1) on the ONTAP cluster.

  .. code-block:: bash

    #Install certificate generated from step 1 on ONTAP cluster
    security certificate install -type client-ca -cert-name <certificate-name> -vserver <vserver-name>
    security ssl modify -vserver <vserver-name> -client-enabled true

4. Confirm the ONTAP security login role supports ``cert`` authentication method.

   .. code-block:: bash

     #Add cert authentication method to ONTAP security login role
     security login create -user-or-group-name admin -application ontapi -authentication-method cert
     security login create -user-or-group-name admin -application http -authentication-method cert

5. Test authentication using certificate generated.

   .. code-block:: bash

     #Test access to ONTAP cluster using certificate. Replace <ONTAP Management LIF> and <vserver name> with Management LIF IP and SVM name.
     curl -X POST -Lk https://<ONTAP-Management-LIF>/servlets/netapp.servlets.admin.XMLrequest_filer --key k8senv.key --cert ~/k8senv.pem -d '<?xml version="1.0" encoding="UTF-8"?><netapp xmlns="http://www.netapp.com/filer/admin" version="1.21" vfiler="<vserver-name>"><vserver-get></vserver-get></netapp>'

6. Encode certificate, key and trusted CA certificate with Base64.

   .. code-block:: bash

     #Encode with Base64 and write each key to a file.
     base64 -w 0 k8senv.pem >> cert_base64
     base64 -w 0 k8senv.key >> key_base64
     base64 -w 0 trustedca.pem >> trustedca_base64

7. Create backend using the values obtained from step 6.

   .. code-block:: bash

     #Trident backend using cert-based auth
     $ cat cert-backend.json
     {
     "version": 1,
     "storageDriverName": "ontap-nas",
     "backendName": "NasBackend",
     "managementLIF": "1.2.3.4",
     "dataLIF": "1.2.3.8",
     "svm": "vserver_test",
     "clientCertificate": "Faaaakkkkeeee...Vaaalllluuuueeee",
     "clientPrivateKey": "LS0tFaKE...0VaLuES0tLS0K",
     "trustedCACertificate": "QNFinfO...SiqOyN",
     "storagePrefix": "myPrefix_"
     }

     #Create backend
     $ tridentctl create backend -f cert-backend.json -n trident
     +------------+----------------+--------------------------------------+--------+---------+
     |    NAME    | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
     +------------+----------------+--------------------------------------+--------+---------+
     | NasBackend | ontap-nas      | 98e19b74-aec7-4a3d-8dcf-128e5033b214 | online |       0 |
     +------------+----------------+--------------------------------------+--------+---------+

Updating Authentication Methods/Rotating Credentials
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Users can update an existing backend to make use of a different authentication
method or to rotate their credentials. This works both ways: backends that make
use of username/password can be updated to use certificates; backends that
utilize certificates can be updated to username/password based. To do this,
an updated backend.json file containing the required parameters must be used to
execute ``tridentctl backend update``.

.. note::

  When rotating passwords, the storage administrator must first update the
  password for the user on ONTAP. This is followed by a backend update. When
  rotating certificates, multiple certificates can be added to the user. The
  backend is then updated to use the new certificate, following which the old
  certificate can be deleted from the ONTAP cluster.

.. code-block:: bash

  #Update backend.json to include chosen auth method
  $ cat cert-backend-updated.json
  {
  "version": 1,
  "storageDriverName": "ontap-nas",
  "backendName": "NasBackend",
  "managementLIF": "1.2.3.4",
  "dataLIF": "1.2.3.8",
  "svm": "vserver_test",
  "username": "vsadmin",
  "password": "secret",
  "storagePrefix": "myPrefix_"
  }

  #Update backend with tridentctl
  $ tridentctl update backend NasBackend -f cert-backend-updated.json -n trident
  +------------+----------------+--------------------------------------+--------+---------+
  |    NAME    | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
  +------------+----------------+--------------------------------------+--------+---------+
  | NasBackend | ontap-nas      | 98e19b74-aec7-4a3d-8dcf-128e5033b214 | online |       9 |
  +------------+----------------+--------------------------------------+--------+---------+

Updating a backend does not disrupt access to volumes that have already been
created, nor impact volume connections made after. A successful backend update
indicates that Trident can communicate with the ONTAP backend and handle future
volume operations.

Export-policy Management
------------------------

Trident uses NFS `export policies`_ to control access to the volumes that it
provisions.

.. _export policies: https://library.netapp.com/ecmdocs/ECMP1196891/html/GUID-9A2B6C3E-C86A-4125-B778-6072A3A19657.html

Trident provides two options when working with export policies:

1. Trident can **dynamically manage the export policy itself**; in this mode of
   operation, the storage admin specifies a list of CIDR blocks that
   represent admissible IP addresses. Trident adds node IPs that fall in
   these ranges to the export policy automatically. Alternatively, when no
   CIDRs are specified, any global-scoped unicast IP found on the nodes will
   be added to the export policy.

2. Storage admins can create an export policy and add rules manually. Trident uses
   the ``default`` export policy unless a different export policy name is specified
   in the configuration.

With (1), Trident automates the management of export policies, creating an export
policy and taking care of additions and deletions of rules to the export policy based
on the worker nodes it runs on. As and when nodes are removed or added to the
Kubernetes cluster, Trident can be set up to permit access to the nodes, thus
providing a more robust way of managing access to the PVs it creates. Trident
will create one export policy per backend. **This feature requires CSI Trident**.
To learn more about this feature, continue to
:ref:`Dynamic Export Policy Management <dynamic-export-policy-ontap>`.

With (2), Trident does not create or otherwise manage export policies themselves.
The export policy must exist before the storage backend is added to Trident,
and it needs to be configured to allow access to every worker node in the
Kubernetes cluster. If the export policy is locked down to specific hosts,
it will need to be updated when new nodes are added to the cluster
and that access should be removed when nodes are removed as well.
