import os
import sys
from urllib.parse import unquote, urlsplit


def configure_flyway():
    raw_url = os.environ.get("DATABASE_URL", "").strip()
    parsed = urlsplit(raw_url)
    if (
        parsed.scheme not in ("postgres", "postgresql")
        or not parsed.hostname
        or not parsed.path
        or parsed.fragment
    ):
        raise SystemExit("DATABASE_URL must be a PostgreSQL connection URL")

    try:
        port = parsed.port or 5432
    except ValueError:
        raise SystemExit("DATABASE_URL has an invalid port") from None

    host = parsed.hostname
    if ":" in host:
        host = f"[{host}]"
    jdbc_url = f"jdbc:postgresql://{host}:{port}{unquote(parsed.path)}"
    if parsed.query:
        jdbc_url += f"?{parsed.query}"
    os.environ["FLYWAY_URL"] = jdbc_url
    if parsed.username is not None:
        os.environ["FLYWAY_USER"] = unquote(parsed.username)
    if parsed.password is not None:
        os.environ["FLYWAY_PASSWORD"] = unquote(parsed.password)
    os.environ["FLYWAY_LOCATIONS"] = "filesystem:/flyway/sql"
    os.environ.setdefault("FLYWAY_CONNECT_RETRIES", "10")
    os.environ.pop("DATABASE_URL", None)


if __name__ == "__main__":
    configure_flyway()
    os.execv("/flyway/flyway", ["/flyway/flyway", *sys.argv[1:]])
