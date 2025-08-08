"use client"

import { useState, useEffect } from "react"
import { useSession } from "next-auth/react"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { formatDistanceToNow } from "date-fns"
import { zhCN } from "date-fns/locale"
import { MessageCircle, Send } from "lucide-react"

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

interface CommentSectionProps {
  articleId: string
  comments: Comment[]
}

export function CommentSection({ articleId, comments: initialComments }: CommentSectionProps) {
  const { data: session } = useSession()
  const [comments, setComments] = useState<Comment[]>(initialComments)
  const [newComment, setNewComment] = useState("")
  const [submitting, setSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!session || !newComment.trim()) return

    setSubmitting(true)
    try {
      const response = await fetch(`/api/articles/${articleId}/comments`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({ content: newComment })
      })

      if (response.ok) {
        const comment = await response.json()
        setComments(prev => [...prev, comment])
        setNewComment("")
      }
    } catch (error) {
      console.error("Submit comment error:", error)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="bg-white rounded-lg p-6">
      <h2 className="text-xl font-semibold mb-6 flex items-center gap-2">
        <MessageCircle className="h-5 w-5" />
        评论 ({comments.length})
      </h2>

      {/* Comment Form */}
      {session && (
        <form onSubmit={handleSubmit} className="mb-8">
          <div className="flex gap-4">
            <Avatar className="h-10 w-10">
              <AvatarImage src={session.user.avatar} />
              <AvatarFallback>
                {session.user.username.charAt(0)}
              </AvatarFallback>
            </Avatar>
            <div className="flex-1">
              <Textarea
                value={newComment}
                onChange={(e) => setNewComment(e.target.value)}
                placeholder="写下你的评论..."
                rows={3}
              />
              <div className="mt-2 flex justify-end">
                <Button type="submit" disabled={submitting || !newComment.trim()}>
                  <Send className="h-4 w-4 mr-2" />
                  {submitting ? "发送中..." : "发送评论"}
                </Button>
              </div>
            </div>
          </div>
        </form>
      )}

      {/* Comments List */}
      <div className="space-y-6">
        {comments.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            还没有评论，来发表第一个评论吧！
          </div>
        ) : (
          comments.map((comment) => (
            <div key={comment.id} className="flex gap-4">
              <Avatar className="h-10 w-10">
                <AvatarImage src={comment.author.avatar} />
                <AvatarFallback>
                  {comment.author.username.charAt(0)}
                </AvatarFallback>
              </Avatar>
              <div className="flex-1">
                <div className="flex items-center gap-2 mb-1">
                  <span className="font-medium">{comment.author.username}</span>
                  <span className="text-sm text-gray-500">
                    {formatDistanceToNow(new Date(comment.createdAt), {
                      addSuffix: true,
                      locale: zhCN
                    })}
                  </span>
                </div>
                <div className="text-gray-700">{comment.content}</div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}