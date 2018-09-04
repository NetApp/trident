Known issues
^^^^^^^^^^^^

#. Volume names must be a minimum of 2 characters in length

   This is a Docker client limitation. The client will interpret a single character name as being a Windows path.
   `See bug 25773 <https://github.com/docker/docker/issues/25773>`_.

#. Docker Swarm presently makes use of volume name instead of volume ID as its unique volume identifier.  This combined with 1) Swarm's design & distributed nature (in which volume requests are simultaneously sent to each node simultaneously and Trident must run independently at each) and 2) Element OS 's general flexibility to create multiple commonly named volumes (with differing volume IDs) result in only the ontap-nas and ontap-san drivers functioning in Swarm mode.  NetApp has provided feedback to the Docker team, but does not have any indication of future recourse.


#. If a FlexGroup is in the process of being provisioned, ONTAP will not provision a second FlexGroup if the second
   FlexGroup has one or more aggregates in common with the FlexGroup being provisioned.
