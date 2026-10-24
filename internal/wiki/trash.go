package wiki

import (
	"encoding/json"
	"net/http"
)

// Trash access is separate from the ordinary reading endpoints. Deletion never
// makes a private draft readable to a different member, including administrators.
func (a *App) trashedArticle(id string, u *User) (Article, error) {
	v, err := getArticle(a.store.DB, id)
	if err != nil {
		return v, err
	}
	if v.DeletedAt == "" || v.Status == "draft" && v.AuthorID != u.ID {
		return Article{}, fail(404, "NOT_FOUND", "回收站中没有这篇文章")
	}
	return v, nil
}

func (a *App) trashRoutes() {
	a.route("GET /api/v1/admin/trash", "admin", func(w http.ResponseWriter, r *http.Request, u *User) error {
		cursorTime, cursorID := "", ""
		if value := r.URL.Query().Get("cursor"); value != "" {
			var pair []string
			if json.Unmarshal([]byte(value), &pair) != nil || len(pair) != 2 {
				return fail(400, "VALIDATION_ERROR", "回收站游标无效")
			}
			cursorTime, cursorID = pair[0], pair[1]
		}
		rows, err := a.store.DB.Query(`SELECT id,title,deleted_at FROM articles WHERE deleted_at<>'' AND (status='published' OR author_id=?) AND (?='' OR deleted_at<? OR (deleted_at=? AND id<?)) ORDER BY deleted_at DESC,id DESC LIMIT 21`, u.ID, cursorTime, cursorTime, cursorTime, cursorID)
		if err != nil {
			return err
		}
		defer rows.Close()
		items := []map[string]string{}
		for rows.Next() {
			var id, title, ts string
			if err := rows.Scan(&id, &title, &ts); err != nil {
				return err
			}
			items = append(items, map[string]string{"id": id, "title": title, "deleted_at": ts})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		next := ""
		if len(items) > 20 {
			items = items[:20]
			last := items[len(items)-1]
			b, _ := json.Marshal([]string{last["deleted_at"], last["id"]})
			next = string(b)
		}
		writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
		return nil
	})
	a.route("GET /api/v1/admin/trash/{id}", "admin", func(w http.ResponseWriter, r *http.Request, u *User) error {
		v, err := a.trashedArticle(r.PathValue("id"), u)
		if err != nil {
			return err
		}
		writeJSON(w, 200, v)
		return nil
	})
	a.route("GET /api/v1/admin/trash/{id}/versions", "admin", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if _, err := a.trashedArticle(r.PathValue("id"), u); err != nil {
			return err
		}
		return a.listVersions(w, r)
	})
	a.route("GET /api/v1/admin/trash/{id}/versions/{version}", "admin", func(w http.ResponseWriter, r *http.Request, u *User) error {
		if _, err := a.trashedArticle(r.PathValue("id"), u); err != nil {
			return err
		}
		v, err := a.version(r.PathValue("id"), r.PathValue("version"))
		if err != nil {
			return err
		}
		writeJSON(w, 200, v)
		return nil
	})
}
