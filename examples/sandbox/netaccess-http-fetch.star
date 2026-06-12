#!/usr/bin/env kite
# netaccess-http-fetch.star — fetches a URL over HTTPS from inside the
# default sandbox profile. Demonstrates that the host network is fully
# reachable and TLS verification works (the curated /etc/ssl/certs mount
# provides the CA roots).
#
# Run:
#   kite ./netaccess-http-fetch.star --sandbox --permissions=allow-net
#   STARKITE_SECURITY_SANDBOX=net-access ./netaccess-http-fetch.star --permissions=allow-net

resp = http.url("https://example.com").get()
print("status: %d" % resp.status_code)
print("body length: %d bytes" % len(resp.get_text()))

# $HOME is invisible to this script — uncomment to see the failure:
# print(read_text("/etc/passwd"))   # would error: file not found
