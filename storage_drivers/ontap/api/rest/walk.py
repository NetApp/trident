#!/usr/bin/env python3

# https://pypi.org/project/pyhumps/
import humps

#import yaml
import ruamel.yaml
from ruamel.yaml.scalarstring import DoubleQuotedScalarString as dq
from pprint import pprint as pp


def walk(v):
    pp(type(v))
    pp(v)

    if isinstance(v, ruamel.yaml.comments.CommentedSeq):
        for d in v:
            if "in" in d:
                in_value = d["in"]
                if in_value == "query":
                    if "name" in d:
                        name = d["name"] + ".query"
                        go_variable_name = humps.pascalize(name.replace(".", "_"))
                        d["x-go-name"] = dq(go_variable_name)
                if in_value == "path":
                    if "name" in d:
                        name = d["name"] + ".path"
                        go_variable_name = humps.pascalize(name.replace(".", "_"))
                        d["x-go-name"] = dq(go_variable_name)

    if isinstance(v, list):
        for e in v:
            walk(e)

    if isinstance(v, dict):
        for key in list(v.keys()):
            walk(v[key])

with open('./snippet.yaml') as input_file:

    yaml = ruamel.yaml.YAML(typ='safe')
    yaml.indent(sequence=4, offset=2)
    dataMap = yaml.load(input_file)

    walk(dataMap)


