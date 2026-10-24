package wiki

import "net/http"

type communityMember struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	AvatarMediaID string `json:"avatar_media_id,omitempty"`
	ArticleCount  int    `json:"article_count"`
}

func (a *App) community(w http.ResponseWriter, r *http.Request, _ *User) error {
	// Count each published article once per contributor, including previous
	// editors. Private drafts and recycled articles never influence this list.
	rows, err := a.store.DB.Query(`WITH published AS (
	 SELECT id,author_id,updated_by,created_at,updated_at FROM articles WHERE status='published' AND deleted_at=''
	), contributions AS (
	 SELECT author_id AS user_id,id AS article_id,julianday(created_at) AS edited_at FROM published
	 UNION ALL SELECT updated_by,id,julianday(updated_at) FROM published
	 UNION ALL SELECT json_extract(v.snapshot,'$.updated_by'),v.article_id,julianday(json_extract(v.snapshot,'$.updated_at'))
	 FROM article_versions v JOIN published p ON p.id=v.article_id
	), activity AS (
	 SELECT user_id,count(DISTINCT article_id) AS article_count,max(edited_at) AS edited_at FROM contributions GROUP BY user_id
	)
	SELECT u.id,u.name,coalesce(u.avatar_media_id,''),coalesce(a.article_count,0)
	FROM users u LEFT JOIN activity a ON a.user_id=u.id
	WHERE u.disabled=0 AND (u.id NOT LIKE 'legacy-user-%' OR EXISTS(SELECT 1 FROM legacy_user_profiles p WHERE p.user_id=u.id))
	ORDER BY a.edited_at DESC,julianday(u.created_at) DESC,u.id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := []communityMember{}
	for rows.Next() {
		var member communityMember
		if err := rows.Scan(&member.ID, &member.Name, &member.AvatarMediaID, &member.ArticleCount); err != nil {
			return err
		}
		items = append(items, member)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	writeJSON(w, 200, map[string]any{"items": items})
	return nil
}
