import json
from pathlib import Path
import sqlite3
import tempfile
import unittest
from export_matterwiki_sqlite import export


class ExportTests(unittest.TestCase):
    def test_source_unchanged_passwords_omitted_and_deleted_history_retained(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "source.sqlite"
            con = sqlite3.connect(source)
            con.executescript("""
                CREATE TABLE users(id,name,email,password,about,created_at,updated_at);
                CREATE TABLE topics(id,name,description);
                CREATE TABLE articles(id,title,body,created_at,updated_at,topic_id,user_id,what_changed);
                CREATE TABLE archives(id,article_id,title,body,updated_at,user_id,what_changed);
                INSERT INTO users VALUES(1,'旧作者','old@example.test','DO_NOT_MIGRATE_PASSWORD','简介','2020-01-01','2020-01-02');
                INSERT INTO topics VALUES(1,'人物','说明');
                INSERT INTO articles VALUES(1,'现存','<div>原文</div>','2020-01-01 00:00:00','2021-01-01 00:00:00',1,1,'补充');
                INSERT INTO archives VALUES(4,1,'初稿','<div>初稿原文</div>','2020-01-01 00:00:00',1,'初稿');
                INSERT INTO archives VALUES(5,2,'已删除','<div>保留的历史</div>','2020-01-01 00:00:00',1,'记录');
            """)
            con.commit()
            con.close()
            original = source.read_bytes()
            output = Path(directory) / "private" / "export.json"
            report = export(source, output)
            self.assertEqual(source.read_bytes(), original)
            self.assertNotIn("DO_NOT_MIGRATE_PASSWORD", output.read_text())
            self.assertEqual(report["expected_versions"], 3)
            self.assertEqual(report["recovered_deleted_articles"], 1)
            recovered = json.loads(output.read_text())["articles"][1]
            self.assertEqual(recovered["archive_id"], 5)
            self.assertTrue(recovered["deletion_time_unknown"])
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(ValueError):
                export(source, source)


if __name__ == "__main__":
    unittest.main()
