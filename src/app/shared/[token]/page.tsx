import { notFound } from "next/navigation"
import { prisma } from "@/lib/prisma"
import { ArticleContent } from "@/components/article-content"
import { CommentSection } from "@/components/comment-section"

interface SharedArticlePageProps {
  params: Promise<{
    token: string
  }>
}

export default async function SharedArticlePage({ params }: SharedArticlePageProps) {
  const { token } = await params

  // Find share record
  const share = await prisma.articleShare.findFirst({
    where: {
      token,
      OR: [
        { expiresAt: null },
        { expiresAt: { gt: new Date() } }
      ]
    },
    include: {
      article: {
        include: {
          author: {
            select: {
              id: true,
              username: true,
              avatar: true
            }
          },
          topic: {
            select: {
              id: true,
              name: true,
              color: true,
              icon: true
            }
          },
          tags: {
            include: {
              tag: true
            }
          },
          comments: {
            include: {
              author: {
                select: {
                  id: true,
                  username: true,
                  avatar: true
                }
              }
            },
            orderBy: { createdAt: "asc" }
          }
        }
      }
    }
  })

  if (!share || !share.article) {
    notFound()
  }

  const article = {
    ...share.article,
    createdAt: share.article.createdAt.toISOString(),
    updatedAt: share.article.updatedAt.toISOString(),
    comments: share.article.comments.map(comment => ({
      ...comment,
      createdAt: comment.createdAt.toISOString(),
      author: {
        ...comment.author,
        avatar: comment.author.avatar || undefined
      }
    }))
  }

  // Check permission
  const canEdit = share.permission === "EDIT"
  const canComment = share.permission === "EDIT" || share.permission === "COMMENT"

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-4xl mx-auto p-4">
        {/* Share Banner */}
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-4 mb-6">
          <p className="text-sm text-blue-800">
            这是通过分享链接访问的词条
            {share.email && `，分享给: ${share.email}`}
            {share.expiresAt && `，有效期至: ${share.expiresAt.toLocaleDateString('zh-CN')}`}
          </p>
        </div>

        {/* Article Content */}
        <ArticleContent 
          content={article.content}
        />

        {/* Comments Section */}
        {canComment && (
          <div className="mt-8">
            <CommentSection 
              articleId={article.slug}
              comments={article.comments}
            />
          </div>
        )}
      </div>
    </div>
  )
}