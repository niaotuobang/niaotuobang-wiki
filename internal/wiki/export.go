package wiki

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) articleIDs() ([]string, error) {
	rows, e := a.store.DB.Query(`SELECT id FROM articles ORDER BY created_at,id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (a *App) exportRows(w io.Writer, query string) error {
	rows, e := a.store.DB.Query(query)
	if e != nil {
		return e
	}
	defer rows.Close()
	first := true
	for rows.Next() {
		var raw string
		if e = rows.Scan(&raw); e != nil {
			return e
		}
		if !json.Valid([]byte(raw)) {
			return fmt.Errorf("stored export record is invalid JSON")
		}
		if !first {
			if _, e = io.WriteString(w, ","); e != nil {
				return e
			}
		}
		first = false
		if _, e = io.WriteString(w, raw); e != nil {
			return e
		}
	}
	return rows.Err()
}
func (a *App) ExportJSON(w io.Writer) error {
	ids, e := a.articleIDs()
	if e != nil {
		return e
	}
	if _, e = io.WriteString(w, `{"schema_version":1,"articles":[`); e != nil {
		return e
	}
	encoder := json.NewEncoder(w)
	for i, id := range ids {
		if i > 0 {
			if _, e = io.WriteString(w, ","); e != nil {
				return e
			}
		}
		v, e := getArticle(a.store.DB, id)
		if e != nil {
			return e
		}
		if e = encoder.Encode(v); e != nil {
			return e
		}
	}
	tags, e := a.store.tags()
	if e != nil {
		return e
	}
	if _, e = io.WriteString(w, `],"tags":`); e != nil {
		return e
	}
	if e = encoder.Encode(tags); e != nil {
		return e
	}
	for _, field := range []struct{ name, query string }{
		{"versions", `SELECT snapshot FROM article_versions ORDER BY article_id,revision`},
		{"drafts", `SELECT json_object('article_id',article_id,'user_id',user_id,'body',json(body),'updated_at',updated_at) FROM article_drafts ORDER BY article_id,user_id`},
		{"media", `SELECT json_object('id',id,'owner_id',owner_id,'name',name,'content_type',content_type,'size',size,'state',state,'object_key',object_key,'created_at',created_at,'checksum',checksum) FROM media WHERE state='ready' ORDER BY id`},
		{"authors", `SELECT json_object('id',id,'name',name) FROM users ORDER BY id`},
	} {
		if _, e = io.WriteString(w, `,"`+field.name+`":[`); e != nil {
			return e
		}
		if e = a.exportRows(w, field.query); e != nil {
			return e
		}
		if _, e = io.WriteString(w, "]"); e != nil {
			return e
		}
	}
	_, e = io.WriteString(w, "}\n")
	return e
}
func (a *App) ExportFolder(output string) error {
	archive, e := a.Backup(output)
	if e != nil {
		return e
	}
	temp, e := os.MkdirTemp(a.store.DataDir, ".readable-export-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(temp)
	dir := filepath.Join(temp, "snapshot")
	if e = RestoreBackup(archive, dir); e != nil {
		return e
	}
	snapshot, e := Open(Config{DataDir: dir})
	if e != nil {
		return e
	}
	defer snapshot.Close()
	f, e := os.OpenFile(filepath.Join(output, "articles.json"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	e = snapshot.ExportJSON(f)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	md, e := os.OpenFile(filepath.Join(output, "articles.md"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer md.Close()
	ids, e := snapshot.articleIDs()
	if e != nil {
		return e
	}
	for _, id := range ids {
		v, e := getArticle(snapshot.store.DB, id)
		if e != nil {
			return e
		}
		if v.Status != "published" || v.DeletedAt != "" {
			continue
		}
		if _, e = fmt.Fprintf(md, "# %s\n\n%s\n\n---\n\n", v.Title, snapshot.markdown(v.Document)); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) markdown(n Node) string {
	if n.Type == "text" {
		text := strings.NewReplacer("\\", "\\\\", "*", "\\*", "[", "\\[", "]", "\\]").Replace(n.Text)
		for _, m := range n.Marks {
			switch m.Type {
			case "bold":
				text = "**" + text + "**"
			case "italic":
				text = "*" + text + "*"
			case "strike":
				text = "~~" + text + "~~"
			case "code":
				text = "`" + n.Text + "`"
			case "link":
				href, _ := m.Attrs["href"].(string)
				text = "[" + text + "](" + href + ")"
			}
		}
		return text
	}
	if n.Type == "image" || n.Type == "video" {
		m, e := a.media(attr(n, "mediaId"))
		if e != nil {
			return "[媒体缺失]"
		}
		prefix := "!"
		label := attr(n, "alt")
		if n.Type == "video" {
			prefix = ""
			label = attr(n, "title")
		}
		return prefix + "[" + label + "](" + m.Key + ")\n\n"
	}
	var text strings.Builder
	for _, child := range n.Content {
		text.WriteString(a.markdown(child))
	}
	body := text.String()
	switch n.Type {
	case "heading":
		return strings.Repeat("#", number(n.Attrs["level"])) + " " + body + "\n\n"
	case "paragraph":
		return body + "\n\n"
	case "hardBreak":
		return "  \n"
	case "horizontalRule":
		return "\n---\n\n"
	case "listItem":
		return "- " + strings.TrimSpace(body) + "\n"
	case "blockquote":
		return "> " + strings.ReplaceAll(strings.TrimSpace(body), "\n", "\n> ") + "\n\n"
	case "codeBlock":
		return "````\n" + body + "\n````\n\n"
	}
	return body
}
