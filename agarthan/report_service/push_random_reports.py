#!/usr/bin/env python3
import argparse
import json
import os
import random
import time
import urllib.error
import urllib.request
from typing import List, Optional, Tuple


DEFAULT_SEVERITIES = ["low", "medium", "high", "critical"]
DEFAULT_LOCATIONS = [
    "5th Ave and Pine St",
    "Main St near the library",
    "Central Park entrance",
    "Station Road underpass",
    "Riverside bike lane",
    "Market district block C",
]
DEFAULT_TITLE_NOUNS = [
    "streetlight",
    "pothole",
    "traffic signal",
    "sidewalk",
    "drain",
    "crosswalk",
    "bus stop",
    "bridge",
    "tree",
]
DEFAULT_TITLE_ADJECTIVES = [
    "broken",
    "damaged",
    "blocked",
    "flooded",
    "missing",
    "unsafe",
    "flickering",
    "collapsed",
    "overgrown",
]
DEFAULT_DESCRIPTION_WORDS = [
    "resident",
    "reported",
    "issue",
    "causing",
    "danger",
    "traffic",
    "slow",
    "visibility",
    "hazard",
    "night",
    "morning",
    "urgent",
    "needs",
    "repair",
    "cleanup",
    "inspection",
    "area",
    "community",
    "safety",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Push randomly generated reports to the report service."
    )
    parser.add_argument(
        "-t",
        "--token",
        default=os.getenv("REPORT_JWT") or os.getenv("JWT_TOKEN"),
        help="JWT token for Authorization header (or set REPORT_JWT/JWT_TOKEN).",
    )
    parser.add_argument(
        "-u",
        "--base-url",
        default="http://localhost:3001",
        help="Base URL for the report service (default: http://localhost:3001).",
    )
    parser.add_argument(
        "-e",
        "--endpoint",
        default="/reports",
        help="Endpoint path to POST reports (default: /reports).",
    )
    parser.add_argument(
        "-n",
        "--count",
        type=int,
        default=10,
        help="Number of reports to send (default: 10).",
    )
    parser.add_argument(
        "--categories",
        default="1",
        help="Comma-separated report_categories_id values (default: 1).",
    )
    parser.add_argument(
        "--severities",
        default=",".join(DEFAULT_SEVERITIES),
        help="Comma-separated severities to choose from.",
    )
    parser.add_argument(
        "--public-rate",
        type=float,
        default=0.7,
        help="Probability of is_public being true (default: 0.7).",
    )
    parser.add_argument(
        "--anon-rate",
        type=float,
        default=0.2,
        help="Probability of is_anon being true (default: 0.2).",
    )
    parser.add_argument(
        "--sleep",
        type=float,
        default=0.0,
        help="Seconds to sleep between requests (default: 0).",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=10.0,
        help="Request timeout in seconds (default: 10).",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=None,
        help="Random seed for reproducible data.",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print payloads without sending requests.",
    )
    args = parser.parse_args()

    if not args.token:
        parser.error("JWT token is required via --token or REPORT_JWT/JWT_TOKEN.")

    if args.count <= 0:
        parser.error("--count must be greater than zero.")

    return args


def parse_int_list(raw: str, label: str) -> List[int]:
    values = []
    for part in raw.split(","):
        part = part.strip()
        if not part:
            continue
        try:
            values.append(int(part))
        except ValueError as exc:
            raise ValueError(f"Invalid {label} value: {part}") from exc
    if not values:
        raise ValueError(f"At least one {label} value is required.")
    return values


def random_sentence(word_bank: List[str], min_words: int, max_words: int) -> str:
    length = random.randint(min_words, max_words)
    words = random.choices(word_bank, k=length)
    sentence = " ".join(words).strip()
    return sentence[:1].upper() + sentence[1:] + "."


def random_title() -> str:
    adjective = random.choice(DEFAULT_TITLE_ADJECTIVES)
    noun = random.choice(DEFAULT_TITLE_NOUNS)
    return f"{adjective.capitalize()} {noun}"


def random_report(categories: List[int], severities: List[str]) -> dict:
    return {
        "title": random_title(),
        "description": random_sentence(DEFAULT_DESCRIPTION_WORDS, 8, 14),
        "location": random.choice(DEFAULT_LOCATIONS),
        "severity": random.choice(severities),
        "report_categories_id": random.choice(categories),
    }


def post_report(url: str, token: str, payload: dict, timeout: float) -> Tuple[bool, Optional[int], str]:
    body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=body,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
            return True, resp.status, raw
    except urllib.error.HTTPError as err:
        raw = err.read().decode("utf-8", errors="replace")
        return False, err.code, raw
    except urllib.error.URLError as err:
        return False, None, str(err)


def main() -> int:
    args = parse_args()

    if args.seed is not None:
        random.seed(args.seed)

    try:
        categories = parse_int_list(args.categories, "category")
        severities = [s.strip() for s in args.severities.split(",") if s.strip()]
    except ValueError as exc:
        print(f"Error: {exc}")
        return 2

    if not severities:
        print("Error: At least one severity is required.")
        return 2
    if not 0 <= args.public_rate <= 1:
        print("Error: --public-rate must be between 0 and 1.")
        return 2
    if not 0 <= args.anon_rate <= 1:
        print("Error: --anon-rate must be between 0 and 1.")
        return 2

    base_url = args.base_url.rstrip("/")
    endpoint = args.endpoint if args.endpoint.startswith("/") else f"/{args.endpoint}"
    url = f"{base_url}{endpoint}"

    successes = 0
    failures = 0

    for idx in range(1, args.count + 1):
        payload = random_report(categories, severities)
        payload["is_public"] = random.random() < args.public_rate
        payload["is_anon"] = random.random() < args.anon_rate

        if args.dry_run:
            print(f"[{idx}/{args.count}] DRY RUN {json.dumps(payload)}")
            continue

        ok, status, body = post_report(url, args.token, payload, args.timeout)
        if ok:
            successes += 1
            print(f"[{idx}/{args.count}] {status} queued")
        else:
            failures += 1
            status_label = status if status is not None else "error"
            print(f"[{idx}/{args.count}] {status_label} {body}")

        if args.sleep > 0 and idx < args.count:
            time.sleep(args.sleep)

    print(f"Done. Success: {successes}, Failed: {failures}")
    return 0 if failures == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
