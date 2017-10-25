########
REST API
########

While :ref:`tridentctl` is the easiest way to interact with Trident's REST API,
you can use the REST endpoint directly if you prefer.

This is particularly useful for advanced installations that are using Trident
as a standalone binary in non-Kubernetes deployments.

For better security, Trident's :ref:`REST API` is restricted to localhost by
default when running inside a pod. You will need to set Trident's ``-address``
argument in its pod configuration to change this behavior.

The API works as follows:

* ``GET <trident-address>/trident/v1/<object-type>``:  Lists all objects of that
  type.
* ``GET <trident-address>/trident/v1/<object-type>/<object-name>``:  Gets the
  details of the named object.
* ``POST <trident-address>/trident/v1/<object-type>``:  Creates an object of the
  specified type.  Requires a JSON configuration for the object to be created;
  see the previous section for the specification of each object type.  If the
  object already exists, behavior varies:  backends update the existing object,
  while all other object types will fail the operation.
* ``DELETE <trident-address>/trident/v1/<object-type>/<object-name>``:  Deletes
  the named resource.  Note that volumes associated with backends or storage
  classes will continue to exist; these must be deleted separately.  See the
  section on backend deletion below.

To see an example of how these APIs are called, pass the debug (``-d``) flag
to :ref:`tridentctl`.
