.. _deploying-with-tridentctl:

#########################
Deploying with tridentctl
#########################

Welcome to the Deployment Guide for installing Trident using
``tridentctl``! This page explains the various steps involved
in deploying Trident in your Kubernetes cluster using ``tridentctl``.

Before you begin
----------------

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
* :ref:`Volume mount capability <Preparing the worker node>` from all of the
  Kubernetes worker nodes
* A Linux host with ``kubectl`` (or ``oc``, if you're using OpenShift) installed
  and configured to manage the Kubernetes cluster you want to use
* Set the ``KUBECONFIG`` environment variable to point to your Kubernetes
  cluster configuration.
* Enable the :ref:`Feature Gates <Feature Requirements>` required by Trident

Got all that? Great! Let's get started.

1: Qualify your Kubernetes cluster
----------------------------------

You made sure that you have everything in hand from the
:ref:`previous section <Before you begin>`, right? Right.

The first thing you need to do is log into the Linux host and verify that it is
managing a *working*,
:ref:`supported Kubernetes cluster <Supported frontends (orchestrators)>` that
you have the necessary privileges to.

.. note::
  With OpenShift, you will use ``oc`` instead of ``kubectl`` in all of the
  examples that follow, and you need to login as **system:admin** first by
  running ``oc login -u system:admin`` or ``oc login -u kube-admin``.

.. code-block:: bash

  # Are you running a supported Kubernetes server version?
  kubectl version

  # Are you a Kubernetes cluster administrator?
  kubectl auth can-i '*' '*' --all-namespaces

  # Can you launch a pod that uses an image from Docker Hub and can reach your
  # storage system over the pod network?
  kubectl run -i --tty ping --image=busybox --restart=Never --rm -- \
    ping <management IP>

Identify your Kubernetes server version. You will be using it when you
:ref:`Install Trident <3: Install Trident>`.

2: Download & extract the installer
-----------------------------------

.. note::
   Trident's installer is responsible for creating a Trident pod, configuring
   the CRD objects that are used to maintain its state and to
   initialize the CSI Sidecars that perform actions such as provisioning and
   attaching volumes to the cluster hosts.

Download the latest version of the `Trident installer bundle`_ from the
*Downloads* section and extract it.

For example, if the latest version is 21.07.0:

.. code-block:: console

   wget https://github.com/NetApp/trident/releases/download/v21.07.0/trident-installer-21.07.0.tar.gz
   tar -xf trident-installer-21.07.0.tar.gz
   cd trident-installer

.. _Trident installer bundle: https://github.com/NetApp/trident/releases/latest

3: Install Trident
------------------

Install Trident in the desired namespace by executing the
:ref:`tridentctl install <install>` command. The installation procedure
slightly differs depending on the version of Kubernetes being used.

Installing Trident on Kubernetes 1.13
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

On Kubernetes ``1.13``, there are a couple of options when installing Trident:

- Install Trident in the desired namespace by executing the
  ``tridentctl install`` command with the ``--csi`` flag. The CSI interface is
  `first included in Kubernetes 1.13 <https://kubernetes.io/blog/2019/01/15/container-storage-interface-ga/>`_.
  and requires activating :ref:`feature gates <Feature Requirements>`.
  The output observed when installing will be similar to that shown
  :ref:`below <Installing Trident on Kubernetes 1.14 and above>`.

- If for some reason the :ref:`feature gates <Feature Requirements>` required by Trident
  cannot be enabled, you can install Trident without the ``--csi`` flag. This will
  configure Trident to work in its traditional format without using the CSI
  specification.

Installing Trident on Kubernetes 1.14 and above
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Install Trident in the desired namespace by executing the
``tridentctl install`` command.

.. code-block:: console

   $ ./tridentctl install -n trident
   ....
   INFO Starting Trident installation.                namespace=trident
   INFO Created service account.
   INFO Created cluster role.
   INFO Created cluster role binding.
   INFO Added finalizers to custom resource definitions.
   INFO Created Trident service.
   INFO Created Trident secret.
   INFO Created Trident deployment.
   INFO Created Trident daemonset.
   INFO Waiting for Trident pod to start.
   INFO Trident pod started.                          namespace=trident pod=trident-csi-679648bd45-cv2mx
   INFO Waiting for Trident REST interface.
   INFO Trident REST interface is up.                 version=21.07.0
   INFO Trident installation succeeded.
   ....

It will look like this when the installer is complete. Depending on
the number of nodes in your Kubernetes cluster, you may observe more pods:

.. code-block:: console

   $ kubectl get pod -n trident
   NAME                           READY   STATUS    RESTARTS   AGE
   trident-csi-679648bd45-cv2mx   4/4     Running   0          5m29s
   trident-csi-vgc8n              2/2     Running   0          5m29s

   $ ./tridentctl -n trident version
   +----------------+----------------+
   | SERVER VERSION | CLIENT VERSION |
   +----------------+----------------+
   | 21.07.0        | 21.07.0        |
   +----------------+----------------+

If that's what you see, you're done with this step, but **Trident is not
yet fully configured.** Go ahead and continue to the next step.

However, if the installer does not complete successfully or you don't see
a **Running** ``trident-csi-<generated id>``, then Trident had a problem and the platform was *not*
installed.

To help figure out what went wrong, you could run the installer again using the ``-d`` argument,
which will turn on debug mode and help you understand what the problem is:

.. code-block:: console

  ./tridentctl install -n trident -d

After addressing the problem, you can clean up the installation and go back to
the beginning of this step by first running:

.. code-block:: console

  ./tridentctl uninstall -n trident
  INFO Deleted Trident deployment.
  INFO Deleted cluster role binding.
  INFO Deleted cluster role.
  INFO Deleted service account.
  INFO Removed Trident user from security context constraint.
  INFO Trident uninstallation succeeded.

If you continue to have trouble, visit the
:ref:`troubleshooting guide <Troubleshooting>` for more advice.

Customized Installation
~~~~~~~~~~~~~~~~~~~~~~~

Trident's installer allows you to customize attributes. For example, if you have
copied the Trident image to a private repository, you can specify the image name by using
``--trident-image``.  If you have copied the Trident image as well as the needed CSI
sidecar images to a private repository, it may be preferable to specify the location
of that repository by using the ``--image-registry`` switch, which takes the form
``<registry FQDN>[:port]``.

To have Trident automatically configure worker nodes for you, use
``--enable-node-prep``. For more details on how it works, take a look at the
Auto Worker Node Prep section.

.. warning::

  Automatic worker node prep is a **beta feature** meant to be used in
  non-production environments only.

If you are using a distribution of Kubernetes where kubelet keeps its data on a path
other than the usual ``/var/lib/kubelet``, you can specify the alternate path by using
``--kubelet-dir``.

As a last resort, if you need to customize Trident's installation beyond what the
installer's arguments allow, you can also customize Trident's deployment files. Using
the ``--generate-custom-yaml`` parameter will create the following YAML files in the
installer's ``setup`` directory:

- trident-clusterrolebinding.yaml
- trident-deployment.yaml
- trident-crds.yaml
- trident-clusterrole.yaml
- trident-daemonset.yaml
- trident-service.yaml
- trident-namespace.yaml
- trident-serviceaccount.yaml

Once you have generated these files, you can modify them according to your needs and
then use the ``--use-custom-yaml`` to install your custom deployment of Trident.

.. code-block:: console

  ./tridentctl install -n trident --use-custom-yaml

4: Create and Verify your first backend
---------------------------------------

You can now go ahead and create a backend that will be used by Trident
to provision volumes. To do this, create a ``backend.json`` file that
contains the necessary parameters. Sample configuration files for
different backend types can be found in the ``sample-input`` directory.

Visit the :ref:`backend configuration guide <Backend configuration>`
for more details about how to craft the configuration file for
your backend type.

.. code-block:: bash

  cp sample-input/<backend template>.json backend.json
  # Fill out the template for your backend
  vi backend.json

.. code-block:: console

    ./tridentctl -n trident create backend -f backend.json
    +-------------+----------------+--------------------------------------+--------+---------+
    |    NAME     | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
    +-------------+----------------+--------------------------------------+--------+---------+
    | nas-backend | ontap-nas      | 98e19b74-aec7-4a3d-8dcf-128e5033b214 | online |       0 |
    +-------------+----------------+--------------------------------------+--------+---------+

If the creation fails, something was wrong with the backend configuration. You
can view the logs to determine the cause by running:

.. code-block:: console

   ./tridentctl -n trident logs

After addressing the problem, simply go back to the beginning of this step
and try again. If you continue to have trouble, visit the
:ref:`troubleshooting guide <Troubleshooting>` for more advice on how to
determine what went wrong.

5: Add your first storage class
-------------------------------

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
``sample-input/storage-class-csi.yaml.templ`` file that comes with the
installer, replacing ``__BACKEND_TYPE__`` with the storage driver name.

.. code-block:: bash

    ./tridentctl -n trident get backend
    +-------------+----------------+--------------------------------------+--------+---------+
    |    NAME     | STORAGE DRIVER |                 UUID                 | STATE  | VOLUMES |
    +-------------+----------------+--------------------------------------+--------+---------+
    | nas-backend | ontap-nas      | 98e19b74-aec7-4a3d-8dcf-128e5033b214 | online |       0 |
    +-------------+----------------+--------------------------------------+--------+---------+

    cp sample-input/storage-class-csi.yaml.templ sample-input/storage-class-basic-csi.yaml

    # Modify __BACKEND_TYPE__ with the storage driver field above (e.g., ontap-nas)
    vi sample-input/storage-class-basic-csi.yaml

This is a Kubernetes object, so you will use ``kubectl`` to create it in
Kubernetes.

.. code-block:: console

    kubectl create -f sample-input/storage-class-basic-csi.yaml

You should now see a **basic-csi** storage class in both Kubernetes and Trident,
and Trident should have discovered the pools on the backend.

.. code-block:: console

    kubectl get sc basic-csi
    NAME         PROVISIONER             AGE
    basic-csi    csi.trident.netapp.io   15h

    ./tridentctl -n trident get storageclass basic-csi -o json
    {
      "items": [
        {
          "Config": {
            "version": "1",
            "name": "basic-csi",
            "attributes": {
              "backendType": "ontap-nas"
            },
            "storagePools": null,
            "additionalStoragePools": null
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

6: Provision your first volume
------------------------------

Now you're ready to dynamically provision your first volume. How exciting! This
is done by creating a Kubernetes `persistent volume claim`_ (PVC) object, and
this is exactly how your users will do it too.

.. _persistent volume claim: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#persistentvolumeclaims

Create a persistent volume claim (PVC) for a volume that uses the storage
class that you just created.

See ``sample-input/pvc-basic-csi.yaml`` for an example. Make sure the storage
class name matches the one that you created in 6.

.. code-block:: bash

    kubectl create -f sample-input/pvc-basic-csi.yaml

    kubectl get pvc --watch
    NAME      STATUS    VOLUME                                     CAPACITY   ACCESS MODES  STORAGECLASS   AGE
    basic     Pending                                                                       basic          1s
    basic     Pending   pvc-3acb0d1c-b1ae-11e9-8d9f-5254004dfdb7   0                        basic          5s
    basic     Bound     pvc-3acb0d1c-b1ae-11e9-8d9f-5254004dfdb7   1Gi        RWO           basic          7s

7: Mount the volume in a pod
----------------------------

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
  kubectl get pod --watch

  # Verify that the volume is mounted on /usr/share/nginx/html
  kubectl exec -it task-pv-pod -- df -h /usr/share/nginx/html
  Filesystem                                                          Size  Used Avail Use% Mounted on
  10.xx.xx.xx:/trident_pvc_3acb0d1c_b1ae_11e9_8d9f_5254004dfdb7       1.0G  256K  1.0G   1% /usr/share/nginx/html


  # Delete the pod
  kubectl delete pod task-pv-pod

At this point the pod (application) no longer exists but the volume is still
there. You could use it from another pod if you wanted to.

To delete the volume, simply delete the claim:

.. code-block:: console

  kubectl delete pvc basic

**Check you out! You did it!** Now you're dynamically provisioning
Kubernetes volumes like a boss.

Where do you go from here? you can do things like:

  * :ref:`Configure additional backends <Backend configuration>`.
  * :ref:`Model additional storage classes <Managing storage classes>`.
  * Review considerations for moving this into production.
