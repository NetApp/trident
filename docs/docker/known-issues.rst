Known issues
^^^^^^^^^^^^

#. Volume names must be a minimum of 2 characters in length

   This is a Docker client limitation. The client will interpret a single character name as being a Windows path.
   `See bug 25773 <https://github.com/docker/docker/issues/25773>`_.

#. Because Docker Swarm does not orchestrate volume creation across multiple nodes, only the ontap-nas and ontap-san
   drivers will work in Swarm.

#. If a FlexGroup is in the process of being provisioned, ONTAP will not provision a second FlexGroup if the second
   FlexGroup has one or more aggregates in common with the FlexGroup being provisioned.
