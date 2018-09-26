.. _deploying-in-kubernetes:

Deploying
^^^^^^^^^

This guide will take you through the process of deploying Trident and
provisioning your first volume automatically.

Before you begin
================

If you have not already familiarized yourself with the
:ref:`basic concepts <What is Trident?>`, now is a great time to do that. Go
ahead, we'll be here when you get back.

To deploy Trident you need:

.. sidebar:: Need Kubernetes?

  If you do not already have a Kubernetes cluster, you can easily create one for
  demonstration purposes using our
  :ref:`simple Kubernetes install guide <Simple Kubernetes install>`.

* Full privileges to a
  :ref:`supported Kubernetes cluster <Supported frontends (orchestrators)>`
* Access to a
  :ref:`supported NetApp storage system <Supported backends (storage)>`
* :ref:`Volume mount capability <Worker preparation>` from all of the
  Kubernetes worker nodes
* A Linux host with ``kubectl`` (or ``oc``, if you're using OpenShift) installed
  and configured to manage the Kubernetes cluster you want to use
* If you are using Kubernetes with Docker EE 2.0, `follow their steps
  to enable CLI and REST access <https://docs.docker.com/ee/ucp/user-access/cli/>`_;
  the **AUTHTOKEN** must be provided later in Trident's install and uninstall
  commands.

Got all that? Great! Let's get started.

1: Qualify your Kubernetes cluster
==================================

You made sure that you have everything in hand from the
:ref:`previous section <Before you begin>`, right? Right.

The first thing you need to do is log into the Linux host and verify that it is
managing a *working*,
:ref:`supported Kubernetes cluster <Supported frontends (orchestrators)>` that
you have the necessary privileges to.

.. note::
  With OpenShift, you will use ``oc`` instead of ``kubectl`` in all of the
  examples that follow, and you need to login as **system:admin** first by
  running ``oc login -u system:admin``.

.. code-block:: bash

  # Are you running a supported Kubernetes server version?
  kubectl version

  # Are you a Kubernetes cluster administrator?
  kubectl auth can-i '*' '*' --all-namespaces

  # Can you launch a pod that uses an image from Docker Hub and can reach your
  # storage system over the pod network?
  kubectl run -i --tty ping --image=busybox --restart=Never --rm -- \
    ping <management IP>

2: Download & extract the installer
===================================

Download the latest version of the `Trident installer bundle`_ from the
*Downloads* section and extract it.

For example, if the latest version is 18.07.1:

.. code-block:: bash

   wget https://github.com/NetApp/trident/releases/download/v18.07.1/trident-installer-18.07.1.tar.gz
   tar -xf trident-installer-18.07.1.tar.gz
   cd trident-installer

.. _Trident installer bundle: https://github.com/NetApp/trident/releases/latest

3: Configure the installer
==========================

.. sidebar:: Why does Trident need an installer?

  We have an interesting chicken/egg problem: how to make it easy to get a
  persistent volume to store Trident's own metadata when Trident itself isn't
  running yet. The installer handles that for you!

Configure a *temporary* storage backend that the Trident installer will use
*once* to provision a volume to store its own metadata.

You do this by placing a ``backend.json`` file in the installer's ``setup``
directory. Sample configuration files for different backend types can be
found in the ``sample-input`` directory.

Visit the :ref:`backend configuration <Backend configuration>` section of
this guide for more details about how to craft the configuration file for
your backend type.

.. note::

  Many of the backends require some
  :ref:`basic preparation <Backend configuration>`, so make sure that's been
  done before you try to use it. Also, we don't recommend an
  ontap-nas-economy backend or ontap-nas-flexgroup backend for this step as
  volumes of these types have specialized and limited capabilities relative to
  the volumes provisioned on other types of backends.

.. code-block:: bash

  cp sample-input/<backend template>.json setup/backend.json
  # Fill out the template for your backend
  vi setup/backend.json

4: Install Trident
==================

First, let's verify that Trident can be installed:

.. code-block:: console

  # ./tridentctl install --dry-run -n trident
  INFO Starting storage driver.                backend=setup/backend.json
  INFO Storage driver loaded.                  driver=ontap-nas
  INFO Dry run completed, no problems found.

The ``--dry-run`` argument tells the installer to inspect the current
environment and checks that everything looks good for a Trident
installation, but it makes no changes to the environment and will *not*
install Trident.

The ``-n`` argument specifies the namespace (project in OpenShift) that
Trident will be installed into. We recommend installing Trident into its
own namespace to isolate it from other applications.

.. note::
  When using Kubernetes with Docker EE 2.0, you must also provide
  ``--ucp-host`` and ``--ucp-bearer-token`` for the install and uninstall commands::

      UCP_HOST="1.2.3.4"
      EE_USER="admin"
      EE_PASS="password"
      AUTHTOKEN=$(curl -sk -d "{\"username\":\"${EE_USER}\",\"password\":\"${EE_PASS}\"}" https://${UCP_HOST}/auth/login | jq -r .auth_token)
      # ./tridentctl install --dry-run -n trident --ucp-bearer-token="${AUTHTOKEN}" --ucp-host="${UCP_HOST}"

Provided that everything was configured correctly, you can now run the
Trident installer and it should be running in a few minutes:

.. code-block:: console

  #./tridentctl install -n trident
  INFO Starting storage driver.                backend=setup/backend.json
  INFO Storage driver loaded.                  driver=ontap-nas
  INFO Starting Trident installation.          namespace=trident
  INFO Created service account.
  INFO Created cluster role.
  INFO Created cluster role binding.
  INFO Created PVC.
  INFO Created PV.                             pv=trident
  INFO Waiting for PVC to be bound.            pvc=trident
  INFO Created Trident deployment.
  INFO Waiting for Trident pod to start.
  INFO Trident pod started.                    namespace=trident pod=trident-7d5d659bd7-tzth6
  INFO Trident installation succeeded.

It will look like this when the installer is complete:

.. code-block:: console

  # kubectl get pod -n trident
  NAME                       READY     STATUS    RESTARTS   AGE
  trident-7d5d659bd7-tzth6   2/2       Running   1          14s

  # ./tridentctl -n trident version
  +----------------+----------------+
  | SERVER VERSION | CLIENT VERSION |
  +----------------+----------------+
  | 18.07.1        | 18.07.1        |
  +----------------+----------------+

If that's what you see, you're done with this step, but **Trident is not
yet fully configured.** Go ahead and continue to the next step.

However, if the installer does not complete successfully or you don't see
a **Running** ``trident-<generated id>``, then Trident had a problem and the platform was *not*
installed.

To help figure out what went wrong, you could run the installer again using the ``-d`` argument,
which will turn on debug mode and help you understand what the problem is:

.. code-block:: console

  # ./tridentctl install -n trident -d

After addressing the problem, you can clean up the installation and go back to
the beginning of this step by first running:

.. code-block:: console

  # ./tridentctl uninstall -n trident
  INFO Deleted Trident deployment.
  INFO Deleted cluster role binding.
  INFO Deleted cluster role.
  INFO Deleted service account.
  INFO Removed Trident user from security context constraint.
  INFO Trident uninstallation succeeded.

If you continue to have trouble, visit the
:ref:`troubleshooting guide <Troubleshooting>` for more advice.

Customized Installation
-----------------------

Trident's installer allows you to customize attributes such as PV or PVC default names,
by using the installer's ``--pv`` or ``--pvc`` parameters. You can also specify a
storage volume name and size by using ``--volume-name`` and ``--volume-size``. If you have
copied the Trident images to a private repository, you can specify the image names by using
``--trident-image`` and ``--etcd-image``.

Users can also customize Trident's deployment files. Using the ``--generate-custom-yaml``
parameter will create the following YAML files in the installer's ``setup`` directory:

- trident-clusterrolebinding.yaml
- trident-deployment.yaml
- trident-pvc.yaml
- trident-clusterrole.yaml
- trident-namespace.yaml
- trident-serviceaccount.yaml

Once you have generated these files, you can modify them according to your needs and
then use the ``--use-custom-yaml`` to install a customized version of Trident.

.. code-block:: console
  # ./tridentctl install -n trident --use-custom-yaml --volume-name my_volume

5: Add your first backend
=========================

You already created a *temporary* :ref:`backend <Backend configuration>` in
step 3 to provision a volume for that Trident uses for its own metadata.

The installer does not assume that you want to use that backend configuration
for the rest of the volumes that Trident provisions. So Trident forgot about it.

Create a storage backend configuration that Trident will provision volumes
from. This can be the same backend configuration that you used in step 3, or
something completely different. It's up to you.

.. code-block:: bash

    ./tridentctl -n trident create backend -f setup/backend.json
    +-----------------------+----------------+--------+---------+
    |         NAME          | STORAGE DRIVER | ONLINE | VOLUMES |
    +-----------------------+----------------+--------+---------+
    | ontapnas_10.0.0.1     | ontap-nas      | true   |       0 |
    +-----------------------+----------------+--------+---------+

If the creation fails, something was wrong with the backend configuration. You
can view the logs to determine the cause by running:

.. code-block:: console

      ./tridentctl -n trident logs

After addressing the problem, simply go back to the beginning of this step
and try again. If you continue to have trouble, visit the
:ref:`troubleshooting guide <Troubleshooting>` for more advice on how to
determine what went wrong.

6: Add your first storage class
===============================

Kubernetes users provision volumes using persistent volume claims (PVCs) that
specify a `storage class`_ by name. The details are hidden from users, but a
storage class identifies the provisioner that will be used for that class (in
this case, Trident) and what that class means to the provisioner.

.. sidebar:: Basic too basic?

    This is just a basic storage class to get you started. There's an art to
    :ref:`crafting differentiated storage classes <Designing a storage class>`
    that you should explore further when you're looking at building them for
    production.

Create a storage class Kubernetes users will specify when they want a volume.
The configuration of the class needs to model the backend that you created
in the previous step so that Trident will use it to provision new volumes.

The simplest storage class to start with is one based on the
``sample-input/storage-class-basic.yaml.templ`` file that comes with the
installer, replacing ``__BACKEND_TYPE__`` with the storage driver name.

.. code-block:: bash

    ./tridentctl -n trident get backend
    +-----------------------+----------------+--------+---------+
    |         NAME          | STORAGE DRIVER | ONLINE | VOLUMES |
    +-----------------------+----------------+--------+---------+
    | ontapnas_10.0.0.1     | ontap-nas      | true   |       0 |
    +-----------------------+----------------+--------+---------+

    cp sample-input/storage-class-basic.yaml.templ sample-input/storage-class-basic.yaml
    # Modify __BACKEND_TYPE__ with the storage driver field above (e.g., ontap-nas)
    vi sample-input/storage-class-basic.yaml

This is a Kubernetes object, so you will use ``kubectl`` to create it in
Kubernetes.

.. code-block:: bash

    kubectl create -f sample-input/storage-class-basic.yaml

You should now see a **basic** storage class in both Kubernetes and Trident,
and Trident should have discovered the pools on the backend.

.. code-block:: bash

    kubectl get sc basic
    NAME      PROVISIONER
    basic     netapp.io/trident

    ./tridentctl -n trident get storageclass basic -o json
    {
      "items": [
        {
          "Config": {
            "version": "1",
            "name": "basic",
            "attributes": {
              "backendType": "ontap-nas"
            }
          },
          "storage": {
            "ontapnas_10.0.0.1": [
              "aggr1",
              "aggr2",
              "aggr3",
              "aggr4"
            ]
          }
        }
      ]
    }

.. _storage class: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#storageclasses

7: Provision your first volume
==============================

Now you're ready to dynamically provision your first volume. How exciting! This
is done by creating a Kubernetes `persistent volume claim`_ (PVC) object, and
this is exactly how your users will do it too.

.. _persistent volume claim: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims

Create a persistent volume claim (PVC) for a volume that uses the storage
class that you just created.

See ``sample-input/pvc-basic.yaml`` for an example. Make sure the storage
class name matches the one that you created in 6.

.. code-block:: bash

    kubectl create -f sample-input/pvc-basic.yaml
    # The '-aw' argument lets you watch the pvc get provisioned
    kubectl get pvc -aw
    NAME      STATUS    VOLUME    CAPACITY   ACCESS MODES   STORAGECLASS   AGE
    basic     Pending                                       basic          1s
    basic     Pending   default-basic-6cb59   0                   basic     5s
    basic     Bound     default-basic-6cb59   1Gi       RWO       basic     5s

8: Mount the volume in a pod
============================

Now that you have a volume, let's mount it. We'll launch an nginx pod that
mounts the PV under ``/usr/share/nginx/html``.

.. code-block:: bash

  cat << EOF > task-pv-pod.yaml
  kind: Pod
  apiVersion: v1
  metadata:
    name: task-pv-pod
  spec:
    volumes:
      - name: task-pv-storage
        persistentVolumeClaim:
         claimName: basic
    containers:
      - name: task-pv-container
        image: nginx
        ports:
          - containerPort: 80
            name: "http-server"
        volumeMounts:
          - mountPath: "/usr/share/nginx/html"
            name: task-pv-storage
  EOF
  kubectl create -f task-pv-pod.yaml

.. code-block:: bash

  # Wait for the pod to start
  kubectl get pod -aw

  # Verify that the volume is mounted on /usr/share/nginx/html
  kubectl exec -it task-pv-pod -- df -h /usr/share/nginx/html
  Filesystem                                      Size  Used Avail Use% Mounted on
  10.0.0.1:/trident_demo_default_basic_6cb59  973M  192K  973M   1% /usr/share/nginx/html

  # Delete the pod
  kubectl delete pod task-pv-pod

At this point the pod (application) no longer exists but the volume is still
there. You could use it from another pod if you wanted to.

To delete the volume, simply delete the claim:

.. code-block:: bash

  kubectl delete pvc basic

**Check you out! You did it!** Now you're dynamically provisioning
Kubernetes volumes like a boss.

..
  Where do you go from here? you can do things like:

  * Configure additional backends
  * Model additional storage classes
  * Review considerations for moving this into production
