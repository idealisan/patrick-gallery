#!/usr/bin/env python3
"""Parse an Android uiautomator XML dump and print the center (x,y) of the
first node whose content-desc / text / class matches the given substring.

This is the reliable way to drive a Flutter app (e.g. the official immich
Android client) from the command line: do NOT guess tap coordinates from
screenshots. Instead `uiautomator dump`, locate the control by its
content-desc / text / class, and tap its center.

Usage:
    android_uicenter.py <ui.xml> <match> [class-substr]

Examples:
    android_uicenter.py ui.xml Next
    android_uicenter.py ui.xml "" android.widget.EditText
    android_uicenter.py ui.xml login android.widget.Button

Output: "<cx> <cy>  [<label>]"   where label is content-desc or text or class.
        or "NOTFOUND" when nothing matches.
"""
import sys
import re


def main():
    if len(sys.argv) < 3:
        print("usage: android_uicenter.py <ui.xml> <match> [class-substr]", file=sys.stderr)
        sys.exit(2)

    path, match = sys.argv[1], sys.argv[2]
    cls = sys.argv[3] if len(sys.argv) > 3 else None

    with open(path, encoding="utf-8", errors="replace") as fh:
        data = fh.read()

    for m in re.finditer(r"<node\b([^>]*)/>", data):
        attrs = m.group(1)
        cd = re.search(r'content-desc="([^"]*)"', attrs)
        tx = re.search(r'text="([^"]*)"', attrs)
        cl = re.search(r'class="([^"]*)"', attrs)
        cd_s = cd.group(1) if cd else ""
        tx_s = tx.group(1) if tx else ""
        cl_s = cl.group(1) if cl else ""

        if cls and cls not in cl_s:
            continue
        if match and match not in cd_s and match not in tx_s:
            continue

        b = re.search(r'bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"', attrs)
        if not b:
            continue

        x1, y1, x2, y2 = map(int, b.groups())
        cx, cy = (x1 + x2) // 2, (y1 + y2) // 2
        label = cd_s or tx_s or cl_s
        print(f"{cx} {cy}  [{label}]")
        return

    print("NOTFOUND")


if __name__ == "__main__":
    main()
