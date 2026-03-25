"use client"

import { useState, useEffect } from "react"
import { useParams } from "next/navigation"
import { useSession } from "next-auth/react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { ArticleContent } from "@/components/article-content"
import { CommentSection } from "@/components/comment-section"
import { ShareDialog } from "@/components/share-dialog"
import { formatDistanceToNow } from "date-fns"
import { zhCN } from "date-fns/locale"
import { Edit, Eye, MessageCircle, Calendar, Share2 } from "lucide-react"
import Link from "next/link"

interface Share {
  id: string
  email?: string
  permission: "VIEW" | "EDIT" | "COMMENT"
  token?: string
  expiresAt?: string
  createdAt: string
  user?: {
    id: string
    username: string
    email: string
  }
}

interface Author {
  id: string
  username: string
  avatar?: string
  bio?: string
}

interface Topic {
  id: string
  name: string
  color: string
}

interface Tag {
  id: string
  name: string
  color: string
}

interface ArticleTag {
  tag: Tag
}

interface Comment {
  id: string
  content: string
  createdAt: string
  author: {
    id: string
    username: string
    avatar?: string
  }
}

interface Article {
  id: string
  title: string
  slug: string
  content: string
  excerpt?: string
  viewCount: number
  createdAt: string
  updatedAt: string
  author: Author
  topic?: Topic
  tags: ArticleTag[]
  comments: Comment[]
  _count: {
    comments: number
  }
}

export default function ArticlePage() {
  const params = useParams()
  const { data: session } = useSession()
  const [article, setArticle] = useState<Article | null>(null)
  const [shares, setShares] = useState<Share[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  useEffect(() => {
    const fetchArticle = async () => {
      try {
        const [articleRes, sharesRes] = await Promise.all([
          fetch(`/api/articles/${params.slug}`),
          fetch(`/api/articles/${params.slug}/share`)
        ])

        if (articleRes.ok) {
          const data = await articleRes.json()
          setArticle(data)
        } else if (articleRes.status === 404) {
          setError("词条不存在")
        } else {
          setError("加载失败")
        }

        if (sharesRes.ok) {
          const sharesData = await sharesRes.json()
          setShares(sharesData)
        }
      } catch (error) {
        console.error("Fetch article error:", error)
        setError("服务器错误")
      } finally {
        setLoading(false)
      }
    }

    if (params.slug) {
      fetchArticle()
    }
  }, [params.slug])

  const handleShare = async (data: {
    email?: string
    permission: "VIEW" | "EDIT" | "COMMENT"
    expiresAt?: string
  }) => {
    const response = await fetch(`/api/articles/${params.slug}/share`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json"
      },
      body: JSON.stringify(data)
    })

    if (!response.ok) {
      const error = await response.json()
      throw new Error(error.error || "分享失败")
    }

    // Refresh shares list
    const sharesRes = await fetch(`/api/articles/${params.slug}/share`)
    if (sharesRes.ok) {
      const sharesData = await sharesRes.json()
      setShares(sharesData)
    }
  }

  const handleRemoveShare = async (shareId: string) => {
    const response = await fetch(`/api/articles/${params.slug}/share/${shareId}`, {
      method: "DELETE"
    })

    if (!response.ok) {
      throw new Error("删除分享失败")
    }

    // Refresh shares list
    const sharesRes = await fetch(`/api/articles/${params.slug}/share`)
    if (sharesRes.ok) {
      const sharesData = await sharesRes.json()
      setShares(sharesData)
    }
  }

  if (loading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600 mx-auto mb-4"></div>
          <p>加载中...</p>
        </div>
      </div>
    )
  }

  if (error || !article) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-4">{error || "词条不存在"}</h1>
          <Link href="/">
            <Button>返回首页</Button>
          </Link>
        </div>
      </div>
    )
  }

  const canEdit = session && (
    session.user.role === "SUPER_ADMIN" ||
    session.user.role === "ADMIN" ||
    article.author.id === session.user.id
  )

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Article Header */}
      <div className="bg-white border-b">
        <div className="container mx-auto px-4 py-8 max-w-4xl">
          <h1 className="text-3xl md:text-4xl font-bold mb-4">{article.title}</h1>
          
          {/* Meta Info */}
          <div className="flex flex-wrap items-center gap-4 text-sm text-gray-600 mb-6">
            <div className="flex items-center gap-2">
              <Avatar className="h-6 w-6">
                <AvatarImage src={article.author.avatar} />
                <AvatarFallback>
                  {article.author.username.charAt(0)}
                </AvatarFallback>
              </Avatar>
              <Link 
                href={`/profile/${article.author.username}`}
                className="hover:text-blue-600"
              >
                {article.author.username}
              </Link>
            </div>
            <div className="flex items-center gap-1">
              <Calendar className="h-4 w-4" />
              <span>
                {formatDistanceToNow(new Date(article.createdAt), {
                  addSuffix: true,
                  locale: zhCN
                })}
              </span>
            </div>
            <div className="flex items-center gap-1">
              <Eye className="h-4 w-4" />
              <span>{article.viewCount} 次浏览</span>
            </div>
            <div className="flex items-center gap-1">
              <MessageCircle className="h-4 w-4" />
              <span>{article._count.comments} 条评论</span>
            </div>
          </div>

          {/* Tags and Topic */}
          <div className="flex flex-wrap gap-2 mb-6">
            {article.topic && (
              <Badge 
                variant="secondary"
                style={{ backgroundColor: `${article.topic.color}20`, color: article.topic.color }}
              >
                {article.topic.name}
              </Badge>
            )}
            {article.tags.map(({ tag }) => (
              <Badge 
                key={tag.id}
                variant="outline"
                style={{ borderColor: tag.color, color: tag.color }}
              >
                #{tag.name}
              </Badge>
            ))}
          </div>

          {/* Actions */}
          <div className="flex gap-2">
            {canEdit && (
              <Link href={`/articles/${article.slug}/edit`}>
                <Button variant="outline">
                  <Edit className="h-4 w-4 mr-2" />
                  编辑
                </Button>
              </Link>
            )}
            <ShareDialog
              articleSlug={article.slug}
              shares={shares}
              onShare={handleShare}
              onRemoveShare={handleRemoveShare}
            >
              <Button variant="outline">
                <Share2 className="h-4 w-4 mr-2" />
                分享
              </Button>
            </ShareDialog>
          </div>
        </div>
      </div>

      {/* Article Content */}
      <div className="container mx-auto px-4 py-8 max-w-4xl">
        <div className="bg-white rounded-lg p-6 mb-8">
          <ArticleContent content={article.content} />
        </div>

        {/* Comments Section */}
        <CommentSection articleId={article.id} comments={article.comments} />
      </div>
    </div>
  )
}