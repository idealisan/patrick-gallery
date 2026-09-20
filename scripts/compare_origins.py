#!/usr/bin/env python3
"""并排对比：原版 Immich vs immich-go 的 API 契约一致性。

对两个运行中的服务（原版 Immich 与 immich-go）分别跑 Schemathesis（同一份
官方 v3.1.0 OpenAPI 契约），解析各自的 JUnit 报告，按 endpoint 逐条对比：
  - origin 通过、go 未通过  -> GAP（immich-go 尚未覆盖/行为不一致）
  - 两边都通过              -> OK（immich-go 覆盖且与契约一致）
  - 两边都失败              -> BOTH_FAIL（多为该端点两边都未实现/契约歧义）

本脚本只做“对比报告”，不裁决通过/失败（裁决由 scripts/schemathesis_check.py 负责）。

前置：两个服务都已启动且可登录；本机已 `pip install schemathesis==4.27.5`。
典型用法（见 docs/SIDE_BY_SIDE.md）：
  # 原版 Immich 用 docker-compose.yml 起在 :2283，immich-go 起在 :8081
  python3 scripts/compare_origins.py \
      --origin-url http://localhost:2283/api --go-url http://localhost:8081/api
"""
import argparse
import os
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
SPEC = os.path.join(ROOT, "open-api", "immich-openapi-specs.json")
SCHEMA_THESIS_VER = "4.27.5"
ADMIN_EMAIL = "admin@immich.app"
ADMIN_PASS = "password"

CHECKS = ["not_a_server_error", "status_code_conformance",
          "response_schema_conformance", "content_type_conformance"]


def log(*a):
    print("[compare_origins]", *a, flush=True)


def ensure_schemathesis():
    try:
        import schemathesis  # noqa: F401
        return
    except Exception:
        log("installing schemathesis==%s", SCHEMA_THESIS_VER)
        subprocess.check_call([sys.executable, "-m", "pip", "install", "--quiet",
                               f"schemathesis=={SCHEMA_THESIS_VER}"])


def login(base):
    url = base.rstrip("/") + "/auth/login"
    data = __import__("json").dumps({"email": ADMIN_EMAIL, "password": ADMIN_PASS}).encode()
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=20) as r:
        return __import__("json").loads(r.read()).get("accessToken")


def run_one(base, token, spec, out_dir, max_examples):
    cmd = ["schemathesis", "run", spec, "--url", base.rstrip("/"),
           "-H", f"Authorization: Bearer {token}", "--max-examples", str(max_examples),
           "--workers", "1", "--phases", "examples", "--suppress-health-check", "all",
           *sum((["-c", c] for c in CHECKS), [])]
    log("schemathesis -> %s", base)
    subprocess.run(cmd, check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    import glob
    cands = sorted(glob.glob(os.path.join(out_dir, "junit*.xml")), key=os.path.getmtime)
    return cands[-1] if cands else None


def parse(junit_path):
    """Return dict: endpoint -> ('pass'|'skip'|'fail', [check-types])."""
    out = {}
    if not junit_path or not os.path.exists(junit_path):
        return out
    root = ET.parse(junit_path).getroot()
    for tc in root.iter("testcase"):
        name = tc.get("name")
        if tc.find("skipped") is not None:
            out[name] = ("skip", [])
            continue
        fail = tc.find("failure")
        if fail is None:
            out[name] = ("pass", [])
            continue
        text = (fail.text or "") + (fail.get("message") or "")
        tl = text.lower()
        kinds = []
        if "response violates schema" in tl:
            kinds.append("schema")
        elif "content type" in tl:
            kinds.append("content_type")
        elif "server error" in tl:
            kinds.append("5xx")
        elif "undocumented http status code" in tl:
            kinds.append("status_code")
        else:
            kinds.append("other")
        out[name] = ("fail", kinds)
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--origin-url", required=True)
    ap.add_argument("--go-url", required=True)
    ap.add_argument("--origin-token")
    ap.add_argument("--go-token")
    ap.add_argument("--spec", default=SPEC)
    ap.add_argument("--max-examples", type=int, default=1)
    args = ap.parse_args()

    ensure_schemathesis()
    tok_o = args.origin_token or login(args.origin_url)
    tok_g = args.go_token or login(args.go_url)

    with tempfile.TemporaryDirectory() as d:
        jo = run_one(args.origin_url, tok_o, args.spec, os.path.join(d, "o"), args.max_examples)
        jg = run_one(args.go_url, tok_g, args.spec, os.path.join(d, "g"), args.max_examples)
        origin = parse(jo)
        go = parse(jg)

    eps = sorted(set(origin) | set(go))
    log("endpoints seen — origin:%d  go:%d  union:%d", len(origin), len(go), len(eps))
    print(f"{'ENDPOINT':42} {'ORIGIN':8} {'IMMICH-GO':10} NOTE")
    print("-" * 90)
    gaps = both_fail = ok = 0
    for ep in eps:
        o = origin.get(ep, ("absent", []))
        g = go.get(ep, ("absent", []))
        if o[0] == "pass" and g[0] == "pass":
            note, ok = "OK", ok + 1
        elif o[0] == "pass" and g[0] != "pass":
            note, gaps = "GAP", gaps + 1
        elif o[0] != "pass" and g[0] == "pass":
            note = "GO-AHEAD"
        elif o[0] != "pass" and g[0] != "pass":
            note, both_fail = "BOTH_FAIL", both_fail + 1
        else:
            note = ""
        print(f"{ep:42} {o[0]:8} {g[0]:10} {note}")
    print("-" * 90)
    print(f"OK(两边都通过)={ok}  GAP(原版过/go未过)={gaps}  BOTH_FAIL={both_fail}")
    print("GAP = immich-go 尚未覆盖或行为不一致；BOTH_FAIL = 两边均未实现/契约歧义。")


if __name__ == "__main__":
    main()
