# 运行与数据操作

## 程序与数据

`make build` 先构建 `web`，再将 Vue 资源嵌入 `bin/wiki`。发布后由 Go 一个进程提供网页、API 和媒体授权。SQLite 使用 WAL、外键和忙等待；一个运行实例对应一个持久化数据目录。

```sh
./bin/wiki serve --data-dir ./data/wiki --listen 127.0.0.1:8080
./bin/wiki healthcheck --listen 127.0.0.1:8080
```

首次设置需要数据目录内的 `setup-token`，完成后自动失效。容器使用非 root 用户，需要可写的 `/data`。获取容器的初始化凭证时，应将它复制到本机私有目录，完成设置后删除副本。

`--demo` 只供本机使用，包含虚构故事和自动演示会话。它拒绝公开监听地址及已有正式账号的数据目录。普通 `serve` 不包含演示登录。

代码按功能文件组织在 `internal/wiki`，网页资源单独放在 `internal/webui`。第一版保持一个业务包，避免在规模很小时增加跨包接口；SQL 迁移嵌入在 `internal/wiki/migrations`。未来按模块拆包不改变 `/api/v1`。

## 配置

配置来自环境变量；程序不自动读取 `.env`。正式部署由平台注入环境变量，或由运维在受保护的本地文件中维护。

| 变量 | 含义 |
| --- | --- |
| `DATA_DIR` | 持久目录，默认 `./data` |
| `LISTEN_ADDR` | 默认 `127.0.0.1:8080`；容器为 `0.0.0.0:8080` |
| `BASE_URL` | 对外的 HTTP(S) 源地址；HTTPS 部署须设置，以启用 Secure Cookie |
| `STORAGE_DRIVER` | `local` 或 `oss`，默认本地 |
| `OSS_ENDPOINT` | OSS 服务地址；跨云部署使用可达的公网 HTTPS Endpoint |
| `OSS_REGION` | OSS 区域，用于 V4 签名 |
| `OSS_BUCKET` | 媒体存储桶，默认应为私有读写 |
| `OSS_ACCESS_KEY_ID`、`OSS_ACCESS_KEY_SECRET` | 只在服务端提供的凭证 |
| `OSS_SECURITY_TOKEN` | 使用临时凭证时的安全令牌，可选 |
| `GOMEMLIMIT` | Go 运行时软内存目标，可按部署容器上限设置 |

配置键名可以公开，真实配置值不能进入仓库、日志或示例。网页不会拿到长期 OSS 密钥。CLI profile 与实际资源清单不属于公开应用配置。

## 图片与视频

支持 JPEG、PNG、WebP、GIF，以及 MP4、WebM、MOV 容器。图片上限 20 MiB、2400 万像素；视频上限 200 MiB。实际视频编码能否播放取决于客户端，建议使用 H.264／AAC 的 MP4；不支持时阅读器提供下载入口。第一版不运行自动转码。

本地模式采用“申请意图 → 流式 PUT → 校验完成”协议。文件在完成前保存在临时区，校验后移动到最终对象。视频读取支持 Range，不将整段视频放入 Go 内存。

OSS 使用 V4 签名直传，绑定对象键、Content-Type、Content-Length 和有效期。浏览器自动设置文件的 Content-Length，不应手动设置这个受限请求头。Go 在完成阶段检查实际元数据及文件特征，以 ETag 为条件复制到客户端不能写入的最终键，并设为私有。完成接口可以幂等重试。

Bucket CORS 需要允许网页源的 `PUT`、`GET`、`HEAD` 及 `Content-Type` 请求头。媒体通过顶层 `/media/:id` 校验访问权限，再由 OSS 传输字节。读取签名默认十五分钟，持有地址的人在有效期内能读取，因此撤权不是即时作废已签发 URL。

每个客户端最多并行上传两个文件；未完成上传有数量与容量限制。超过一小时的临时上传定期清理。已完成媒体保守保留，尤其不会删除仍被草稿或历史版本使用的文件；第一版不提供不可恢复的在线清空媒体功能。

## 备份、导出与恢复

```sh
./bin/wiki backup --data-dir ./data/wiki --output ./backups
./bin/wiki export --data-dir ./data/wiki --output ./backups/readable
./bin/wiki restore --input ./backups/backup-file.tar.gz --data-dir ./data/restored
```

备份使用 `VACUUM INTO` 制作数据库一致性快照，再打包所有已完成媒体和 SHA-256 清单。本地和 OSS 驱动都会备份实际媒体字节，不只有对象键。备份不包含有效会话或邀请。

`export` 从同一备份快照生成 `articles.json` 和便于阅读的 `articles.md`。JSON 保留正文结构、Tag、作者署名、版本、草稿与媒体清单；对应 tar.gz 保存实际文件。Markdown 是阅读导出，不能替代完整备份；如需显示其中的相对媒体链接，可将对应备份包解压到同一目录。

恢复只接受空目标目录，校验清单、文件类型、路径和 SQLite 完整性后才就位。当前默认限制单个条目不超过 1 GiB、总展开量不超过 20 GiB；较大空间在正式使用前需要调整恢复限额并演练。恢复到本地驱动即可查看完整备份，无须访问原 OSS。

数据库结构升级前自动保存迁移备份。回滚须使用匹配版本的程序和数据库；不能直接让旧程序打开新结构。

## Matterwiki 导入

先取得授权导出，在正式切换时停止旧站写入。导入工具离线消费规范化 JSON，不自动访问旧站、采集微信群或下载不明外部链接。旧站 API 返回的 `data` 数组需整理为下面的顶层集合；每篇文章的历史版本放在 `history` 中，按从早到晚导入，当前正文最后保存。

```json
{
  "topics": [{"id": 1, "name": "人物", "description": "群友的小传"}],
  "users": [{"id": 7, "name": "旧站作者"}],
  "articles": [{
    "id": 80,
    "title": "一个虚构人物",
    "body": "<p>人工写下的小传。</p>",
    "topic_id": 1,
    "user_id": 7,
    "created_at": "2024-01-02T00:00:00Z",
    "updated_at": "2025-03-04T00:00:00Z",
    "history": []
  }],
  "media": []
}
```

附件使用 `{"url":"原文中的地址","path":"media/photo.png","content_type":"image/png"}`，路径相对于导出 JSON，必须位于导出目录内。工具校验文件、复制为不可变媒体对象并重写正文引用。不会执行原始 HTML；遇到不支持的表格、协议或节点，先报告并阻止实际导入，避免静默丢失内容。

```sh
# 默认预演，不写入文章
./bin/wiki import-matterwiki --data-dir ./data/wiki --input ./legacy-export/export.json

# 阅读私有的 imports/preview-report.json 后再应用
./bin/wiki import-matterwiki --data-dir ./data/wiki --input ./legacy-export/export.json --apply
```

导入须先创建管理员。旧主题转成普通 Tag，旧成员只作为署名账号，不导入登录凭证；旧 ID 映射为稳定的 `legacy-数字` 文章 ID。重复导入跳过已有文章，不把导入工具当作双向同步。原始输入按内容哈希保存在受保护的数据目录，报告也只保存在该目录。

时间戳应在导出时规范为带时区的 ISO 8601；兼容的旧 SQLite 时间格式按 UTC 解释，正式迁移前须核对旧站时区。没有可靠发生日期的文章不自动补时间线日期。

旧 `#/article/数字` 链接由网页入口转换为新路由。要延续旧域名上的链接，还需要让旧入口提供这个过渡页面，因为 URL hash 不会发给服务器。

## 小程序与后续扩展

小程序代码包以后通过微信分发；内容、账号、分页和媒体协议复用 Go。小程序需要原生正文渲染器和适合微信上传方式的客户端适配，Vue 组件不会直接运行在原生小程序中。

当前提供的是网页与共享 API，尚未实现原生小程序、微信身份绑定或 AI 任务。资格、合法域名及真机媒体兼容在对应阶段验证。
