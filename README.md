# 鸟托邦 Wiki

给微信群友共同写故事的轻量 Wiki。人物、事件、金句和群梗都是文章，用 Tag 和发生时间串起来。页面参考 Matterwiki，正文、留白和直接编辑是重点。

新版使用 **Go + Vue 3 + SQLite**。Go 同时提供网页资源和 `/api/v1`，发行文件运行时无需 Node.js、PostgreSQL 或 Redis。仓库原有的 Next.js 源码暂留作迁移参考，新版入口是 `cmd/wiki` 和 `web`。

- [RFC 0001：页面与架构](docs/rfcs/0001-lightweight-wiki-go-vue.md)
- [运行、媒体、备份和旧站迁移](docs/operations.md)
- [实现与验证记录](docs/implementation-status.md)
- [OpenAPI 契约](api/openapi.yaml)

## 本地体验

从源码构建需要 Go 1.26+、Node.js 22.12+、npm 和 Make。Node.js 仅用于构建 Vue。

```sh
make demo
```

打开 <http://127.0.0.1:8080>。演示使用虚构内容，数据保存在 `data/demo`，只允许绑定本机回环地址。可以查看列表、人物文章、标签和时间线，试写草稿、发布与恢复版本。演示与正式数据应使用不同目录。

## 建立自己的 Wiki

```sh
make build
./bin/wiki serve --data-dir ./data/wiki --listen 127.0.0.1:8080
```

第一次启动会在数据目录生成权限为 `0600` 的 `setup-token`。在网页上用这份一次性凭证创建管理员，再邀请群友加入。默认只允许登录成员阅读，管理员可显式开启公开阅读。

编译后的 `bin/wiki` 已嵌入网页。部署时只需对应平台的程序、可写持久目录和环境配置。正式 HTTPS 入口需要设置 `BASE_URL`，如通用示例 `https://wiki.example.com`，以正确启用安全 Cookie。

## 开发与检查

```sh
# 终端一：Go API，使用独立的演示数据
make dev-api

# 终端二：Vite 热更新，代理 API 到 Go
make dev-web

make test
make check
```

Vue 页面包括首页、Tag 列表、文章、富文本编辑器、版本历史、时间线、搜索、个人草稿和成员管理。编辑器独立按需加载，阅读页不加载它。图片、视频均通过媒体 ID 引用，支持本地流式上传及 OSS 签名直传。

## 容器

```sh
# 使用本机工具链构建 Linux 程序并封装成无 Node.js 的镜像
make image
docker compose up -d --no-build
```

`Dockerfile.binary` 从已编译程序构建最小镜像，不需要拉取基础镜像。`Dockerfile` 另外提供完整的多阶段源码构建。两种方式均需持久化 `/data`；Compose 默认限制为 1 核、1 GiB，端口只绑定本机。

程序内包含仅在系统证书缺失时使用的 Go 公共根证书后备集，最小容器仍可验证 OSS 等 HTTPS 服务；随程序依赖更新维护。[Go 根证书后备包](https://pkg.go.dev/golang.org/x/crypto/x509roots/fallback)

## 当前边界

第一版实现统一文章和 Tag、图文视频、草稿与版本、链接与反向链接、本地账号与邀请、备份恢复和 Matterwiki 导入。AI、微信／手机号／邮箱绑定及原生小程序属于 RFC 的后续阶段；未来小程序复用当前 Go API 和正文格式。

真实账号、云资源、部署记录、生产导出和凭证只放在 Git 忽略的本地私有目录或部署平台秘密配置中，公开文档只保留通用示例。
