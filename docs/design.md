# 群聊人物 Wiki 系统设计文档

> 本文为旧版设计记录。新的统一文章与 Tag 模型、Go + Vue 架构、媒体和小程序规划见 [RFC 0001](rfcs/0001-lightweight-wiki-go-vue.md)，当前开发入口为 `cmd/wiki` 和 `web`。本文件中的 Next.js 方案只作为迁移参考。

## 项目概述

这是一个专门为群聊社区人物建设设计的知识管理平台，帮助用户记录和分享群成员的特点、经历和故事，形成持久化的群聊文化档案。

## 核心功能

### 1. 用户系统
- 多用户支持，区分普通用户和管理员
- 用户权限分级（超级管理员、管理员、编辑者、普通用户）
- 用户个人主页展示其在群聊中的贡献

### 2. Wiki 词条系统
- 任何人都可以创建和编辑词条
- 词条版本历史记录
- 支持创建关于特定用户的词条
- 富文本编辑器，支持图片、视频嵌入

### 3. 话题系统
- 词条可以归属于不同话题
- 话题分类管理
- 热门话题展示

### 4. 媒体管理
- 图片上传和展示
- 视频嵌入功能
- 文件附件支持

### 5. 搜索功能
- 全文搜索词条内容
- 按标签、作者、时间筛选
- 搜索结果高亮显示

### 6. 分享与权限
- Wiki 内容可公开分享
- 细粒度权限控制
- 私密词条功能

## 技术架构

### 前端技术栈
- Next.js 14 (App Router)
- TypeScript
- Tailwind CSS
- React Query
- Zustand (状态管理)

### 后端技术栈
- Next.js API Routes
- Prisma ORM
- PostgreSQL
- NextAuth.js (认证)
- AWS S3 (文件存储)

### 数据库设计概要

#### 用户表 (User)
- id: 主键
- username: 用户名
- email: 邮箱
- role: 角色枚举
- avatar: 头像URL
- createdAt: 创建时间

#### 词条表 (Article)
- id: 主键
- title: 标题
- content: 内容 (JSON格式存储富文本)
- authorId: 作者ID
- topicId: 话题ID (可选)
- isPublic: 是否公开
- viewCount: 浏览次数
- createdAt: 创建时间
- updatedAt: 更新时间

#### 话题表 (Topic)
- id: 主键
- name: 话题名称
- description: 描述
- color: 主题色
- createdAt: 创建时间

#### 词条版本表 (ArticleVersion)
- id: 主键
- articleId: 词条ID
- content: 版本内容
- editorId: 编辑者ID
- createdAt: 创建时间

#### 媒体文件表 (Media)
- id: 主键
- filename: 文件名
- originalName: 原始文件名
- mimeType: 文件类型
- size: 文件大小
- url: 访问URL
- uploaderId: 上传者ID
- createdAt: 创建时间

#### 标签表 (Tag)
- id: 主键
- name: 标签名
- color: 标签颜色

#### 词条标签关联表 (ArticleTag)
- articleId: 词条ID
- tagId: 标签ID

## 特色功能设计

### 1. 人物画像系统
- 为每个群成员创建专属词条
- 记录成员的经典语录、特点、贡献
- 支持多人共同编辑完善

### 2. 群聊记忆功能
- 重要事件记录
- 经典对话存档
- 群文化传承

### 3. 协作编辑
- 实时显示在线编辑者
- 编辑冲突提醒
- 版本对比功能

## API 设计原则

- RESTful API 设计
- 统一的响应格式
- 完善的错误处理
- 请求频率限制
- API 文档自动生成

## 安全考虑

- JWT 认证
- XSS 防护
- CSRF 保护
- 文件上传安全检查
- SQL 注入防护

## 性能优化

- 数据库索引优化
- 图片懒加载和压缩
- CDN 加速
- 缓存策略
- 分页加载

## 部署方案

- Docker 容器化部署
- Nginx 反向代理
- PostgreSQL 主从复制
- 定期备份策略
