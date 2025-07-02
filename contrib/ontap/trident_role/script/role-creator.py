"""
ONTAP Role Creation Python Sample Scripts
This script was developed by NetApp to help demonstrate
NetApp technologies. This script is not officially
supported as a standard NetApp product.

Purpose: THE FOLLOWING SCRIPT SHOWS HOW TO CREATE ROLE USING REST API.

usage: role-creator.py [-h] [-v VSERVER_NAME] [-r ROLE_NAME] [--zapi] [--rest] [-i HOST_IP] [-u USERNAME] [-p PASSWORD] [-j JSON]
                       [--log-level {CRITICAL,FATAL,ERROR,WARN,WARNING,INFO,DEBUG,NOTSET}]

Copyright (c) 2025 NetApp, Inc. All Rights Reserved.
Licensed under the BSD 3-Clause "New or Revised" License (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
https://opensource.org/licenses/BSD-3-Clause
"""

import json
import logging
import sys
import argparse
from ontap_cli import ONTAPCLI
from netapp_ontap import NetAppRestError

vserver_name = None
role_name = None
zapi = False
rest = False
host_ip = None
username = None
password = None
log = None
log_level = None
zapi_json_file = "./zapi_custom_role.json"
rest_json_file = "./rest_custom_role.json"
json_file = None


def validate_flags(args, parser):
    if args.host_ip is None:
        log.error("Please provide host_ip")
        parser.print_help()
        sys.exit(1)

    if args.username is None:
        log.error("Please provide username")
        parser.print_help()
        sys.exit(1)

    if args.password is None:
        log.error("Please provide password")
        parser.print_help()
        sys.exit(1)

    if args.zapi is False and args.rest is False:
        log.error("Please provide at least one either zapi or rest flag")
        parser.print_help()
        sys.exit(1)

    if args.zapi is True and args.rest is True:
        log.error("Please provide either of one zapi or rest flag")
        parser.print_help()
        sys.exit(1)


def read_flags():
    # Setting up the flags that can be passed to the script.
    parser = argparse.ArgumentParser(description="Role generator.")
    parser.add_argument(
        "-v", "--vserver-name", help="The name of the vserver, default is None"
    )
    parser.add_argument(
        "-r", "--role-name", help="The name of the role, default is 'trident'"
    )
    parser.add_argument("--zapi", action="store_true", help="Creates ZAPI role")
    parser.add_argument("--rest", action="store_true", help="Creates REST role")
    parser.add_argument("-i", "--host-ip", help="The IP address of the ONTAP")
    parser.add_argument("-u", "--username", help="The username of the ONTAP")
    parser.add_argument("-p", "--password", help="The password of the ONTAP")
    parser.add_argument(
        "-j",
        "--json",
        help="Path to the input JSON file, default is ZAPI='./zapi_custom_role.json REST='./rest_custom_role.json'",
    )
    parser.add_argument(
        "--log-level",
        help="The log level of the script",
        default="INFO",
        choices=[
            "CRITICAL",
            "FATAL",
            "ERROR",
            "WARN",
            "WARNING",
            "INFO",
            "DEBUG",
            "NOTSET",
        ],
    )

    args = parser.parse_args()

    # Validating the flags that are passed
    validate_flags(args, parser)

    global zapi, rest, vserver_name, role_name, host_ip, username, password, log_level, json_file

    # If vserver_name is not passed, it will be None
    # meaning, the roles will be created at the cluster level.
    if args.vserver_name is not None:
        vserver_name = args.vserver_name

    # If role_name is not passed, it will be 'trident'
    if args.role_name is not None:
        role_name = args.role_name
    else:
        role_name = "trident"

    if args.zapi:
        if args.json is None:
            json_file = zapi_json_file
    else:
        if args.json is None:
            json_file = rest_json_file

    zapi = args.zapi
    rest = args.rest
    host_ip = args.host_ip
    username = args.username
    password = args.password
    log_level_str = args.log_level
    log_level = getattr(logging, log_level_str.upper(), None)


def zapi_roles(ontap_cli):
    # Reading the JSON file, which contains the ZAPI commands and the access level
    with open(json_file, "r") as f:
        data = json.load(f)

    for item in data:
        # Creating/modifying the role for the command.
        log.info(
            f"Creating role for command {item['command']} with access {item['access_level']}"
        )
        body = {
            "cmddirname": item["command"],
            "access": item["access_level"],
            "role": role_name,
        }
        if vserver_name is not None:
            body["vserver"] = vserver_name
        try:
            ontap_cli.execute(command="security login role create", body=body)
        except NetAppRestError as e:
            log.error(e.cause)


def rest_roles(ontap_cli):
    # Reading the JSON file, which contains the REST API paths and the access level
    with open(json_file, "r") as f:
        data = json.load(f)

    for item in data:
        # Creating/modifying the role for the path.
        log.info(f"Creating role for api {item['path']} with access {item['access']}")
        body = {"access": item["access"], "api": item["path"], "role": role_name}
        if vserver_name is not None:
            body["vserver"] = vserver_name
        try:
            ontap_cli.execute(command="security login rest-role create", body=body)
        except NetAppRestError as e:
            log.error(e.cause)


if __name__ == "__main__":
    # Reading the flags that are passed
    read_flags()

    # Setting up the logger
    log = logging.getLogger()
    log.setLevel(log_level)
    handler = logging.StreamHandler(sys.stdout)
    handler.setLevel(logging.DEBUG)
    log.addHandler(handler)

    # Creating the ONTAPCLI object, which will be used to execute the commands,
    # directly on the ONTAP.
    ontap_cli = ONTAPCLI(host_mgmt_ip=host_ip, username=username, password=password)

    # Based on the flags passed, we will either create the roles using ZAPI or REST
    if zapi:
        zapi_roles(ontap_cli)
    else:
        rest_roles(ontap_cli)
