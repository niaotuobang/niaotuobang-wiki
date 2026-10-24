#!/usr/bin/env python3
"""Read a Matterwiki SQLite snapshot without modifying it; never export passwords."""
import argparse
import collections
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import sqlite3


def timestamp(value):
    parsed = dt.datetime.fromisoformat(str(value).replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=dt.timezone.utc)
    return parsed.astimezone(dt.timezone.utc).isoformat(timespec="microseconds")


def export(source, output):
    source, output = Path(source).resolve(), Path(output).resolve()
    if source == output or output.exists():
        raise ValueError("output must be a new file separate from the source")
    con = sqlite3.connect(source.as_uri() + "?mode=ro", uri=True)
    con.row_factory = sqlite3.Row
    con.execute("PRAGMA query_only=ON")
    con.execute("BEGIN")
    if con.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
        raise ValueError("source database integrity check failed")
    users = [dict(r) for r in con.execute("SELECT id,name,email,about,created_at,updated_at FROM users ORDER BY id")]
    topics = [dict(r) for r in con.execute("SELECT id,name,description FROM topics ORDER BY id")]
    current = {r["id"]: dict(r) for r in con.execute("SELECT id,title,body,created_at,updated_at,topic_id,user_id,what_changed FROM articles ORDER BY id")}
    history = collections.defaultdict(list)
    archive_count = 0
    for row in con.execute("SELECT id,article_id,title,body,updated_at,user_id,what_changed FROM archives ORDER BY updated_at,id"):
        r = dict(row)
        history[r["article_id"]].append({
            "archive_id": r["id"], "title": r["title"], "body": r["body"],
            "user_id": r["user_id"], "updated_at": r["updated_at"],
            "created_at": timestamp(r["updated_at"]), "topic_unknown": True,
            "change_summary": r["what_changed"],
        })
        archive_count += 1
    con.close()
    articles = []
    archived_at = dt.datetime.now(dt.timezone.utc).isoformat()
    for id in sorted(set(current) | set(history)):
        revisions = history[id]
        if id in current:
            row = current[id]
            article = {k: row[k] for k in ["id", "title", "body", "created_at", "updated_at", "topic_id", "user_id"]}
            article["change_summary"] = row["what_changed"]
            article["history"] = revisions
        else:
            # The original current row no longer exists. Preserve every surviving
            # archive exactly once and expose its latest available content in trash.
            article = dict(revisions[-1])
            article.update(id=id, history=revisions[:-1], topic_id="",
                           created_at=revisions[0]["created_at"], deleted_at=archived_at,
                           deletion_time_unknown=True, creation_time_unknown=True)
        articles.append(article)
    bundle = {"users": users, "topics": topics, "articles": articles, "media": []}
    output.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as f:
        json.dump(bundle, f, ensure_ascii=False, indent=2)
        f.write("\n")
    emails = collections.Counter(u["email"].strip().lower() for u in users)
    return {"source_sha256": hashlib.sha256(source.read_bytes()).hexdigest(),
            "users": len(users), "topics": len(topics), "published_articles": len(current),
            "recovered_deleted_articles": len(articles)-len(current), "archives": archive_count,
            "expected_versions": archive_count+len(current),
            "shared_email_aliases": sum(n-1 for n in emails.values())}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    print(json.dumps(export(args.input, args.output), ensure_ascii=False))
