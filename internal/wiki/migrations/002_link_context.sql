ALTER TABLE article_links ADD COLUMN paragraph_id TEXT NOT NULL DEFAULT '';
ALTER TABLE article_links ADD COLUMN context TEXT NOT NULL DEFAULT '';
CREATE INDEX article_links_by_target ON article_links(target_id,source_id);
