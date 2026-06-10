#!/usr/bin/env python3
"""
Refresh solscan.io cf_clearance cookie via cloudscraper.
Writes the cookie to .cookie_cache for the Go binary to consume.
Exit code: 0 on success, 1 on failure.
"""

import os
import sys
import cloudscraper
import json

CACHE_FILE = os.path.join(
    os.path.dirname(os.path.abspath(__file__)),
    ".cookie_cache",
)


def refresh() -> str | None:
    scraper = cloudscraper.create_scraper(
        browser={
            "browser": "chrome",
            "platform": "windows",
            "desktop": True,
        },
    )

    # Hit the Solscan landing page to get past Cloudflare
    resp = scraper.get(
        "https://solscan.io",
        timeout=30,
        headers={
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.9",
        },
    )

    if resp.status_code not in (200, 301, 302, 403):
        print(f"Unexpected status: {resp.status_code}", file=sys.stderr)
        return None

    cf = resp.cookies.get("cf_clearance")
    if not cf:
        # cloudscraper might have attached it to the session directly
        for cookie in scraper.cookies:
            if cookie.name == "cf_clearance":
                cf = cookie.value
                break

    if not cf:
        print("No cf_clearance cookie found", file=sys.stderr)
        return None

    return cf


def main():
    cf = refresh()
    if not cf:
        print("Failed to refresh cookie", file=sys.stderr)
        sys.exit(1)

    # Write to cache file (atomically via temp + rename)
    tmp = CACHE_FILE + ".tmp"
    with open(tmp, "w") as f:
        f.write(cf.strip())
    os.rename(tmp, CACHE_FILE)

    print(f"OK cf_clearance refreshed → {CACHE_FILE}")
    print(f"  cookie[:60] = {cf[:60]}...")


if __name__ == "__main__":
    main()