#!/usr/bin/env python3

###
# See also:
#   https://github.com/yaml/pyyaml/wiki/PyYAML-yaml.load(input)-Deprecation
#   https://stackoverflow.com/questions/11875770/how-to-overcome-datetime-datetime-not-json-serializable#36142844

import sys
#import yaml
import ruamel.yaml
import json

def custom_default(obj):
    """Overwrite the default JSON serializer."""
    import calendar, datetime

    if isinstance(obj, datetime.datetime):
        if obj.utcoffset() is not None:
            obj = obj - obj.utcoffset()
        millis = int(
            calendar.timegm(obj.timetuple()) * 1000 +
            obj.microsecond / 1000
        )
        return millis
    raise TypeError('Not sure how to serialize %s' % (obj,))

yaml = ruamel.yaml.YAML()
yaml.indent(sequence=4, offset=2)

with open('swagger_full_converted.yaml') as swagger_yaml:
    #y = yaml.load(swagger_yaml, Loader=yaml.FullLoader)
    y = yaml.load(swagger_yaml)
    with open('swagger_full_converted.json', 'w') as swagger_json:
        json.dump(y, swagger_json, indent=4, default=custom_default)

