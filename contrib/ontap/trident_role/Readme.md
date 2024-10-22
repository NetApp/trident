
# Trident custom-role generator

There are three primary methods for users to consume the list of commands/APIs required to create a role on ONTAP specific to Trident.



## Options

- Raw
- ONTAP CLI Pastable
- Python script


## Raw

In the [**raw**](raw) folder, users can access a list of ZAPI commands and REST paths in JSON format, which can be consumed as needed.

## ONTAP CLI Pastable

In the [**cli_pastable**](cli_pastable) folder, users can access a list of ONTAP CLI commands that can be copy-pasted into the ONTAP CLI to create a custom role.
- In order to create a zapi-based role, users can copy-paste the commands in the [**zapi_custom_role_output.txt**](cli_pastable/zapi_custom_role_output.txt) file.
- In order to create a rest-based role, users can copy-paste the commands in the [**rest_custom_role_output.txt**](cli_pastable/rest_custom_role_output.txt) file.

Both ZAPI and REST roles are currently designed to create a role named **trident** at the cluster level. If users need to create a role at the SVM level or change the role name from trident to a different name, you can utilize the role-generator bash script provided alongside the output files.

Before proceeding, ensure you copy the commands/APIs from the [**raw**](raw) folder to the location where you will run the script, or provide the path to the script accordingly.

How to use the script:

```bash
./role-generator.sh -r <role-name> -v <vserver-name> --zapi/--rest
````

To view detailed usage instructions, execute the script with the -h or --help option.

## Python script

In the [**script**](script) folder, users can access a Python script that can be used to create a custom role on ONTAP

How to use the script:

```bash
pip install -r requirements.txt
python role-creator.py -i <host-ip> -u <username> -p <password> --zapi/--rest
```
Same as above, here too you need to copy the commands/APIs from the [**raw**](raw) folder to the location where you will run the script, or provide the path to the script accordingly.

By default, the script will create a role named **trident** at the cluster level. If you need to create a role at the SVM level or change the role name from trident to a different name, you can use **--role-name** and **--vserver-name** options.


To view detailed usage instructions, execute the script with the -h or --help option.



