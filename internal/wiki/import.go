package wiki

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type legacyID string

func (id *legacyID) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if len(b) > 0 && b[0] == '"' {
		if e := json.Unmarshal(b, &s); e != nil {
			return e
		}
	} else {
		s = string(b)
	}
	if !safeID.MatchString(s) {
		return fmt.Errorf("unsupported legacy identifier")
	}
	*id = legacyID(s)
	return nil
}

type legacyRevision struct {
	Title         string   `json:"title"`
	Body          string   `json:"body"`
	TopicID       legacyID `json:"topic_id"`
	UserID        legacyID `json:"user_id"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	ChangeSummary string   `json:"change_summary"`
}
type legacyArticle struct {
	ID legacyID `json:"id"`
	legacyRevision
	History []legacyRevision `json:"history"`
}
type legacyTopic struct {
	ID          legacyID `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
}
type legacyUser struct {
	ID   legacyID `json:"id"`
	Name string   `json:"name"`
}
type legacyMedia struct {
	URL         string `json:"url"`
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
}
type legacyBundle struct {
	Articles []legacyArticle `json:"articles"`
	Topics   []legacyTopic   `json:"topics"`
	Users    []legacyUser    `json:"users"`
	Media    []legacyMedia   `json:"media"`
}
type ImportReport struct {
	Articles int      `json:"articles"`
	Versions int      `json:"versions"`
	Media    int      `json:"media"`
	Skipped  int      `json:"skipped"`
	Applied  bool     `json:"applied"`
	Warnings []string `json:"warnings"`
}
type importedMedia struct {
	Media
	Path string
}

var legacyArticleLink = regexp.MustCompile(`#/?article/(\d+)`)

func legacyTime(s string) (string, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05.000Z"} {
		if t, e := time.Parse(layout, s); e == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("missing or invalid legacy timestamp")
}
func (a *App) ImportMatterwiki(input string, apply bool) (report ImportReport, err error) {
	report.Warnings = []string{}
	raw, err := os.ReadFile(input)
	if err != nil {
		return report, err
	}
	if len(raw) > 64<<20 {
		return report, fmt.Errorf("import bundle exceeds 64 MiB")
	}
	var bundle legacyBundle
	if err = json.Unmarshal(raw, &bundle); err != nil {
		return report, err
	}
	if len(bundle.Articles) == 0 {
		return report, fmt.Errorf("bundle needs an articles array")
	}
	report.Articles = len(bundle.Articles)
	report.Media = len(bundle.Media)
	prepared := map[string][]Article{}
	topics := map[legacyID]Tag{}
	users := map[legacyID]User{}
	media := map[string]importedMedia{}
	var admin User
	if err = a.store.DB.QueryRow(`SELECT id,name,login,role FROM users WHERE role='admin' LIMIT 1`).Scan(&admin.ID, &admin.Name, &admin.Login, &admin.Role); err != nil {
		return report, fmt.Errorf("create the first administrator before importing")
	}
	for _, u := range bundle.Users {
		if u.ID == "" {
			return report, fmt.Errorf("legacy user has no ID")
		}
		users[u.ID] = User{ID: "legacy-user-" + string(u.ID), Name: u.Name, Login: "legacy-user-" + string(u.ID), Role: "viewer"}
	}
	author := func(id legacyID) User {
		if id == "" {
			return admin
		}
		if u, ok := users[id]; ok {
			return u
		}
		u := User{ID: "legacy-user-" + string(id), Name: "旧站成员 " + string(id), Login: "legacy-user-" + string(id), Role: "viewer"}
		users[id] = u
		return u
	}
	for _, topic := range bundle.Topics {
		if topic.ID == "" || strings.TrimSpace(topic.Name) == "" {
			return report, fmt.Errorf("legacy topic needs ID and name")
		}
		t := Tag{ID: "legacy-topic-" + string(topic.ID), Name: strings.TrimSpace(topic.Name), Description: topic.Description, ShowInNav: true, NavOrder: len(topics)}
		var existing string
		e := a.store.DB.QueryRow(`SELECT id FROM tags WHERE name_key=?`, strings.ToLower(t.Name)).Scan(&existing)
		if e == nil {
			t.ID = existing
		} else if e != sql.ErrNoRows {
			return report, e
		}
		topics[topic.ID] = t
	}
	base, err := filepath.EvalSymlinks(filepath.Dir(input))
	if err != nil {
		return report, err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return report, err
	}
	for _, m := range bundle.Media {
		if m.URL == "" || filepath.IsAbs(m.Path) {
			return report, fmt.Errorf("media paths must be relative to bundle")
		}
		path, e := filepath.EvalSymlinks(filepath.Join(base, m.Path))
		if e != nil {
			return report, e
		}
		relative, e := filepath.Rel(base, path)
		if e != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return report, fmt.Errorf("media path escapes bundle directory")
		}
		info, e := os.Stat(path)
		if e != nil {
			return report, e
		}
		limit, ok := mediaTypes[m.ContentType]
		if !ok || info.Size() <= 0 || info.Size() > limit {
			return report, fmt.Errorf("unsupported import media type or size")
		}
		f, e := os.Open(path)
		if e != nil {
			return report, e
		}
		prefix, e := io.ReadAll(io.LimitReader(f, 65536))
		f.Close()
		if e != nil {
			return report, e
		}
		if e = inspectMedia(prefix, m.ContentType); e != nil {
			return report, e
		}
		h := sha256.Sum256([]byte(m.URL))
		id := "legacy-media-" + hex.EncodeToString(h[:12])
		media[m.URL] = importedMedia{Media: Media{ID: id, Owner: admin.ID, Name: filepath.Base(m.Path), ContentType: m.ContentType, Size: info.Size(), State: "ready", Key: "media/" + newID(), CreatedAt: now()}, Path: path}
	}
	seen := map[legacyID]bool{}
	for _, article := range bundle.Articles {
		if article.ID == "" || seen[article.ID] {
			return report, fmt.Errorf("missing or duplicate article ID")
		}
		seen[article.ID] = true
		id := "legacy-" + string(article.ID)
		var exists int
		if err = a.store.DB.QueryRow(`SELECT count(*) FROM import_map WHERE source='matterwiki' AND legacy_id=?`, string(article.ID)).Scan(&exists); err != nil {
			return report, err
		}
		if exists > 0 {
			report.Skipped++
			continue
		}
		created, e := legacyTime(article.CreatedAt)
		if e != nil {
			return report, fmt.Errorf("article %s: %w", article.ID, e)
		}
		history := append([]legacyRevision{}, article.History...)
		sort.SliceStable(history, func(i, j int) bool { return history[i].CreatedAt < history[j].CreatedAt })
		history = append(history, article.legacyRevision)
		originalAuthor := author(article.UserID)
		for index, revision := range history {
			if revision.Title == "" {
				revision.Title = article.Title
			}
			if revision.TopicID == "" {
				revision.TopicID = article.TopicID
			}
			if revision.UserID == "" {
				revision.UserID = article.UserID
			}
			user := author(revision.UserID)
			timestamp := revision.UpdatedAt
			if timestamp == "" {
				timestamp = revision.CreatedAt
			}
			updated, e := legacyTime(timestamp)
			if e != nil {
				return report, fmt.Errorf("article %s history timestamp: %w", article.ID, e)
			}
			doc, warnings, e := convertLegacyBody(revision.Body, media)
			if e != nil {
				return report, fmt.Errorf("article %s: %w", article.ID, e)
			}
			for _, w := range warnings {
				report.Warnings = append(report.Warnings, "article "+string(article.ID)+": "+w)
			}
			doc = canonicalDocument(doc)
			plain, e := validateWrite(WriteArticle{Title: revision.Title, Document: doc, Status: "published"}, true)
			if e != nil {
				return report, e
			}
			tags := []Tag{}
			if revision.TopicID != "" {
				tag, ok := topics[revision.TopicID]
				if !ok {
					return report, fmt.Errorf("article %s references a missing topic", article.ID)
				}
				tags = append(tags, tag)
			}
			prepared[id] = append(prepared[id], Article{ID: id, Title: revision.Title, Document: doc, SchemaVersion: 1, PlainText: plain, Tags: tags, Status: "published", Revision: index + 1, AuthorID: originalAuthor.ID, AuthorName: originalAuthor.Name, UpdatedBy: user.ID, UpdatedByName: user.Name, CreatedAt: created, UpdatedAt: updated, ChangeSummary: revision.ChangeSummary})
			report.Versions++
		}
	}
	// Missing internal targets and media never become silently broken references.
	for _, versions := range prepared {
		for _, v := range versions {
			_, links := references(v.Document)
			for _, target := range links {
				if _, ok := prepared[target]; !ok {
					var count int
					if err = a.store.DB.QueryRow(`SELECT count(*) FROM articles WHERE id=? AND status='published' AND deleted_at=''`, target).Scan(&count); err != nil {
						return report, err
					}
					if count == 0 {
						return report, fmt.Errorf("import references an article absent from the bundle")
					}
				}
			}
		}
	}
	if apply && len(report.Warnings) > 0 {
		return report, fmt.Errorf("unsupported HTML requires review; raw input remains unchanged")
	}
	if !apply {
		return report, nil
	}
	a.uploadMu.Lock()
	defer a.uploadMu.Unlock()
	private := filepath.Join(a.store.DataDir, "imports")
	if err = os.MkdirAll(private, 0700); err != nil {
		return report, err
	}
	rawHash := sha256.Sum256(raw)
	if err = os.WriteFile(filepath.Join(private, "matterwiki-"+hex.EncodeToString(rawHash[:])+".json"), raw, 0600); err != nil {
		return report, err
	}
	stagedMedia, cleanup, err := a.stageImportedMedia(media)
	if err != nil {
		cleanup()
		return report, err
	}
	committed := false
	defer func() {
		if !committed {
			cleanup()
		}
	}()

	tx, err := a.store.DB.Begin()
	if err != nil {
		return report, err
	}
	defer tx.Rollback()
	for _, u := range users {
		if u.Name == "" {
			u.Name = "旧站成员"
		}
		if _, err = tx.Exec(`INSERT INTO users(id,name,login,password_hash,role) VALUES(?,?,?,?,'viewer') ON CONFLICT(id) DO NOTHING`, u.ID, u.Name, u.Login, []byte{}); err != nil {
			return report, err
		}
	}
	for _, t := range topics {
		if _, err = tx.Exec(`INSERT INTO tags(id,name,name_key,description,show_in_nav,nav_order) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, t.ID, t.Name, strings.ToLower(t.Name), t.Description, t.ShowInNav, t.NavOrder); err != nil {
			return report, err
		}
	}
	for _, m := range stagedMedia {
		if _, err = tx.Exec(`INSERT INTO media(id,owner_id,name,content_type,size,state,object_key,created_at,checksum) VALUES(?,?,?,?,?,'ready',?,?,?)`, m.ID, m.Owner, m.Name, m.ContentType, m.Size, m.Key, m.CreatedAt, m.Checksum); err != nil {
			return report, err
		}
	}
	for id, versions := range prepared {
		current := versions[len(versions)-1]
		doc, _ := json.Marshal(current.Document)
		if _, err = tx.Exec(`INSERT INTO articles(id,title,document,plain_text,status,revision,author_id,updated_by,created_at,updated_at,change_summary) VALUES(?,?,?,?,'published',?,?,?,?,?,?)`, id, current.Title, string(doc), current.PlainText, current.Revision, current.AuthorID, current.UpdatedBy, current.CreatedAt, current.UpdatedAt, current.ChangeSummary); err != nil {
			return report, err
		}
		for _, tag := range current.Tags {
			if _, err = tx.Exec(`INSERT INTO article_tags(article_id,tag_id) VALUES(?,?)`, id, tag.ID); err != nil {
				return report, err
			}
		}
		for _, v := range versions {
			snapshot, _ := json.Marshal(v)
			if _, err = tx.Exec(`INSERT INTO article_versions(article_id,revision,snapshot) VALUES(?,?,?)`, id, v.Revision, string(snapshot)); err != nil {
				return report, err
			}
			mids, _ := references(v.Document)
			for _, mid := range mids {
				if _, err = tx.Exec(`INSERT INTO article_media(article_id,revision,media_id) VALUES(?,?,?)`, id, v.Revision, mid); err != nil {
					return report, err
				}
			}
		}
		_, links := references(current.Document)
		for _, target := range links {
			context := linkContexts(current.Document)[target]
			if _, err = tx.Exec(`INSERT INTO article_links(source_id,target_id,paragraph_id,context) VALUES(?,?,?,?)`, id, target, context.ParagraphID, context.Text); err != nil {
				return report, err
			}
		}
		if _, err = tx.Exec(`INSERT INTO import_map(source,legacy_id,article_id) VALUES('matterwiki',?,?)`, strings.TrimPrefix(id, "legacy-"), id); err != nil {
			return report, err
		}
	}
	if err = tx.Commit(); err != nil {
		return report, err
	}
	report.Applied = true
	committed = true
	b, _ := json.MarshalIndent(report, "", "  ")
	err = os.WriteFile(filepath.Join(private, "last-report.json"), b, 0600)
	return report, err
}

func convertLegacyBody(body string, media map[string]importedMedia) (Node, []string, error) {
	if !strings.Contains(body, "<") {
		return document(strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")...), nil, nil
	}
	root, e := html.Parse(strings.NewReader(body))
	if e != nil {
		return Node{}, nil, e
	}
	warnings := []string{}
	var conversionError error
	get := func(n *html.Node, key string) string {
		for _, a := range n.Attr {
			if a.Key == key {
				return a.Val
			}
		}
		return ""
	}
	var inline func(*html.Node, []Mark) []Node
	inline = func(n *html.Node, marks []Mark) []Node {
		if n.Type == html.TextNode {
			return []Node{{Type: "text", Text: n.Data, Marks: marks}}
		}
		if n.Type != html.ElementNode {
			return nil
		}
		tag := n.Data
		if tag == "script" || tag == "style" {
			warnings = append(warnings, "discarded unsafe "+tag)
			return nil
		}
		if tag == "img" || tag == "video" {
			src := get(n, "src")
			if tag == "video" && src == "" {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Data == "source" {
						src = get(c, "src")
						break
					}
				}
			}
			m, ok := media[src]
			if !ok {
				conversionError = fmt.Errorf("media needs a bundled local file")
				return nil
			}
			typ := "image"
			if tag == "video" {
				typ = "video"
			}
			return []Node{{Type: typ, Attrs: map[string]any{"mediaId": m.ID, "alt": get(n, "alt"), "title": get(n, "title")}}}
		}
		if tag == "br" {
			return []Node{{Type: "hardBreak"}}
		}
		allowed := map[string]bool{"span": true, "font": true, "a": true, "strong": true, "b": true, "em": true, "i": true, "s": true, "del": true, "code": true, "u": true, "small": true}
		if !allowed[tag] {
			warnings = append(warnings, "unsupported element "+tag)
		}
		markType := map[string]string{"strong": "bold", "b": "bold", "em": "italic", "i": "italic", "s": "strike", "del": "strike", "code": "code", "u": "underline"}[tag]
		next := append([]Mark{}, marks...)
		if markType != "" {
			next = append(next, Mark{Type: markType})
		}
		if tag == "a" {
			href := get(n, "href")
			if match := legacyArticleLink.FindStringSubmatch(href); len(match) > 1 {
				href = "/articles/legacy-" + match[1]
			}
			if safeLink(href) {
				next = append(next, Mark{Type: "link", Attrs: map[string]any{"href": href}})
			} else {
				warnings = append(warnings, "unsupported link protocol")
			}
		}
		out := []Node{}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			out = append(out, inline(c, next)...)
		}
		return out
	}
	blockTags := map[string]bool{"html": true, "body": true, "div": true, "section": true, "p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "blockquote": true, "ul": true, "ol": true, "li": true, "pre": true, "hr": true, "table": true}
	var blocks func(*html.Node) []Node
	blocks = func(parent *html.Node) []Node {
		out := []Node{}
		pending := []Node{}
		flush := func() {
			if len(pending) > 0 {
				var text strings.Builder
				for _, n := range pending {
					text.WriteString(n.Text)
				}
				if strings.TrimSpace(text.String()) != "" {
					out = append(out, Node{Type: "paragraph", Content: pending})
				}
				pending = nil
			}
		}
		for n := parent.FirstChild; n != nil; n = n.NextSibling {
			if n.Type == html.ElementNode && (n.Data == "head" || n.Data == "script" || n.Data == "style") {
				if n.Data != "head" {
					warnings = append(warnings, "discarded unsafe "+n.Data)
				}
				continue
			}
			if n.Type == html.ElementNode && blockTags[n.Data] {
				flush()
				switch n.Data {
				case "p", "h1", "h2", "h3", "h4", "h5", "h6":
					parts := []Node{}
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						parts = append(parts, inline(c, nil)...)
					}
					content := []Node{}
					emit := func() {
						if len(content) == 0 {
							return
						}
						typ := "paragraph"
						attrs := map[string]any{}
						if strings.HasPrefix(n.Data, "h") {
							typ = "heading"
							level := int(n.Data[1] - '0')
							if level > 3 {
								level = 3
							}
							attrs["level"] = level
						}
						out = append(out, Node{Type: typ, Attrs: attrs, Content: content})
						content = nil
					}
					for _, part := range parts {
						if part.Type == "image" || part.Type == "video" {
							emit()
							out = append(out, part)
						} else {
							content = append(content, part)
						}
					}
					emit()
				case "ul", "ol", "li", "blockquote":
					typ := map[string]string{"ul": "bulletList", "ol": "orderedList", "li": "listItem", "blockquote": "blockquote"}[n.Data]
					out = append(out, Node{Type: typ, Content: blocks(n)})
				case "hr":
					out = append(out, Node{Type: "horizontalRule"})
				case "pre":
					var text strings.Builder
					var walk func(*html.Node)
					walk = func(x *html.Node) {
						if x.Type == html.TextNode {
							text.WriteString(x.Data)
						}
						for c := x.FirstChild; c != nil; c = c.NextSibling {
							walk(c)
						}
					}
					walk(n)
					out = append(out, Node{Type: "codeBlock", Content: []Node{{Type: "text", Text: text.String()}}})
				case "table":
					warnings = append(warnings, "table requires manual conversion")
				default:
					out = append(out, blocks(n)...)
				}
			} else {
				parts := inline(n, nil)
				for _, part := range parts {
					if part.Type == "image" || part.Type == "video" {
						flush()
						out = append(out, part)
					} else {
						pending = append(pending, part)
					}
				}
			}
		}
		flush()
		return out
	}
	doc := Node{Type: "doc", Content: blocks(root)}
	return doc, warnings, conversionError
}
