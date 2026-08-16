#!/usr/bin/env python3
"""Tap a UI control on the running Android emulator by matching its
content-desc or text substring. Robust to layout shifts (unlike hard-coded
coordinates). Usage:

    python3 scripts/android_tap.py <substring> [dump_path]

It dumps the current UI (via adb uiautomator), finds the first node whose
content-desc or text contains <substring>, computes its center, and taps it.
Prints the tapped (cx,cy) and the matched label, or exits non-zero if not
found.
"""
import subprocess
import sys
import xml.etree.ElementTree as ET

ADB = "adb"


def dump(path="/tmp/ui.xml"):
    subprocess.run([ADB, "shell", "uiautomator", "dump", "/sdcard/ui.xml"],
                   check=True, capture_output=True)
    subprocess.run([ADB, "pull", "/sdcard/ui.xml", path],
                   check=True, capture_output=True)
    return path


def center_of(bounds):
    # bounds like "[128,504][953,658]"
    nums = bounds.strip("[]").replace("][", ",").split(",")
    x1, y1, x2, y2 = map(int, nums)
    return (x1 + x2) // 2, (y1 + y2) // 2


def find(substr, path):
    tree = ET.parse(path)
    for n in tree.iter("node"):
        cd = n.get("content-desc", "") or ""
        txt = n.get("text", "") or ""
        if substr.lower() in cd.lower() or substr.lower() in txt.lower():
            b = n.get("bounds", "")
            if b:
                return center_of(b), (cd or txt)
    return None, None


def main():
    substr = sys.argv[1]
    path = sys.argv[2] if len(sys.argv) > 2 else "/tmp/ui.xml"
    dump(path)
    (cx, cy), label = find(substr, path)
    if cx is None:
        print(f"NOT FOUND: {substr!r}", file=sys.stderr)
        sys.exit(1)
    subprocess.run([ADB, "shell", "input", "tap", str(cx), str(cy)],
                   check=True, capture_output=True)
    print(f"{cx} {cy} [{label}]")


if __name__ == "__main__":
    main()
