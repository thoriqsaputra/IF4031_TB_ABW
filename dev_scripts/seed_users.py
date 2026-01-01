#!/usr/bin/env python3
import argparse
import os
import sys
from datetime import datetime, timezone
from typing import Dict, List, Optional, Sequence, Tuple

try:
    import bcrypt
except ImportError:  # pragma: no cover
    bcrypt = None

try:
    import psycopg2
    from psycopg2.extras import execute_values
except ImportError:  # pragma: no cover
    psycopg2 = None
    execute_values = None


# Edit these lists to manage seed data.
ROLES: List[Dict[str, object]] = [
    {"role_id": 0, "name": "user"},
    {"role_id": 1, "name": "staff"},
    
]

DEPARTMENTS: List[Dict[str, object]] = [
    {"department_id": 1, "name": "Field Operations"},
]

REPORT_CATEGORIES: List[Dict[str, object]] = [
    {"report_categories_id": 1, "name": "Infrastructure", "department_id": 1},
]

USERS: List[Dict[str, object]] = [
    {
        "user_id": 1,
        "email": "normal.user@example.com",
        "password": "Password123!",
        "name": "Normal User",
        "is_active": True,
        "created_at": "2025-01-01T00:00:00Z",
        "role_id": 0,
        "department_id": 0,
    },
    {
        "user_id": 2,
        "email": "staff.user@example.com",
        "password": "StaffPass123!",
        "name": "Staff User",
        "is_active": True,
        "created_at": "2025-01-01T00:00:00Z",
        "role_id": 1,
        "department_id": 1,
    },
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Seed users, roles, and departments directly via a PostgreSQL connection."
    )
    parser.add_argument(
        "--dsn",
        default=os.getenv("DB_DSN")
        or "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable",
        help="PostgreSQL DSN (default: local dev connection).",
    )
    parser.add_argument(
        "--cost",
        type=int,
        default=int(os.getenv("BCRYPT_COST", "10")),
        help="bcrypt cost rounds (default: 10).",
    )
    parser.add_argument("--dry-run", action="store_true", help="Print intended inserts without writing.")
    return parser.parse_args()


def parse_timestamp(raw: Optional[str], fallback: datetime) -> datetime:
    if not raw:
        return fallback
    value = raw.strip()
    if not value:
        return fallback
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(value)
    except ValueError as exc:
        raise ValueError(f"Invalid created_at timestamp: {raw}") from exc


def hash_password(password: str, cost: int) -> str:
    salt = bcrypt.gensalt(rounds=cost)
    return bcrypt.hashpw(password.encode("utf-8"), salt).decode("utf-8")


def normalize_users(users: Sequence[Dict[str, object]], cost: int) -> List[Tuple[object, ...]]:
    now = datetime.now(timezone.utc)
    normalized: List[Tuple[object, ...]] = []
    for user in users:
        created_at = parse_timestamp(user.get("created_at"), now)
        hashed = hash_password(str(user["password"]), cost)
        normalized.append(
            (
                user["user_id"],
                user["email"],
                hashed,
                user["name"],
                bool(user["is_active"]),
                created_at,
                user["role_id"],
                user["department_id"],
            )
        )
    return normalized


def validate_dependencies() -> Optional[str]:
    if bcrypt is None:
        return "Missing dependency: bcrypt. Install with `pip install bcrypt`."
    if psycopg2 is None or execute_values is None:
        return "Missing dependency: psycopg2. Install with `pip install psycopg2-binary`."
    return None


def insert_roles(cur, roles: Sequence[Dict[str, object]]) -> int:
    if not roles:
        return 0
    rows = [(r["role_id"], r["name"]) for r in roles]
    sql = """
        INSERT INTO roles (role_id, name)
        VALUES %s
        ON CONFLICT (role_id) DO UPDATE SET name = EXCLUDED.name
    """
    execute_values(cur, sql, rows)
    return len(rows)


def insert_departments(cur, departments: Sequence[Dict[str, object]]) -> int:
    if not departments:
        return 0
    rows = [(d["department_id"], d["name"]) for d in departments]
    sql = """
        INSERT INTO departments (department_id, name)
        VALUES %s
        ON CONFLICT (department_id) DO UPDATE SET name = EXCLUDED.name
    """
    execute_values(cur, sql, rows)
    return len(rows)


def insert_users(cur, users: List[Tuple[object, ...]]) -> int:
    if not users:
        return 0
    sql = """
        INSERT INTO users (
            user_id,
            email,
            password,
            name,
            is_active,
            created_at,
            role_id,
            department_id
        )
        VALUES %s
        ON CONFLICT (email) DO UPDATE SET
            password = EXCLUDED.password,
            name = EXCLUDED.name,
            is_active = EXCLUDED.is_active,
            created_at = EXCLUDED.created_at,
            role_id = EXCLUDED.role_id,
            department_id = EXCLUDED.department_id
    """
    execute_values(cur, sql, users)
    return len(users)


def insert_report_categories(cur, categories: Sequence[Dict[str, object]]) -> int:
    if not categories:
        return 0
    rows = [(c["report_categories_id"], c["name"], c["department_id"]) for c in categories]
    sql = """
        INSERT INTO report_categories (report_categories_id, name, department_id)
        VALUES %s
        ON CONFLICT (report_categories_id) DO UPDATE SET
            name = EXCLUDED.name,
            department_id = EXCLUDED.department_id
    """
    execute_values(cur, sql, rows)
    return len(rows)


def main() -> int:
    args = parse_args()

    if args.cost < 4 or args.cost > 31:
        print("bcrypt cost must be between 4 and 31.", file=sys.stderr)
        return 2

    dependency_error = validate_dependencies()
    if dependency_error:
        print(dependency_error, file=sys.stderr)
        return 2

    users = normalize_users(USERS, args.cost)

    if args.dry_run:
        print(f"Would insert roles: {len(ROLES)}")
        print(f"Would insert departments: {len(DEPARTMENTS)}")
        print(f"Would insert report categories: {len(REPORT_CATEGORIES)}")
        print(f"Would insert users: {len(users)}")
        return 0

    try:
        conn = psycopg2.connect(args.dsn)
    except Exception as exc:
        print(f"Failed to connect to database: {exc}", file=sys.stderr)
        return 2

    try:
        with conn:
            with conn.cursor() as cur:
                role_count = insert_roles(cur, ROLES)
                dept_count = insert_departments(cur, DEPARTMENTS)
                category_count = insert_report_categories(cur, REPORT_CATEGORIES)
                user_count = insert_users(cur, users)
        print(f"Inserted roles: {role_count}")
        print(f"Inserted departments: {dept_count}")
        print(f"Inserted report categories: {category_count}")
        print(f"Inserted users: {user_count}")
    except Exception as exc:
        print(f"Insert failed: {exc}", file=sys.stderr)
        return 1
    finally:
        conn.close()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
