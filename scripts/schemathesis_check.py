#!/usr/bin/env python3
"""immich-go API contract regression check (Schemathesis).

This is the regression gate for API contract conformance against the official
Immich v3.1.0 OpenAPI spec. It builds immich-go, starts it on a throwaway DB,
logs in, then runs Schemathesis with the conformance checks and decides pass/fail.

GATE LOGIC
----------
The run FAILS (exit 1) when a CRITICAL check regresses:
  - response_schema_conformance  (DTO shape is wrong)
  - content_type_conformance     (wrong Content-Type)
  - not_a_server_error           (5xx server error)
Status-code conformance failures are ALLOWED only for operations listed in
scripts/schemathesis-allowlist.txt (known-unimplemented endpoints / benign
edges such as POST /assets 400). A status_code_conformance failure on an
operation NOT in the allowlist is treated as a regression and fails the run.
This keeps the gate green today (critical checks clean; status gaps are
allowlisted) while catching any future DTO-shape break or newly-broken endpoint.

Usage
-----
  python3 scripts/schemathesis_check.py [options]

  --port PORT          port for the throwaway server (default 8099)
  --max-examples N     schemathesis --max-examples (default 1)
  --spec PATH          OpenAPI spec (default open-api/immich-openapi-specs.json)
  --bin PATH           reuse an existing immich-go binary (skip build)
  --report-dir DIR     where to drop junit.xml + text (default reports/schemathesis/regression)
  --no-server          assume a server is already running; use --base-url/--token
  --base-url URL       base url when --no-server (default http://localhost:8099/api)
  --token TOKEN        bearer token when --no-server

Prereqs: go (to build), python3 + `schemathesis` (pip install schemathesis==4.24.3).
"""
import argparse
import glob
import json
import os
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
SPEC_DEFAULT = os.path.join(ROOT, "open-api", "immich-openapi-specs.json")
ALLOWLIST = os.path.join(HERE, "schemathesis-allowlist.txt")
SCHEMA_THESIS_VER = "4.24.3"
ADMIN_EMAIL = "admin@immich.app"
ADMIN_PASS = "password"


def log(msg, *a):
    if a:
        try:
            msg = msg % a
        except Exception:
            pass
    print("[schemathesis_check]", msg, flush=True)


def ensure_schemathesis():
    try:
        import schemathesis  # noqa: F401
        return
    except Exception:
        log("schemathesis not importable; installing schemathesis==%s", SCHEMA_THESIS_VER)
        subprocess.check_call(
            [sys.executable, "-m", "pip", "install", "--quiet", f"schemathesis=={SCHEMA_THESIS_VER}"]
        )


def build_binary(dest):
    env = dict(os.environ)
    env["CGO_ENABLED"] = "0"
    log("building immich-go (CGO_ENABLED=0) -> %s", dest)
    subprocess.check_call(["go", "build", "-o", dest, "."], cwd=ROOT, env=env)
    return dest


def start_server(bin_path, port, workdir):
    os.makedirs(workdir, exist_ok=True)
    db = os.path.join(workdir, "immich.db")
    env = dict(os.environ)
    env["IMMICH_PORT"] = str(port)
    env["IMMICH_DB"] = db
    env["IMMICH_RESOURCE"] = os.path.join(workdir, "resources")
    env["IMMICH_COMPAT_VERSION"] = "3.1.0"
    log("starting server on :%d (db=%s)", port, db)
    p = subprocess.Popen(
        [bin_path], cwd=workdir, env=env,
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    url = f"http://127.0.0.1:{port}/api/server/ping"
    for _ in range(80):
        if p.poll() is not None:
            raise RuntimeError("server exited early (code %s)" % p.returncode)
        try:
            with urllib.request.urlopen(url, timeout=2) as r:
                if r.status == 200:
                    log("server ready on :%d", port)
                    return p
        except Exception:
            pass
        time.sleep(0.5)
    p.terminate()
    raise RuntimeError("server did not become ready in time")


def login(port):
    url = f"http://127.0.0.1:{port}/api/auth/login"
    data = json.dumps({"email": ADMIN_EMAIL, "password": ADMIN_PASS}).encode()
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=15) as r:
        body = json.load(r)
    tok = body.get("accessToken") or body.get("token")
    if not tok:
        raise RuntimeError("login returned no token: %r" % body)
    return tok


def run_schemathesis(spec, port, token, report_dir, max_examples):
    os.makedirs(report_dir, exist_ok=True)
    base = f"http://localhost:{port}/api"
    cmd = [
        "schemathesis", "run", spec,
        "--url", base,
        "-H", f"Authorization: Bearer {token}",
        "--max-examples", str(max_examples),
        "--workers", "1",
        "--phases", "examples",
        "--suppress-health-check", "all",
        "-c", "not_a_server_error",
        "-c", "status_code_conformance",
        "-c", "response_schema_conformance",
        "-c", "content_type_conformance",
        "--report", "junit",
        "--report-dir", report_dir,
    ]
    log("running schemathesis: %s", " ".join(cmd))
    # Ignore schemathesis' own exit code; we gate on classified failures.
    subprocess.run(cmd, check=False)
    # Schemathesis names the report junit-<timestamp>.xml; pick the newest.
    candidates = sorted(glob.glob(os.path.join(report_dir, "junit*.xml")), key=os.path.getmtime)
    return candidates[-1] if candidates else None


def classify(junit_path, allowlist):
    tree = ET.parse(junit_path)
    root = tree.getroot()
    critical = {"response_schema": 0, "content_type": 0, "server_error": 0}
    status_allowed = 0
    status_unallowed = []
    for tc in root.iter("testcase"):
        fail = tc.find("failure")
        if fail is None:
            continue
        text = (fail.text or "") + (fail.get("message") or "")
        tl = text.lower()
        if "response violates schema" in tl:
            critical["response_schema"] += 1
        elif "content type" in tl:
            critical["content_type"] += 1
        elif "server error" in tl:
            critical["server_error"] += 1
        elif "undocumented http status code" in tl:
            name = tc.get("name")
            if name in allowlist:
                status_allowed += 1
            else:
                status_unallowed.append(name)
        else:
            # Unknown failure shape — surface it as critical so it is never ignored.
            critical["response_schema"] += 1
            log("WARN unclassified failure on %s: %.120s", tc.get("name"), text)
    return critical, status_allowed, status_unallowed


def load_allowlist():
    if not os.path.exists(ALLOWLIST):
        log("no allowlist at %s", ALLOWLIST)
        return set()
    out = set()
    with open(ALLOWLIST) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            out.add(line)
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=8099)
    ap.add_argument("--max-examples", type=int, default=1)
    ap.add_argument("--spec", default=SPEC_DEFAULT)
    ap.add_argument("--bin", default=None)
    ap.add_argument("--report-dir", default=os.path.join(ROOT, "reports", "schemathesis", "regression"))
    ap.add_argument("--no-server", action="store_true")
    ap.add_argument("--base-url", default=None)
    ap.add_argument("--token", default=None)
    args = ap.parse_args()

    tmp = os.path.join(ROOT, ".schemathesis-tmp")
    os.makedirs(tmp, exist_ok=True)
    proc = None
    try:
        if args.no_server:
            base = args.base_url or f"http://localhost:{args.port}/api"
            token = args.token or os.environ.get("SCHEMATHESIS_TOKEN")
            if not token:
                raise SystemExit("ERROR: --no-server requires --token (or SCHEMATHESIS_TOKEN)")
            log("using external server %s", base)
        else:
            ensure_schemathesis()
            bin_path = args.bin or os.path.join(tmp, "immich-go")
            if not args.bin:
                build_binary(bin_path)
            proc = start_server(bin_path, args.port, os.path.join(tmp, "data"))
            token = login(args.port)

        junit = run_schemathesis(args.spec, args.port, token, args.report_dir, args.max_examples)
        if not os.path.exists(junit):
            raise SystemExit("ERROR: schemathesis produced no junit report at %s" % junit)

        allow = load_allowlist()
        crit, s_allowed, s_unallowed = classify(junit, allow)
        log("critical failures (must be 0): %s", crit)
        log("status_code gaps allowed by allowlist: %d", s_allowed)
        if s_unallowed:
            log("status_code gaps NOT in allowlist (REGRESSION): %s", s_unallowed)

        regressed = any(v > 0 for v in crit.values()) or bool(s_unallowed)
        if regressed:
            log("RESULT: FAIL — contract regression detected")
            return 1
        log("RESULT: PASS — no contract regressions (critical checks clean; "
            "%d status_code gaps are allowlisted)", s_allowed)
        return 0
    finally:
        if proc is not None:
            try:
                proc.send_signal(signal.SIGTERM)
                proc.wait(timeout=5)
            except Exception:
                proc.kill()


if __name__ == "__main__":
    sys.exit(main())
