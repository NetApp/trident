#!/usr/bin/env python3

from pprint import pprint as pp

#import yaml
import ruamel.yaml
from ruamel.yaml.scalarstring import DoubleQuotedScalarString as dq

yaml = ruamel.yaml.YAML(typ='safe')
yaml.indent(sequence=4, offset=2)

with open('swagger_full.yaml') as input_file:
    dataMap = yaml.load(input_file)
    dm = dataMap

with open('swagger_full_converted.yaml') as input_file:
    dataMap2 = yaml.load(input_file)
    dm2 = dataMap2

#pp(dataMap)

