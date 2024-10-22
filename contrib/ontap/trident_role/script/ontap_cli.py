import logging
import json
from netapp_ontap import HostConnection
from netapp_ontap.resources import CLI


class ONTAPCLI(object):
    def __init__(self, host_mgmt_ip, username="admin", password=None):
        # Set appropriate class variables
        self.LOG = logging.getLogger()
        self.host_mgmt_ip = host_mgmt_ip
        self.username = username
        self.password = password

        # Check credentials and connection success
        try:
            HostConnection(
                self.host_mgmt_ip,
                username=self.username,
                password=self.password,
                verify=False,
            )
        except Exception as e:
            self.LOG.exception(
                f"Unable to successfully connect.  Check your username/password. Exception:{e}"
            )
            raise e

    def execute(self, command, **kwargs):
        with HostConnection(
            self.host_mgmt_ip,
            username=self.username,
            password=self.password,
            verify=False,
        ):
            response = CLI().execute(command=command, **kwargs)
            self.LOG.debug(json.dumps(response.http_response.json(), indent=4))
            return response.http_response.json()
