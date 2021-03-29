#!/usr/bin/env python3

import humps

def pascalize(s):
    s = s.replace(".", "_")
    s = s.replace("_url", "_URL")
    s = s.replace("_uuid", "_UUID")
    s = s.replace("_uid", "_UID")
    s = s.replace("_http", "_HTTP")
    s = s.replace("_id", "_ID")
    s = s.replace("_ip", "_IP")

    s = s.replace("url_", "URL_")
    s = s.replace("uuid_", "UUID_")
    s = s.replace("uid_", "UID_")
    s = s.replace("http_", "HTTP_")
    s = s.replace("id_", "ID_")
    s = s.replace("ip_", "IP_")
    return humps.pascalize(s)


print(pascalize("cap.url.query.parameter"))
print(pascalize("cluster.uuid.QueryParameter"))
print(pascalize("frus.id"))

