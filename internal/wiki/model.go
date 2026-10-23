package wiki

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}
type Node struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}
type Tag struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ShowInNav   bool   `json:"show_in_nav"`
	NavOrder    int    `json:"nav_order"`
	Count       int    `json:"count"`
}
type Article struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Document      Node     `json:"document,omitzero"`
	SchemaVersion int      `json:"schema_version"`
	PlainText     string   `json:"plain_text"`
	Tags          []Tag    `json:"tags"`
	MediaKinds    []string `json:"media_kinds"`
	Status        string   `json:"status"`
	Revision      int      `json:"revision"`
	OccurredAt    string   `json:"occurred_at"`
	DatePrecision string   `json:"date_precision"`
	AuthorID      string   `json:"author_id"`
	AuthorName    string   `json:"author_name"`
	UpdatedBy     string   `json:"updated_by"`
	UpdatedByName string   `json:"updated_by_name"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	ChangeSummary string   `json:"change_summary"`
	DeletedAt     string   `json:"deleted_at,omitempty"`
}
type WriteArticle struct {
	Title            string   `json:"title"`
	Document         Node     `json:"document"`
	TagIDs           []string `json:"tag_ids"`
	OccurredAt       string   `json:"occurred_at"`
	DatePrecision    string   `json:"date_precision"`
	ChangeSummary    string   `json:"change_summary"`
	ExpectedRevision int      `json:"expected_revision"`
	Status           string   `json:"status"`
}
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Login string `json:"login"`
	Role  string `json:"role"`
}
type APIError struct {
	Status        int
	Code, Message string
}

func (e *APIError) Error() string                 { return e.Message }
func fail(status int, code, message string) error { return &APIError{status, code, message} }
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func now() string                    { return time.Now().UTC().Format(time.RFC3339Nano) }
func attr(n Node, key string) string { v, _ := n.Attrs[key].(string); return v }

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func validateDate(value, precision string) error {
	if value == "" && precision == "" {
		return nil
	}
	layout := map[string]string{"year": "2006", "month": "2006-01", "day": "2006-01-02"}[precision]
	d, err := time.Parse(layout, value)
	if layout == "" || err != nil || d.Year() < 1 || d.Format(layout) != value {
		return fail(400, "VALIDATION_ERROR", "发生时间与精度不匹配，请使用有效的年、月或日期")
	}
	return nil
}
func safeLink(href string) bool {
	if strings.HasPrefix(href, "/articles/") {
		return safeID.MatchString(strings.TrimPrefix(href, "/articles/"))
	}
	u, err := url.Parse(href)
	return err == nil && ((u.Scheme == "https" || u.Scheme == "http") && u.Host != "" || u.Scheme == "mailto" && u.Opaque != "")
}
func validateDocument(doc Node) (string, error) {
	if doc.Type != "doc" {
		return "", fail(400, "VALIDATION_ERROR", "正文格式无效")
	}
	var text strings.Builder
	count := 0
	allowed := map[string]bool{"doc": true, "paragraph": true, "text": true, "heading": true, "bulletList": true, "orderedList": true, "listItem": true, "blockquote": true, "hardBreak": true, "horizontalRule": true, "codeBlock": true, "image": true, "video": true}
	var walk func(Node, int) error
	walk = func(n Node, depth int) error {
		count++
		if depth > 20 || count > 20000 || !allowed[n.Type] {
			return fail(400, "VALIDATION_ERROR", "正文包含不支持的节点或层级过深")
		}
		blocks := map[string]bool{"paragraph": true, "heading": true, "bulletList": true, "orderedList": true, "blockquote": true, "codeBlock": true, "horizontalRule": true, "image": true, "video": true}
		if n.Type != "text" && n.Text != "" || n.Type != "text" && len(n.Marks) > 0 {
			return fail(400, "VALIDATION_ERROR", "文字和文字格式只能保存在文本节点")
		}
		if n.Type == "text" && n.Text == "" {
			return fail(400, "VALIDATION_ERROR", "空文本应使用空段落表示")
		}
		for i, child := range n.Content {
			valid := false
			switch n.Type {
			case "doc", "blockquote":
				valid = blocks[child.Type]
			case "paragraph", "heading":
				valid = child.Type == "text" || child.Type == "hardBreak"
			case "codeBlock":
				valid = child.Type == "text"
			case "bulletList", "orderedList":
				valid = child.Type == "listItem"
			case "listItem":
				valid = blocks[child.Type] && (i > 0 || child.Type == "paragraph")
			}
			if !valid {
				return fail(400, "VALIDATION_ERROR", "正文节点的嵌套结构不受支持")
			}
		}
		if n.Type == "text" {
			text.WriteString(n.Text)
		}
		if n.Type == "image" || n.Type == "video" {
			if !safeID.MatchString(attr(n, "mediaId")) {
				return fail(400, "VALIDATION_ERROR", "媒体引用无效")
			}
			text.WriteString(" ")
		}
		if n.Type == "heading" {
			v, ok := n.Attrs["level"].(float64)
			if !ok {
				if i, yes := n.Attrs["level"].(int); yes {
					v = float64(i)
					ok = true
				}
			}
			if !ok || v < 1 || v > 3 || v != float64(int(v)) {
				return fail(400, "VALIDATION_ERROR", "标题级别应为 1 至 3")
			}
		}
		for _, m := range n.Marks {
			switch m.Type {
			case "bold", "italic", "strike", "code", "underline":
			case "link":
				href, _ := m.Attrs["href"].(string)
				if !safeLink(href) {
					return fail(400, "VALIDATION_ERROR", "链接仅支持站内文章、HTTP、HTTPS 和邮箱")
				}
			default:
				return fail(400, "VALIDATION_ERROR", "不支持的文字格式")
			}
		}
		for _, child := range n.Content {
			if err := walk(child, depth+1); err != nil {
				return err
			}
		}
		if n.Type != "text" {
			text.WriteString("\n")
		}
		return nil
	}
	err := walk(doc, 0)
	if text.Len() > 1<<20 {
		return "", fail(413, "VALIDATION_ERROR", "正文过长")
	}
	return strings.TrimSpace(text.String()), err
}
func references(doc Node) (media, links []string) {
	seenMedia, seenLinks := map[string]bool{}, map[string]bool{}
	var walk func(Node)
	walk = func(n Node) {
		if n.Type == "image" || n.Type == "video" {
			id := attr(n, "mediaId")
			if !seenMedia[id] {
				media = append(media, id)
				seenMedia[id] = true
			}
		}
		for _, m := range n.Marks {
			if m.Type == "link" {
				h, _ := m.Attrs["href"].(string)
				if strings.HasPrefix(h, "/articles/") {
					id := strings.TrimPrefix(h, "/articles/")
					if !seenLinks[id] {
						links = append(links, id)
						seenLinks[id] = true
					}
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(doc)
	return
}
func validateWrite(w WriteArticle, publish bool) (string, error) {
	if w.ExpectedRevision < 0 || w.Status != "" && w.Status != "published" && w.Status != "draft" {
		return "", fail(400, "VALIDATION_ERROR", "发布状态或基础版本无效")
	}
	if len([]rune(strings.TrimSpace(w.Title))) > 200 {
		return "", fail(400, "VALIDATION_ERROR", "标题最多 200 个字符")
	}
	plain, err := validateDocument(w.Document)
	if err != nil {
		return "", err
	}
	media, links := references(w.Document)
	if len(media) > 100 || len(links) > 200 {
		return "", fail(400, "VALIDATION_ERROR", "一篇文章最多包含 100 个媒体和 200 个文章链接")
	}
	if publish && (strings.TrimSpace(w.Title) == "" || plain == "" && len(media) == 0) {
		return "", fail(400, "VALIDATION_ERROR", "请填写标题与正文")
	}
	if len(w.TagIDs) > 30 || len(w.ChangeSummary) > 1000 {
		return "", fail(400, "VALIDATION_ERROR", "标签或修改说明过长")
	}
	if len(w.OccurredAt) > 10 || w.DatePrecision != "" && w.DatePrecision != "year" && w.DatePrecision != "month" && w.DatePrecision != "day" {
		return "", fail(400, "VALIDATION_ERROR", "发生时间格式无效")
	}
	if publish {
		if err := validateDate(w.OccurredAt, w.DatePrecision); err != nil {
			return "", err
		}
	}
	return plain, nil
}

// Canonical nodes never persist browser-generated media URLs or arbitrary HTML attributes.
func canonicalDocument(n Node) Node {
	return canonicalNode(n, map[string]bool{})
}
func canonicalNode(n Node, seen map[string]bool) Node {
	n.Content = append([]Node(nil), n.Content...)
	n.Marks = append([]Mark(nil), n.Marks...)
	attrs := map[string]any{}
	for _, key := range []string{"id", "level", "mediaId", "alt", "title", "start"} {
		if value, ok := n.Attrs[key]; ok {
			attrs[key] = value
		}
	}
	if n.Type != "text" && n.Type != "doc" {
		if id, ok := attrs["id"].(string); !ok || !safeID.MatchString(id) || seen[id] {
			attrs["id"] = newID()
		}
		seen[attrs["id"].(string)] = true
	}
	n.Attrs = attrs
	for i, child := range n.Content {
		n.Content[i] = canonicalNode(child, seen)
	}
	for i, mark := range n.Marks {
		if mark.Type == "link" {
			n.Marks[i].Attrs = map[string]any{"href": mark.Attrs["href"]}
		} else {
			n.Marks[i].Attrs = nil
		}
	}
	return n
}

type linkContext struct{ ParagraphID, Text string }

func linkContexts(doc Node) map[string]linkContext {
	out := map[string]linkContext{}
	var text func(Node) string
	text = func(n Node) string {
		result := n.Text
		for _, c := range n.Content {
			result += text(c)
		}
		return result
	}
	var walk func(Node, linkContext)
	walk = func(n Node, context linkContext) {
		if n.Type == "paragraph" || n.Type == "heading" {
			context = linkContext{attr(n, "id"), text(n)}
		}
		for _, mark := range n.Marks {
			href, _ := mark.Attrs["href"].(string)
			if mark.Type == "link" && strings.HasPrefix(href, "/articles/") {
				target := strings.TrimPrefix(href, "/articles/")
				if _, ok := out[target]; !ok {
					out[target] = context
				}
			}
		}
		for _, child := range n.Content {
			walk(child, context)
		}
	}
	walk(doc, linkContext{})
	return out
}
func mediaKinds(doc Node) []string {
	set := map[string]bool{}
	var walk func(Node)
	walk = func(n Node) {
		if n.Type == "image" || n.Type == "video" {
			set[n.Type] = true
		}
		for _, child := range n.Content {
			walk(child)
		}
	}
	walk(doc)
	out := []string{}
	for _, kind := range []string{"image", "video"} {
		if set[kind] {
			out = append(out, kind)
		}
	}
	return out
}
func fromArticle(a Article) WriteArticle {
	w := WriteArticle{Title: a.Title, Document: a.Document, OccurredAt: a.OccurredAt, DatePrecision: a.DatePrecision, ChangeSummary: a.ChangeSummary, ExpectedRevision: a.Revision, Status: "published"}
	for _, t := range a.Tags {
		w.TagIDs = append(w.TagIDs, t.ID)
	}
	return w
}
func sqlPlaceholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }
func number(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
func contentError(err error) error { return fmt.Errorf("content: %w", err) }
