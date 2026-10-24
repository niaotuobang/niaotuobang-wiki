package wiki

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB      *sql.DB
	DataDir string
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"/wiki.db?"+url.Values{"_pragma": []string{"foreign_keys(1)", "journal_mode(WAL)", "busy_timeout(5000)"}}.Encode())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrate(db, version, abs); err != nil {
		db.Close()
		return nil, err
	}
	os.Chmod(filepath.Join(abs, "wiki.db"), 0600)
	return &Store{db, abs}, nil
}

type querier interface {
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

func getArticle(q querier, id string) (Article, error) {
	a := Article{Tags: []Tag{}, SchemaVersion: 1}
	var doc string
	err := q.QueryRow(`SELECT a.id,a.title,a.document,a.plain_text,a.status,a.revision,a.occurred_at,a.date_precision,a.author_id,u.name,a.updated_by,e.name,a.created_at,a.updated_at,a.change_summary,a.deleted_at,coalesce(e.avatar_media_id,''),coalesce(u.avatar_media_id,'') FROM articles a JOIN users u ON u.id=a.author_id JOIN users e ON e.id=a.updated_by WHERE a.id=?`, id).Scan(&a.ID, &a.Title, &doc, &a.PlainText, &a.Status, &a.Revision, &a.OccurredAt, &a.DatePrecision, &a.AuthorID, &a.AuthorName, &a.UpdatedBy, &a.UpdatedByName, &a.CreatedAt, &a.UpdatedAt, &a.ChangeSummary, &a.DeletedAt, &a.UpdatedByAvatar, &a.AuthorAvatar)
	if errors.Is(err, sql.ErrNoRows) {
		return a, fail(404, "NOT_FOUND", "文章不存在或已放入回收站")
	}
	if err != nil {
		return a, err
	}
	if err = json.Unmarshal([]byte(doc), &a.Document); err != nil {
		return a, err
	}
	a.MediaKinds = mediaKinds(a.Document)
	rows, err := q.Query(`SELECT t.id,t.name,t.description,t.show_in_nav,t.nav_order FROM tags t JOIN article_tags at ON at.tag_id=t.id WHERE at.article_id=? ORDER BY t.nav_order,t.name`, id)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.ShowInNav, &t.NavOrder); err != nil {
			return a, err
		}
		a.Tags = append(a.Tags, t)
	}
	return a, rows.Err()
}
func canRead(a Article, u *User) bool {
	return a.DeletedAt == "" && (a.Status == "published" || u != nil && a.AuthorID == u.ID)
}
func (s *Store) saveArticle(id string, w WriteArticle, u User, create bool) (Article, error) {
	publish := w.Status != "draft"
	plain, err := validateWrite(w, publish)
	if err != nil {
		return Article{}, err
	}
	w.Document = canonicalDocument(w.Document)
	tx, err := s.DB.Begin()
	if err != nil {
		return Article{}, err
	}
	defer tx.Rollback()
	timestamp := now()
	if create {
		id = newID()
		doc, _ := json.Marshal(w.Document)
		_, err = tx.Exec(`INSERT INTO articles(id,title,document,plain_text,status,author_id,updated_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, id, strings.TrimSpace(w.Title), string(doc), plain, "draft", u.ID, u.ID, timestamp, timestamp)
		if err != nil {
			return Article{}, err
		}
	}
	current, err := getArticle(tx, id)
	if err != nil {
		return Article{}, err
	}
	if current.DeletedAt != "" || current.Status == "draft" && current.AuthorID != u.ID {
		return Article{}, fail(404, "NOT_FOUND", "文章不存在")
	}
	if w.ExpectedRevision != current.Revision {
		return Article{}, fail(409, "REVISION_CONFLICT", "文章已被其他人更新，请保留当前内容、比较差异后重新发布")
	}
	if !publish {
		b, _ := json.Marshal(w)
		_, err = tx.Exec(`INSERT INTO article_drafts(article_id,user_id,body,updated_at) VALUES(?,?,?,?) ON CONFLICT(article_id,user_id) DO UPDATE SET body=excluded.body,updated_at=excluded.updated_at`, id, u.ID, string(b), timestamp)
		if err != nil {
			return Article{}, err
		}
		if err = tx.Commit(); err != nil {
			return Article{}, err
		}
		return getArticle(s.DB, id)
	}
	media, links := references(w.Document)
	for _, mid := range media {
		var owner, state string
		err = tx.QueryRow(`SELECT owner_id,state FROM media WHERE id=?`, mid).Scan(&owner, &state)
		if err != nil || state != "ready" {
			return Article{}, fail(400, "MEDIA_NOT_READY", "图片或视频尚未完成上传")
		}
		if owner != u.ID {
			var n int
			tx.QueryRow(`SELECT count(*) FROM article_media WHERE article_id=? AND media_id=?`, id, mid).Scan(&n)
			if n == 0 {
				return Article{}, fail(403, "FORBIDDEN", "不能使用未授权的媒体")
			}
		}
	}
	for _, target := range links {
		a, e := getArticle(tx, target)
		if e != nil || !canRead(a, &u) || a.Status != "published" {
			return Article{}, fail(400, "VALIDATION_ERROR", "引用的文章不存在或尚未发布")
		}
	}
	if _, err = tx.Exec(`DELETE FROM article_tags WHERE article_id=?`, id); err != nil {
		return Article{}, err
	}
	seen := map[string]bool{}
	for _, tid := range w.TagIDs {
		if seen[tid] {
			continue
		}
		seen[tid] = true
		var exists int
		if err = tx.QueryRow(`SELECT count(*) FROM tags WHERE id=?`, tid).Scan(&exists); err != nil {
			return Article{}, err
		}
		if exists == 0 {
			return Article{}, fail(400, "VALIDATION_ERROR", "标签已被删除，请重新选择")
		}
		if _, err = tx.Exec(`INSERT INTO article_tags(article_id,tag_id) VALUES(?,?)`, id, tid); err != nil {
			return Article{}, err
		}
	}
	revision := current.Revision + 1
	doc, _ := json.Marshal(w.Document)
	_, err = tx.Exec(`UPDATE articles SET title=?,document=?,plain_text=?,status='published',revision=?,occurred_at=?,date_precision=?,updated_by=?,updated_at=?,change_summary=? WHERE id=?`, strings.TrimSpace(w.Title), string(doc), plain, revision, w.OccurredAt, w.DatePrecision, u.ID, timestamp, w.ChangeSummary, id)
	if err != nil {
		return Article{}, err
	}
	if _, err = tx.Exec(`DELETE FROM article_links WHERE source_id=?`, id); err != nil {
		return Article{}, err
	}
	contexts := linkContexts(w.Document)
	for _, target := range links {
		context := contexts[target]
		if _, err = tx.Exec(`INSERT INTO article_links(source_id,target_id,paragraph_id,context) VALUES(?,?,?,?)`, id, target, context.ParagraphID, context.Text); err != nil {
			return Article{}, err
		}
	}
	for _, mid := range media {
		if _, err = tx.Exec(`INSERT INTO article_media(article_id,revision,media_id) VALUES(?,?,?)`, id, revision, mid); err != nil {
			return Article{}, err
		}
	}
	a, err := getArticle(tx, id)
	if err != nil {
		return Article{}, err
	}
	snapshot, _ := json.Marshal(a)
	if _, err = tx.Exec(`INSERT INTO article_versions(article_id,revision,snapshot) VALUES(?,?,?)`, id, revision, string(snapshot)); err != nil {
		return Article{}, err
	}
	if _, err = tx.Exec(`DELETE FROM article_drafts WHERE article_id=? AND user_id=?`, id, u.ID); err != nil {
		return Article{}, err
	}
	if err = tx.Commit(); err != nil {
		return Article{}, err
	}
	return a, nil
}
func (s *Store) tags() ([]Tag, error) {
	rows, err := s.DB.Query(`SELECT t.id,t.name,t.description,t.show_in_nav,t.nav_order,(SELECT count(*) FROM article_tags at JOIN articles a ON a.id=at.article_id WHERE at.tag_id=t.id AND a.status='published' AND a.deleted_at='') FROM tags t ORDER BY t.nav_order,t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.ShowInNav, &t.NavOrder, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (s *Store) saveTag(id string, t Tag, create bool) (Tag, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || len([]rune(t.Name)) > 40 || len([]rune(t.Description)) > 300 {
		return t, fail(400, "VALIDATION_ERROR", "标签名称需要 1 至 40 字，说明最多 300 字")
	}
	if create {
		id = newID()
	}
	t.ID = id
	var err error
	if create {
		_, err = s.DB.Exec(`INSERT INTO tags(id,name,name_key,description,show_in_nav,nav_order) VALUES(?,?,?,?,?,?)`, id, t.Name, strings.ToLower(t.Name), t.Description, t.ShowInNav, t.NavOrder)
	} else {
		var result sql.Result
		result, err = s.DB.Exec(`UPDATE tags SET name=?,name_key=?,description=?,show_in_nav=?,nav_order=? WHERE id=?`, t.Name, strings.ToLower(t.Name), t.Description, t.ShowInNav, t.NavOrder, id)
		if err == nil {
			n, _ := result.RowsAffected()
			if n == 0 {
				return t, fail(404, "NOT_FOUND", "标签不存在")
			}
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return t, fail(409, "TAG_EXISTS", "这个标签已经存在")
		}
		return t, err
	}
	return t, nil
}
func (s *Store) list(v url.Values) (map[string]any, error) {
	where := `a.status='published' AND a.deleted_at=''`
	args := []any{}
	if q := strings.TrimSpace(v.Get("q")); q != "" {
		where += ` AND (instr(lower(a.title),lower(?))>0 OR instr(lower(a.plain_text),lower(?))>0)`
		args = append(args, q, q)
	}
	for _, tid := range v["tag"] {
		where += ` AND EXISTS(SELECT 1 FROM article_tags at WHERE at.article_id=a.id AND at.tag_id=?)`
		args = append(args, tid)
	}
	timeline := v.Get("timeline") == "1"
	if timeline {
		where += ` AND a.occurred_at<>''`
	}
	if v.Get("undated") == "1" {
		where += ` AND a.occurred_at=''`
	}
	if year := v.Get("year"); year != "" {
		if _, err := time.Parse("2006", year); err != nil {
			return nil, fail(400, "VALIDATION_ERROR", "年份无效")
		}
		where += ` AND substr(a.occurred_at,1,4)=?`
		args = append(args, year)
	}
	var total int
	if err := s.DB.QueryRow(`SELECT count(*) FROM articles a WHERE `+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	sortExpr := "a.updated_at"
	if timeline {
		sortExpr = "substr(a.occurred_at || '-01-01',1,10)"
	}
	prefix := ""
	from := "articles"
	if q := strings.TrimSpace(v.Get("q")); q != "" {
		prefix = `WITH ranked AS (SELECT *, CASE WHEN instr(lower(title),lower(?))>0 THEN 1 ELSE 0 END AS title_match FROM articles) `
		from = "ranked"
		args = append([]any{q}, args...)
		sortExpr = "CAST(a.title_match AS TEXT) || " + sortExpr
	}
	// Cursor is the immutable pair of the chosen sort key and article ID, encoded as JSON.
	if cursor := v.Get("cursor"); cursor != "" {
		var c []string
		if json.Unmarshal([]byte(cursor), &c) != nil || len(c) != 2 {
			return nil, fail(400, "VALIDATION_ERROR", "分页游标无效")
		}
		where += ` AND (` + sortExpr + `<? OR (` + sortExpr + `=? AND a.id<?))`
		args = append(args, c[0], c[0], c[1])
	}
	limit := 20
	fmt.Sscan(v.Get("limit"), &limit)
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	order := sortExpr + ` DESC,a.id DESC`
	rows, err := s.DB.Query(prefix+`SELECT a.id, `+sortExpr+` FROM `+from+` a WHERE `+where+` ORDER BY `+order+` LIMIT ?`, append(args, limit+1)...)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	keys := map[string]string{}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
		keys[id] = key
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	more := len(ids) > limit
	if more {
		ids = ids[:limit]
	}
	items := []Article{}
	for _, id := range ids {
		a, err := getArticle(s.DB, id)
		if err != nil {
			return nil, err
		}
		a.Document = Node{}
		runes := []rune(a.PlainText)
		start := 0
		if q := strings.TrimSpace(v.Get("q")); q != "" {
			if index := strings.Index(strings.ToLower(a.PlainText), strings.ToLower(q)); index >= 0 {
				start = len([]rune(a.PlainText[:index])) - 45
				if start < 0 {
					start = 0
				}
			}
		}
		end := start + 180
		if end > len(runes) {
			end = len(runes)
		}
		a.PlainText = string(runes[start:end])
		if start > 0 {
			a.PlainText = "…" + a.PlainText
		}
		if end < len(runes) {
			a.PlainText += "…"
		}
		items = append(items, a)
	}
	cursor := ""
	if more && len(items) > 0 {
		last := items[len(items)-1]
		b, _ := json.Marshal([]string{keys[last.ID], last.ID})
		cursor = string(b)
	}
	return map[string]any{"items": items, "total": total, "next_cursor": cursor}, nil
}
