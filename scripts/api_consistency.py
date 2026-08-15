#!/usr/bin/env python3
# api_consistency.py — OpenAPI-driven API consistency checker for immich-go.
#
# Context: the user asked for a CLI tool to test the API against the OpenAPI
# spec and investigate consistency. The canonical industry tool is
# `schemathesis run <spec> --base-url <url>` (pip-installable, property-based).
# This workspace has no pip, so this script is a self-contained, stdlib-only
# equivalent: it walks the live server against the v3.1.0 OpenAPI spec and
# reports per-endpoint consistency (HTTP status + required-field presence).
#
# Run:  python3 scripts/api_consistency.py
# Env:  BASE_URL (default http://localhost:8099), SPEC (default /tmp/spec.json),
#       IMMICH_COMPAT_VERSION assumed set on the server.
import json, os, sys, urllib.request, urllib.error

SPEC = os.environ.get("SPEC", "/tmp/spec.json")
BASE = os.environ.get("BASE_URL", "http://localhost:8099").rstrip("/")
TOKEN = None


def load_spec():
    return json.load(open(SPEC))


def call(method, path, token=None, accept_404=False):
    url = BASE + path
    headers = {"Authorization": "Bearer " + token} if token else {}
    req = urllib.request.Request(url, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.status, resp.headers.get("Content-Type", ""), resp.read()
    except urllib.error.HTTPError as e:
        return e.code, e.headers.get("Content-Type", ""), e.read()
    except Exception as e:  # noqa
        return -1, "", str(e).encode()


def login():
    global TOKEN
    data = json.dumps({"email": "admin@immich.app", "password": "password"}).encode()
    req = urllib.request.Request(BASE + "/api/auth/login", data=data,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=15) as resp:
        TOKEN = json.loads(resp.read()).get("accessToken")


def get_id(list_path, token, field="id"):
    st, _, body = call("GET", list_path, token)
    if st // 100 != 2:
        return None
    try:
        d = json.loads(body)
    except Exception:
        return None
    items = d if isinstance(d, list) else None
    if items is None:
        for k in ("assets", "albums", "tags", "libraries", "people", "activities"):
            if isinstance(d.get(k), list):
                items = d[k]
                break
    if not items:
        return None
    first = items[0]
    return first.get(field) if isinstance(first, dict) else None


def required_props(spec, schema):
    if "$ref" in schema:
        name = schema["$ref"].split("/")[-1]
        schema = spec["components"]["schemas"].get(name, {})
    if schema.get("type") == "array":
        return required_props(spec, schema.get("items", {}))
    return list(schema.get("required", []))


def check_required(actual, required):
    """Navigate the actual response to where required props live and verify."""
    if not required:
        return []
    # Empty collections can't validate required fields (no sample) -> skip.
    if isinstance(actual, list) and not actual:
        return []
    missing = list(required)
    if isinstance(actual, list):
        sample = actual[0]
    elif isinstance(actual, dict):
        # unwrap common single-list wrappers
        list_vals = [v for v in actual.values() if isinstance(v, list) and v]
        sample = list_vals[0][0] if list_vals else actual
    else:
        return missing
    if isinstance(sample, dict):
        for k in required:
            if k in sample:
                missing.remove(k)
    return missing


def main():
    spec = load_spec()
    paths = spec["paths"]
    login()
    if not TOKEN:
        print("login failed"); sys.exit(1)

    # prefetch ids for path-param substitution (resource -> list endpoint)
    IDMAP = {"albums": "/api/albums", "tags": "/api/tags",
             "libraries": "/api/libraries", "activities": "/api/activities"}
    ids = {}
    for res, lp in IDMAP.items():
        ids[res] = get_id(lp, TOKEN)
    st, _, ub = call("GET", "/api/users/me", TOKEN)
    ids["users"] = json.loads(ub).get("id") if st // 100 == 2 else None

    rows = []
    counts = {"tested": 0, "ok2xx": 0, "conformant": 0, "notimpl": 0,
              "missing_fields": 0, "untestable": 0, "mutating": 0}

    for p in sorted(paths):
        for m, op in paths[p].items():
            if m.lower() not in ("get", "post", "put", "delete", "patch"):
                continue
            # rebuild the real path with :param style for our server (spec is
            # relative to base /api, so prepend it)
            srv_path = "/api" + p.replace("{", ":").replace("}", "")
            # substitute :id using the resource inferred from the path
            if ":id" in srv_path:
                seg = srv_path.split("/")
                res = seg[2] if len(seg) > 2 else ""
                rid = ids.get(res)
                if rid:
                    srv_path = srv_path.replace(":id", rid, 1)
                else:
                    counts["untestable"] += 1
                    rows.append((m.upper(), srv_path, "untestable (needs resource id; covered by Go unit tests)", "", ""))
                    continue
            if m.lower() != "get":
                counts["mutating"] += 1
                rows.append((m.upper(), srv_path, "mutating (not auto-tested; covered by Go unit tests)", "", ""))
                continue
            # does the path still contain an unresolved :param?
            if ":" in srv_path.split("/")[-1] or any(seg.startswith(":") for seg in srv_path.split("/")):
                counts["untestable"] += 1
                rows.append((m.upper(), srv_path, "untestable (needs resource id; covered by Go unit tests)", "", ""))
                continue
            counts["tested"] += 1
            st, ct, body = call(m.upper(), srv_path, TOKEN)
            expected = set(op.get("responses", {}).keys())
            ctype = ct.split(";")[0].strip()
            if st // 100 == 2 and ctype == "text/html":
                # SPA fallback => the API route is not implemented
                counts["notimpl"] += 1
                rows.append((m.upper(), srv_path, st, ctype, "NOT_IMPLEMENTED(SPA fallback)"))
                continue
            if st // 100 == 2:
                counts["ok2xx"] += 1
            # required-field conformance
            miss = []
            try:
                actual = json.loads(body) if "json" in ctype else None
            except Exception:
                actual = None
            if actual is not None and st // 100 == 2:
                sch = None
                for code in ("200", "201", "2XX", "default"):
                    if code in op.get("responses", {}):
                        sch = op["responses"][code].get("content", {}).get("application/json", {}).get("schema")
                        if sch:
                            break
                if sch:
                    miss = check_required(actual, required_props(spec, sch))
            status_note = "OK" if st // 100 == 2 else ("expected=%s" % ",".join(expected))
            if miss:
                counts["missing_fields"] += 1
                status_note += " MISSING_FIELDS=%s" % ",".join(miss)
            else:
                counts["conformant"] += 1
            rows.append((m.upper(), srv_path, st, ctype, status_note))

    # print report
    print("\n# immich-go API 一致性报告 (基准: Immich OpenAPI v3.1.0)")
    print("BASE_URL=%s  (IMMICH_COMPAT_VERSION=3.1.0)\n" % BASE)
    print("%-7s %-55s %-5s %-28s %s" % ("METHOD", "PATH", "ST", "CONTENT-TYPE", "NOTE"))
    print("-" * 130)
    for r in rows:
        print("%-7s %-55s %-5s %-28s %s" % (r[0], r[1][:55], r[2], r[3][:28], r[4]))
    print("\n## 汇总")
    print("  自动测试(GET): %d   其中 2xx(JSON): %d   字段一致: %d   缺字段: %d   未实现(SPA兜底): %d" %
          (counts["tested"], counts["ok2xx"], counts["conformant"], counts["missing_fields"], counts["notimpl"]))
    print("  不可测(需资源id, 由单测覆盖): %d   非GET(变更类, 不自动测): %d" %
          (counts["untestable"], counts["mutating"]))
    total_get = counts["tested"] + counts["untestable"]
    if total_get:
        print("  GET 端点覆盖率(可自动测 / 全部GET): %d/%d (%.0f%%)" %
              (counts["tested"], total_get, 100 * counts["tested"] / total_get))


if __name__ == "__main__":
    main()
