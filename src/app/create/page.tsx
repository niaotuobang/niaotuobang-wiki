"use client"

import { useState, useEffect } from "react"
import { useRouter } from "next/navigation"
import { useSession } from "next-auth/react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Label } from "@/components/ui/label"
import { Checkbox } from "@/components/ui/checkbox"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { Editor } from "@/components/editor"
import { ArrowLeft, Save, X } from "lucide-react"
import Link from "next/link"

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

export default function CreateArticle() {
  const router = useRouter()
  const { data: session } = useSession()
  const [loading, setLoading] = useState(false)
  const [topics, setTopics] = useState<Topic[]>([])
  const [tags, setTags] = useState<Tag[]>([])
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  
  const [formData, setFormData] = useState({
    title: "",
    content: "",
    excerpt: "",
    topicId: "",
    isPublic: true
  })

  useEffect(() => {
    if (!session) {
      router.push("/auth/signin")
      return
    }

    // Fetch topics and tags
    const fetchData = async () => {
      const [topicsRes, tagsRes] = await Promise.all([
        fetch("/api/topics"),
        fetch("/api/tags")
      ])

      if (topicsRes.ok) {
        const topicsData = await topicsRes.json()
        setTopics(topicsData)
      }

      if (tagsRes.ok) {
        const tagsData = await tagsRes.json()
        setTags(tagsData)
      }
    }

    fetchData()
  }, [session, router])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)

    try {
      const response = await fetch("/api/articles", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({
          ...formData,
          tagIds: selectedTags
        })
      })

      if (response.ok) {
        const article = await response.json()
        router.push(`/articles/${article.slug}`)
      } else {
        const error = await response.json()
        alert(error.error || "创建失败")
      }
    } catch (error) {
      console.error("Create article error:", error)
      alert("服务器错误")
    } finally {
      setLoading(false)
    }
  }

  const handleTagToggle = (tagId: string) => {
    setSelectedTags(prev => 
      prev.includes(tagId) 
        ? prev.filter(id => id !== tagId)
        : [...prev, tagId]
    )
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="container mx-auto px-4 py-8 max-w-4xl">
        {/* Header */}
        <div className="mb-8">
          <Link href="/" className="inline-flex items-center text-gray-600 hover:text-gray-900 mb-4">
            <ArrowLeft className="h-4 w-4 mr-2" />
            返回首页
          </Link>
          <h1 className="text-3xl font-bold">创建新词条</h1>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="space-y-6">
          <div>
            <Label htmlFor="title">标题</Label>
            <Input
              id="title"
              value={formData.title}
              onChange={(e) => setFormData(prev => ({ ...prev, title: e.target.value }))}
              placeholder="输入词条标题"
              required
            />
          </div>

          <div>
            <Label htmlFor="excerpt">摘要</Label>
            <Textarea
              id="excerpt"
              value={formData.excerpt}
              onChange={(e) => setFormData(prev => ({ ...prev, excerpt: e.target.value }))}
              placeholder="简要描述这个词条（可选）"
              rows={3}
            />
          </div>

          <div>
            <Label>内容</Label>
            <Editor
              content={formData.content}
              onChange={(content) => setFormData(prev => ({ ...prev, content }))}
              placeholder="开始编写词条内容..."
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label>话题分类</Label>
              <Select value={formData.topicId} onValueChange={(value) => setFormData(prev => ({ ...prev, topicId: value }))}>
                <SelectTrigger>
                  <SelectValue placeholder="选择话题" />
                </SelectTrigger>
                <SelectContent>
                  {topics.map((topic) => (
                    <SelectItem key={topic.id} value={topic.id}>
                      <div className="flex items-center gap-2">
                        <div 
                          className="w-3 h-3 rounded-full" 
                          style={{ backgroundColor: topic.color }}
                        />
                        {topic.name}
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div>
              <Label>标签</Label>
              <div className="flex flex-wrap gap-2 mt-2">
                {tags.map((tag) => (
                  <Badge
                    key={tag.id}
                    variant={selectedTags.includes(tag.id) ? "default" : "outline"}
                    className="cursor-pointer"
                    style={selectedTags.includes(tag.id) ? { backgroundColor: tag.color } : {}}
                    onClick={() => handleTagToggle(tag.id)}
                  >
                    {tag.name}
                  </Badge>
                ))}
              </div>
            </div>
          </div>

          <div className="flex items-center space-x-2">
            <Checkbox
              id="isPublic"
              checked={formData.isPublic}
              onCheckedChange={(checked) => setFormData(prev => ({ ...prev, isPublic: !!checked }))}
            />
            <Label htmlFor="isPublic">公开可见</Label>
          </div>

          <div className="flex justify-end gap-4">
            <Link href="/">
              <Button variant="outline" type="button">
                <X className="h-4 w-4 mr-2" />
                取消
              </Button>
            </Link>
            <Button type="submit" disabled={loading}>
              <Save className="h-4 w-4 mr-2" />
              {loading ? "创建中..." : "创建词条"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  )
}