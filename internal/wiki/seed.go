package wiki

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func paragraph(text string) Node {
	if text == "" {
		return Node{Type: "paragraph", Attrs: map[string]any{"id": newID()}}
	}
	return Node{Type: "paragraph", Attrs: map[string]any{"id": newID()}, Content: []Node{{Type: "text", Text: text}}}
}
func document(lines ...string) Node {
	d := Node{Type: "doc"}
	for _, s := range lines {
		d.Content = append(d.Content, paragraph(s))
	}
	return d
}
func (a *App) seedDemo() error {
	var n int
	if e := a.store.DB.QueryRow(`SELECT count(*) FROM users WHERE id='demo-editor'`).Scan(&n); e != nil {
		return e
	}
	if n > 0 {
		return nil
	}
	var existing int
	if e := a.store.DB.QueryRow(`SELECT count(*) FROM users`).Scan(&existing); e != nil {
		return e
	}
	if existing > 0 {
		return fmt.Errorf("demo mode requires a separate empty data directory")
	}
	if _, e := a.store.DB.Exec(`INSERT INTO users(id,name,login,password_hash,role) VALUES('demo-editor','群友','demo','','admin')`); e != nil {
		return e
	}
	if _, e := a.store.DB.Exec(`INSERT INTO settings(key,value) VALUES('public_read','true') ON CONFLICT(key) DO NOTHING`); e != nil {
		return e
	}
	tags := []Tag{{ID: "people", Name: "人物", Description: "群里每一个人的小传", ShowInNav: true, NavOrder: 1}, {ID: "events", Name: "事件", Description: "一些值得记录的事情", ShowInNav: true, NavOrder: 2}, {ID: "quotes", Name: "金句", Description: "聊天里脱口而出的好句子", ShowInNav: true, NavOrder: 3}, {ID: "jokes", Name: "群梗", Description: "只有我们才懂的笑话", ShowInNav: true, NavOrder: 4}, {ID: "history", Name: "历史记录", Description: "记录我们一起走过的日子", ShowInNav: true, NavOrder: 5}, {ID: "gathering", Name: "聚会", Description: "终于从线上聊到了线下", ShowInNav: true, NavOrder: 6}, {ID: "major", Name: "重大事件", Description: "值得圈出来的那些日子", NavOrder: 7}, {ID: "wedding", Name: "结婚", Description: "一起分享的人生大事", NavOrder: 8}}
	for _, t := range tags {
		if _, e := a.store.DB.Exec(`INSERT INTO tags(id,name,name_key,description,show_in_nav,nav_order) VALUES(?,?,?,?,?,?)`, t.ID, t.Name, t.Name, t.Description, t.ShowInNav, t.NavOrder); e != nil {
			return e
		}
	}
	samples := []struct {
		ID, Title       string
		Lines           []string
		Tags            []string
		Date, Precision string
	}{
		{"welcome", "把群里的故事，留在这里", []string{"有些事，只有这个群的人才知道。", "某天脱口而出的金句，一场说走就走的聚会，一个外号背后长长的故事。聊天会被新的消息推远，我们可以把它们慢慢写下来。", "这里的每一篇文章都一样：可以写一个人，也可以记一件事。加上标签，顺手填一个发生时间，几年以后，再回来看。", "这是一个演示空间，人物和故事均为虚构。欢迎试着编辑，或者写下第一篇属于你们的故事。"}, []string{"history"}, "2026-10", "month"},
		{"linzhou", "林舟", []string{"人设：群里的野生摄影师、周末出走计划发起人、永远说“马上就到”的人。", "加入群的第一天，他发了一张拍糊的月亮，认真解释了十分钟曝光参数。从此大家约定，每次出门都给他留一张拍合照的位置。", "平时话不多，一说到露营和咖啡就停不下来。那句“先出发，天气到了再说”，后来成了群里的暗号。"}, []string{"people"}, "", ""},
		{"autumn", "秋天的第一次见面", []string{"约了三个星期，终于在十月的一个周六见面了。", "有人带了咖啡，有人带了相机，还有人带了整整一袋没有人吃得完的橘子。原来每天隔着屏幕聊天的人，在线下也一样好笑。", "林舟负责拍合照。拍到第七张的时候，大家决定：不等所有人都看镜头了，这一张就很好。"}, []string{"events", "gathering"}, "2025-10-18", "day"},
		{"coffee", "“咖啡可以凉，话不能掉地上”", []string{"某次深夜聊天，大家讨论到最后已经忘记了最初的话题。", "有人准备结束：“咖啡都凉了，明天再说吧。”", "另一位群友接了一句：“咖啡可以凉，话不能掉地上。”于是大家又聊了一个小时。"}, []string{"quotes", "jokes"}, "2025-08", "month"},
		{"summer", "那一年，我们开始叫这里鸟托邦", []string{"起名字时，大家认真提议了一晚上，投了三轮票。", "最后被选中的，却是有人随手打出来的三个字。没有宏大的解释，只是觉得待在这里挺自在。", "后来回头看，名字是怎么来的已经不重要了。重要的是，我们还在这里聊天。"}, []string{"history", "major"}, "2024", "year"},
	}
	u := User{ID: "demo-editor", Name: "群友", Role: "admin"}
	for index, s := range samples {
		v, e := a.store.saveArticle("", WriteArticle{Title: s.Title, Document: document(s.Lines...), TagIDs: s.Tags, OccurredAt: s.Date, DatePrecision: s.Precision, Status: "published", ChangeSummary: "写下这个故事"}, u, true)
		if e != nil {
			return e
		}
		if e = renameSeedID(a.store.DB, v.ID, s.ID, time.Now().UTC().Add(-time.Duration(index+1)*24*time.Hour).Format(time.RFC3339Nano)); e != nil {
			return e
		}
	}
	// One explicit reference demonstrates backlinks without inventing a separate person model.
	v, e := getArticle(a.store.DB, "autumn")
	if e != nil {
		return e
	}
	v.Document.Content = append(v.Document.Content, Node{Type: "paragraph", Attrs: map[string]any{"id": newID()}, Content: []Node{{Type: "text", Text: "合照里的故事，也写在了 "}, {Type: "text", Text: "林舟", Marks: []Mark{{Type: "link", Attrs: map[string]any{"href": "/articles/linzhou"}}}}, {Type: "text", Text: " 的小传里。"}}})
	body := fromArticle(v)
	body.ChangeSummary = "补上人物链接"
	_, e = a.store.saveArticle(v.ID, body, u, false)
	if e != nil {
		return e
	}
	return a.stampSeedUpdate("autumn", time.Now().UTC().Add(-72*time.Hour).Format(time.RFC3339Nano))
}
func renameSeedID(db *sql.DB, old, id, ts string) error {
	// IDs for fixtures are assigned before any dependent article is linked.
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`PRAGMA defer_foreign_keys=ON`); e != nil {
		return e
	}
	for _, table := range []string{"articles", "article_tags", "article_versions", "article_media"} {
		col := "article_id"
		if table == "articles" {
			col = "id"
		}
		if _, e = tx.Exec(`UPDATE `+table+` SET `+col+`=? WHERE `+col+`=?`, id, old); e != nil {
			return e
		}
	}
	var snapshot string
	if e = tx.QueryRow(`SELECT snapshot FROM article_versions WHERE article_id=?`, id).Scan(&snapshot); e != nil {
		return e
	}
	var a Article
	if e = json.Unmarshal([]byte(snapshot), &a); e != nil {
		return e
	}
	a.ID = id
	a.UpdatedAt = ts
	a.CreatedAt = ts
	b, _ := json.Marshal(a)
	if _, e = tx.Exec(`UPDATE article_versions SET snapshot=? WHERE article_id=?`, string(b), id); e != nil {
		return e
	}
	if _, e = tx.Exec(`UPDATE articles SET updated_at=?,created_at=? WHERE id=?`, ts, ts, id); e != nil {
		return e
	}
	return tx.Commit()
}

func (a *App) stampSeedUpdate(id, timestamp string) error {
	tx, e := a.store.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	v, e := getArticle(tx, id)
	if e != nil {
		return e
	}
	v.UpdatedAt = timestamp
	b, _ := json.Marshal(v)
	if _, e = tx.Exec(`UPDATE articles SET updated_at=? WHERE id=?`, timestamp, id); e != nil {
		return e
	}
	if _, e = tx.Exec(`UPDATE article_versions SET snapshot=? WHERE article_id=? AND revision=?`, string(b), id, v.Revision); e != nil {
		return e
	}
	return tx.Commit()
}
