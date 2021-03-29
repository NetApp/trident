#!/usr/bin/env python3

# https://pypi.org/project/pyhumps/
import humps

#import yaml
import ruamel.yaml
from ruamel.yaml.scalarstring import DoubleQuotedScalarString as dq

def pascalize(s):
    print("pascalize ", s)
    s = s.replace(".", "_")
    s = s.replace("_url", "_URL")
    s = s.replace("_uuid", "_UUID")
    s = s.replace("_uid", "_UID")
    s = s.replace("_http", "_HTTP")
    s = s.replace("_id", "_ID")
    s = s.replace("_ipv4", "_IPV4")
    s = s.replace("_ipv6", "_IPV6")
    s = s.replace("_ip", "_IP")
    s = s.replace("_vm", "_VM")
    s = s.replace("_acl", "_ACL")
    s = s.replace("_tcp", "_TCP")
    s = s.replace("_udp", "_UDP")
    s = s.replace("_dns", "_DNS")
    s = s.replace("_uri", "_URI")
    s = s.replace("_tls", "_TLS")
    #s = s.replace("_kdc", "_KDC")

    s = s.replace("url_", "URL_")
    s = s.replace("uuid_", "UUID_")
    s = s.replace("uid_", "UID_")
    s = s.replace("http_", "HTTP_")
    s = s.replace("id_", "ID_")
    s = s.replace("ipv4_", "IPV4_")
    s = s.replace("ipv6_", "IPV6_")
    s = s.replace("ip_", "IP_")
    s = s.replace("vm_", "VM_")
    s = s.replace("acl_", "ACL_")
    s = s.replace("tcp_", "TCP_")
    s = s.replace("udp_", "UDP_")
    s = s.replace("dns_", "DNS_")
    s = s.replace("uri_", "URI_")
    s = s.replace("tls_", "TLS_")
    #s = s.replace("kdc_", "KDC_")

    s = humps.pascalize(s)
    s = s.replace("IPv4", "IPV4")
    s = s.replace("IPv6", "IPV6")
    return s

###
# Do we need to handle all these x-ntap field types?
# grep x-ntap swagger_full.yaml  | \
#   grep -v readCreate | \
#   grep -v writeOnly  | \
#   grep -v advanced   | \
#   grep -v createOnly | \
#   grep -v modifyOnly | \
#   grep -v readModify | \
#   grep -v long-description

############################################################################################################
# See also:
#   https://stackoverflow.com/questions/6866600/how-to-parse-read-a-yaml-file-into-a-python-object#6866697
############################################################################################################

def remove_invalid_doc_fields(d):
    invalid_doc_paths = [
        "Getting started with the ONTAP REST API",
        "HAL linking",
        "HTTP methods",
        "HTTP status codes",
        "Query parameters",
        "Query-based PATCH and DELETE",
        "Records and pagination",
        "Requesting-specific-fields",
        "Response body",
        "SVM tunneling",
        "Size properties",
        "Sorting records",
        "Synchronous-and-asynchronous-operations",
        "Using the private CLI passthrough with the ONTAP REST API",
    ]
    for key in invalid_doc_paths:
        # TODO only pop if the key exists, in case they fix this later
        # TODO a better way to detect these? these are all paths that don't start with "/"
        d['paths'].pop(key)

def fix_incorrect_enum_type(d):
    #   - description: Filter by block_storage.primary.disk_type
    #     in: query
    #     name: block_storage.primary.disk_type
    #     type: enum

    #d["paths"]["/storage/aggregates"]["get"]["parameters"][7]["type"] = "string"
    for parameter in d["paths"]["/storage/aggregates"]["get"]["parameters"]:
        if "type" in parameter:
            if parameter["type"] == "enum":
                parameter["type"] = "string"

def fix_incorrect_unsigned_type(d):
    #   - description: Filter by ha.ports.number
    #     in: query
    #     name: ha.ports.number
    #     type: unsigned

    for parameter in d["paths"]["/cluster/nodes"]["get"]["parameters"]:
        if "type" in parameter:
            if parameter["type"] == "unsigned":
                parameter["type"] = "integer"

def fix_incorrect_type_for_ignore_warnings(d):
    # - "paths./cloud/targets/{uuid}.patch.parameters" must validate one and only one schema (oneOf). Found none valid
    # - paths./cloud/targets/{uuid}.patch.parameters in body must be of type array
    # - paths./cloud/targets/{uuid}.patch.parameters.in in body should be one of [header]
    #
    #   - description: Specifies whether or not warnings should be ignored.
    #     in: query
    #     items:
    #       type: string
    #     name: ignore_warnings
    #     type: boolean
    for parameter in d["paths"]["/cloud/targets/{uuid}"]["patch"]["parameters"]:
        if "name" in parameter:
            if parameter["name"] == "ignore_warnings":
                parameter.pop("items") # remove the incorrect entry for items here

def fix_incorrect_operationIds_for_snaplock(d):
    # - "snaplock_legal_hold_collection_get" is defined 2 times
    # this one is defined as snaplock_legal_hold_collection_get but should be snaplock_legal_hold_get
    d["paths"]["/storage/snaplock/litigations/{id}"]["get"]["operationId"] = "snaplock_legal_hold_get"

    # this one is defined as snaplock_legal_hold_get but should be snaplock_legal_hold_operation_get
    d["paths"]["/storage/snaplock/litigations/{litigation.id}/operations/{id}"]["get"]["operationId"] = "snaplock_legal_hold_operation_get"

def fix_incorrect_default_value_for_include_extensions(d):
    # - definitions.vscan_on_access.scope.include_extensions.default in body must be of type array: "string"
    # - definitions.vscan_on_demand.scope.include_extensions.default in body must be of type array: "string"
    # they were "*" instead of ["*"]
    d["definitions"]["vscan_on_access"]["properties"]["scope"]["properties"]["include_extensions"]["default"] = ["*"]
    d["definitions"]["vscan_on_demand"]["properties"]["scope"]["properties"]["include_extensions"]["default"] = ["*"]

def remove_extra_fields_for_snmp_user_definition(d):
    # - definitions.snmp_user.scope.default in body must be of type string: "null"
    # - definitions.snmp_user.scope.default in body should be one of [svm cluster]
    d["definitions"]["snmp_user"]["properties"]["scope"].pop("default")
    d["definitions"]["snmp_user"]["properties"]["scope"].pop("description")
    d["definitions"]["snmp_user"]["properties"]["scope"].pop("example")
    d["definitions"]["snmp_user"]["properties"]["scope"].pop("readOnly")

def add_unique_types_for_properties(d):
    d["definitions"]["application"]["properties"]["template"]["x-go-name"] = "ApplicationTemplateType"

    d["definitions"]["s3_bucket"]["properties"]["svm"]["x-go-name"] = "S3BucketSvmType"

    d["definitions"]["ip_interface"]["properties"]["svm"]["x-go-name"] = "IPInterfaceSvmType"
    d["definitions"]["ip_interface"]["properties"]["svm"]["properties"]["_links"]["x-go-name"] = "IPInterfaceSvmLinksType"

    d["definitions"]["fc_interface"]["properties"]["svm"]["x-go-name"] = "FcInterfaceSvmType"
    d["definitions"]["fc_interface"]["properties"]["svm"]["properties"]["_links"]["x-go-name"] = "FcInterfaceSvmLinksType"

# def add_go_package_names(d):
#     d["definitions"]["application"]["properties"]["template"]["x-go-name"] = "ApplicationTemplateType"


def walk(o):

    if isinstance(o, ruamel.yaml.comments.CommentedSeq):
        # set an x-go-name to remove duplicated variable names when go-swagger hits these
        for d in o:
            if isinstance(d, dict):
                if "in" in d:
                    in_value = d["in"]
                    if in_value == "query":
                        if "name" in d:
                            name = d["name"] + ".query.parameter"
                            go_variable_name = pascalize(name)
                            d["x-go-name"] = dq(go_variable_name)
                    if in_value == "path":
                        if "name" in d:
                            name = d["name"] + ".path.parameter"
                            go_variable_name = pascalize(name)
                            d["x-go-name"] = dq(go_variable_name)

    if isinstance(o, list):
        for e in o:
            walk(e)

    if isinstance(o, dict):
        d = o
        for key in list(d.keys()):
            v = d[key]
            if isinstance(v, dict):
                # correct typo of the word 'description'
                if "decription" in v:
                    # - definitions.software_status_details_reference.properties.action.properties.message.decription in body is a forbidden property
                    # - definitions.software_status_details_reference.properties.issue.properties.message.decription in body is a forbidden property
                    v["description"] = v.pop("decription")

                # correct other typo of the word 'description'
                if "descriptions" in v:
                    # - definitions.nvme_subsystem.properties.hosts.descriptions in body is a forbidden property
                    v["description"] = v.pop("descriptions")

                # handle ~ (None) values
                if "description" in v:
                    description = v["description"]
                    if (description == "~") or (description is None):
                        # Trying to accomate
                        #  flexcache_relationship:
                        #    description: ~
                        # I think we have 2 options here
                        # 1) just make it an empty string
                        # 2) potentially, just remove the null description???
                        # let's try #1 first
                        v["description"] = ""

                if "type" in v:
                    type = v["type"]

                    isBooleanType = (type == "boolean")
                    isStringType = (type == "string")
                    isIntegerType = (type == "integer")
                    isArrayType = (type == "array")
                    isArrayTypeString = False

                    if isArrayType:
                        if "items" in v:
                            items = v["items"]
                            if "type" in items:
                                items_type = items["type"]
                                isArrayTypeString = True
                        # to fix issues like Snapshot.Owners being sent when it should not be sent
                        # see also:  https://goswagger.io/use/models/schemas.html#omit-empty-values
                        v["x-omitempty"] = True

                    if isIntegerType:
                        # correct minValue to be minimum
                        # correct maxValue to be maximum
                        # see also:  https://swagger.io/docs/specification/data-models/data-types/#numbers
                        if "minValue" in v:
                            minValue = v["minValue"]
                            v['minimum'] = v.pop("minValue")
                        if "maxValue" in v:
                            maxValue = v["maxValue"]
                            v['maximum'] = v.pop("maxValue")

                    if "default" in v:
                        default = v["default"]
                        #print("for", path+"/"+key, "of type:", type, "found default:", default)

                        if (default == "~") or (default is None):
                            # Trying to correct the use_mirrored_aggregates default value of ~
                            # https://stackoverflow.com/questions/51990175/what-is-the-purpose-of-tilde-character-in-yaml
                            v.pop("default") # convert this into a nullable value
                            v["x-nullable"] = True

                        if isBooleanType:
                            if default == "enabled":
                                v["default"] = True

                        if isStringType:
                            v["default"] = dq(v["default"])  # force quotes around the default string value

                    if "example" in v:
                        example = v["example"]
                        #print("for", path+"/"+key, "of type:", type, "found example:", example)

                        if isStringType:
                            v["example"] = dq(v["example"])  # force quotes around the example string value

                        if isArrayType:
                            if isArrayTypeString:
                                for i in range(0, len(example)):
                                    example[i] = dq(example[i])  # force quotes around the example string values

                    if "readOnly" in v:
                        # dataMap["definitions"]["cluster_metrics_response"]["properties"]["records"]["items"]["readOnly"]
                        readOnly = v["readOnly"]
                        if readOnly == 1:
                            # correcting the readOnly boolean property to have a value of True when its set to 1
                            v["readOnly"] = True

                    if "writeOnly" in v:
                        # - definitions.account.properties.password.writeOnly in body is a forbidden property
                        # - definitions.account_password.properties.password.writeOnly in body is a forbidden property
                        v["x-ntap-writeOnly"] = v.pop("writeOnly")

                    if "format" in v:
                        format = v["format"]
                        if format == "date-time":
                            ### TODO maybe only do this in some cases, not all cases?
                            v["x-nullable"] = True
                            #print("for", path+"/"+key, "of type:", type, "added x-nullable: True")

                        if format == "datetime":
                            # - info.expiry_time.default in body must be of type datetime: "365days"
                            # - definitions.security_certificate_sign.expiry_time.default in body must be of type datetime: "365days"
                            #v["format"] = "date-time"
                            v.pop("format")
                            if "default" in v:
                                default = v["default"]
                                if default == "365days":
                                    v.pop("default")


                    # https://stackoverflow.com/questions/52423986/python-yaml-generate-few-values-in-quotes
                    if "enum" in v:
                        # dataMap["definitions"]["account"]["properties"]["scope"]["enum"][0] = '"cluster"'
                        if isStringType:
                            #print("for", path+"/"+key, "of type:", type, "found enum")
                            enum = v["enum"]
                            for i in range(0, len(enum)):
                                enum[i] = dq(enum[i])  # force quotes around the string value

                    if type == "enum":
                        # the type should be string, not enum
                        # - "definitions.aggregate.properties.block_storage.properties.primary.properties.disk_type.type" must validate at least one schema (anyOf)
                        # - definitions.aggregate.properties.block_storage.properties.primary.properties.disk_type.type in body must be of type array: "string"
                        v["type"] = "string"

                    if type == "unsigned":
                        # the type should be integer, not unsigned
                        # - "definitions.cluster.properties.nodes.items" must validate at least one schema (anyOf)
                        # - "definitions.cluster.properties.nodes.items.properties.ha.properties.ports.items" must validate at least one schema (anyOf)
                        # - "definitions.cluster.properties.nodes.items.properties.ha.properties.ports.items.properties.number.type" must validate at least one schema (anyOf)
                        # - definitions.cluster.properties.nodes.items.properties.ha.properties.ports.items.properties.number.type in body must be of type array: "string"
                        v["type"] = "integer"


                    # if "in" in v:
                    #     in_parameter = v["in"]
                    #     if in_parameter == "query":
                    #         if "name" in v:
                    #             name = v["name"] + ".query"
                    #             go_variable_name = humps.pascalize(name.replace(".", "_"))
                    #             v["x-go-name"] = dq(go_variable_name)
                    #     if in_parameter == "path":
                    #         if "name" in v:
                    #             name = v["name"] + ".path"
                    #             go_variable_name = humps.pascalize(name.replace(".", "_"))
                    #             v["x-go-name"] = dq(go_variable_name)

            walk(v) # maybe indent again?

with open('swagger_full.yaml') as input_file:
    #dataMap = yaml.safe_load(input_file)

    yaml = ruamel.yaml.YAML()
    yaml.indent(sequence=4, offset=2)
    dataMap = yaml.load(input_file)

    remove_invalid_doc_fields(dataMap)
    fix_incorrect_enum_type(dataMap)
    fix_incorrect_unsigned_type(dataMap)
    fix_incorrect_type_for_ignore_warnings(dataMap)
    fix_incorrect_operationIds_for_snaplock(dataMap)
    fix_incorrect_default_value_for_include_extensions(dataMap)
    remove_extra_fields_for_snmp_user_definition(dataMap)
    add_unique_types_for_properties(dataMap)

    walk(dataMap)

    with open('swagger_full_converted.yaml', 'w') as output_file:
        yaml.dump(dataMap, output_file)

