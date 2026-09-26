#!/usr/bin/env kite
# requests.star - HTTP client convenience methods
#
# Demonstrates top-level http functions for sending GET, POST, PUT,
# PATCH, and DELETE requests without manually instantiating http.client().
#
# Run:
#   kite run --allow-net ./examples/core/http-client/requests.star
#   kite run --dry-run --allow-net ./examples/core/http-client/requests.star

def main():
    # 1. Simple GET request
    print("=== 1. Simple GET ===")
    resp = http.get("https://httpbin.org/get", timeout="10s")
    print("Status:", resp.status_code)
    print("Headers:", resp.headers)
    print("Body preview:", resp.get_text()[:60])

    # 2. POST with dict (auto-encoded to JSON) and custom headers
    print("\n=== 2. POST with JSON body ===")
    post_resp = http.post(
        "https://httpbin.org/post",
        body={"message": "Hello from starkite!", "version": "v0.8"},
        headers={"User-Agent": "starkite-http"},
    )
    print("Status:", post_resp.status_code)
    print("Response text:", post_resp.get_text()[:80])

    # 3. PUT and DELETE requests
    print("\n=== 3. PUT & DELETE ===")
    put_resp = http.put("https://httpbin.org/put", body="raw text payload")
    print("PUT Status:", put_resp.status_code)

    del_resp = http.delete("https://httpbin.org/delete")
    print("DELETE Status:", del_resp.status_code)

    # 4. Safe error handling with try_ variants
    print("\n=== 4. Safe error handling (try_get) ===")
    # try_get returns a Result object that never raises an unhandled error
    res = http.try_get("https://invalid-host-starkite-test-xyz.org", timeout="2s")
    if res.ok:
        print("Success:", res.value.status_code)
    else:
        print("Handled expected error cleanly:", res.error)
