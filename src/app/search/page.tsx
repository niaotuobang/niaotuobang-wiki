"use client"

import { useState, useEffect, useRef } from "react"
import { useRouter, useSearchParams } from "next/navigation"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar"
import { Search, Clock, Eye, MessageCircle } from "lucide-react"
import { formatDistanceToNow } from "date-fns"
import { zhCN } from "date-fns/locale"
import Link from "next/link"

interface SearchResult {
  id: string
  title: string
  slug: string
  excerpt?: string
  viewCount: number
  createdAt: string
  updatedAt: string
  author: {
    id: string
    username: string
    avatar?: string
  }
  topic?: {
    id: string
    name: string
    color: string
    icon?: string
  }
  tags: {
    tag: {
      id: string
      name: string
      color: string
    }
  }[]
  _count: {
    comments: number
  }
}

interface TopicSuggestion {
  id: string
  name: string
  color: string
  icon?: string
}

interface TagSuggestion {
  id: string
  name: string
  color: string
}

export default function SearchPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const initialQuery = searchParams.get("q") || ""
  
  const [query, setQuery] = useState(initialQuery)
  const [results, setResults] = useState<SearchResult[]>([])
  const [topics, setTopics] = useState<TopicSuggestion[]>([])
  const [tags, setTags] = useState<TagSuggestion[]>([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const [page, setPage] = useState(1)
  const inputRef = useRef<HTMLInputElement>(null)

  const performSearch = async (searchQuery: string, pageNum = 1) => {
    if (!searchQuery.trim()) return

    setLoading(true)
    try {
      const params = new URLSearchParams({
        q: searchQuery,
        page: pageNum.toString(),
        limit: "10"
      })
      
      const response = await fetch(`/api/search?${params}`)
      if (response.ok) {
        const data = await response.json()
        setResults(data.articles)
        setTopics(data.topics)
        setTags(data.tags)
        setTotal(data.pagination.total)
        setPage(pageNum)
      }
    } catch (error) {
      console.error("Search error:", error)
    } finally {
      setLoading(false)
    }
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (query.trim()) {
      router.push(`/search?q=${encodeURIComponent(query.trim())}`)
      performSearch(query.trim())
    }
  }

  const handleSuggestionClick = (type: "topic" | "tag", id: string, name: string) => {
    setQuery(name)
    performSearch(name)
  }

  useEffect(() => {
    if (initialQuery) {
      performSearch(initialQuery)
    }
    // Focus input on load
    inputRef.current?.focus()
  }, [initialQuery])

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-4xl mx-auto p-4">
        {/* Search Header */}
        <div className="bg-white rounded-lg shadow-sm p-6 mb-6">
          <h1 className="text-2xl font-bold mb-4">搜索 Wiki</h1>
          <form onSubmit={handleSubmit} className="flex gap-2">
            <Input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索词条、话题或标签..."
              className="flex-1"
            />
            <Button type="submit" disabled={loading}>
              <Search className="h-4 w-4 mr-2" />
              {loading ? "搜索中..." : "搜索"}
            </Button>
          </form>
        </div>

        {/* Suggestions */}
        {query && (topics.length > 0 || tags.length > 0) && (
          <div className="bg-white rounded-lg shadow-sm p-6 mb-6">
            <h2 className="text-lg font-semibold mb-3">相关推荐</h2>
            <div className="space-y-3">
              {topics.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-gray-600 mb-2">话题</h3>
                  <div className="flex flex-wrap gap-2">
                    {topics.map((topic) => (
                      <button
                        key={topic.id}
                        onClick={() => handleSuggestionClick("topic", topic.id, topic.name)}
                        className="flex items-center gap-1 px-3 py-1 rounded-full text-sm hover:bg-gray-100"
                        style={{ backgroundColor: `${topic.color}20`, color: topic.color }}
                      >
                        {topic.icon && <span>{topic.icon}</span>}
                        {topic.name}
                      </button>
                    ))}
                  </div>
                </div>
              )}
              {tags.length > 0 && (
                <div>
                  <h3 className="text-sm font-medium text-gray-600 mb-2">标签</h3>
                  <div className="flex flex-wrap gap-2">
                    {tags.map((tag) => (
                      <button
                        key={tag.id}
                        onClick={() => handleSuggestionClick("tag", tag.id, tag.name)}
                        className="px-3 py-1 rounded-full text-sm hover:bg-gray-100"
                        style={{ backgroundColor: `${tag.color}20`, color: tag.color }}
                      >
                        #{tag.name}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Search Results */}
        <div className="space-y-4">
          {results.length > 0 ? (
            <>
              <div className="text-sm text-gray-600">
                找到 {total} 个结果
              </div>
              {results.map((article) => (
                <Card key={article.id} className="hover:shadow-md transition-shadow">
                  <CardHeader>
                    <div className="flex items-start justify-between">
                      <div className="flex-1">
                        <CardTitle className="text-lg mb-2">
                          <Link 
                            href={`/articles/${article.slug}`}
                            className="hover:text-blue-600 transition-colors"
                          >
                            {article.title}
                          </Link>
                        </CardTitle>
                        <div className="flex items-center gap-4 text-sm text-gray-600">
                          <div className="flex items-center gap-2">
                            <Avatar className="h-6 w-6">
                              <AvatarImage src={article.author.avatar} />
                              <AvatarFallback className="text-xs">
                                {article.author.username.charAt(0)}
                              </AvatarFallback>
                            </Avatar>
                            <span>{article.author.username}</span>
                          </div>
                          <div className="flex items-center gap-1">
                            <Clock className="h-4 w-4" />
                            <span>
                              {formatDistanceToNow(new Date(article.updatedAt), {
                                addSuffix: true,
                                locale: zhCN
                              })}
                            </span>
                          </div>
                          <div className="flex items-center gap-1">
                            <Eye className="h-4 w-4" />
                            <span>{article.viewCount}</span>
                          </div>
                          <div className="flex items-center gap-1">
                            <MessageCircle className="h-4 w-4" />
                            <span>{article._count.comments}</span>
                          </div>
                        </div>
                      </div>
                      {article.topic && (
                        <Link
                          href={`/?topic=${article.topic.id}`}
                          className="flex items-center gap-1 px-2 py-1 rounded text-sm hover:bg-gray-100"
                          style={{ backgroundColor: `${article.topic.color}20`, color: article.topic.color }}
                        >
                          {article.topic.icon && <span>{article.topic.icon}</span>}
                          {article.topic.name}
                        </Link>
                      )}
                    </div>
                  </CardHeader>
                  <CardContent>
                    {article.excerpt && (
                      <p className="text-gray-700 mb-3 line-clamp-2">
                        {article.excerpt}
                      </p>
                    )}
                    {article.tags.length > 0 && (
                      <div className="flex flex-wrap gap-2">
                        {article.tags.map(({ tag }) => (
                          <Link
                            key={tag.id}
                            href={`/?tag=${tag.id}`}
                            className="text-xs"
                          >
                            <Badge
                              variant="secondary"
                              style={{ backgroundColor: `${tag.color}20`, color: tag.color }}
                            >
                              #{tag.name}
                            </Badge>
                          </Link>
                        ))}
                      </div>
                    )}
                  </CardContent>
                </Card>
              ))}
              {/* Pagination could be added here */}
            </>
          ) : query ? (
            <div className="text-center py-8 text-gray-500">
              没有找到与 &quot;{query}&quot; 相关的结果
            </div>
          ) : (
            <div className="text-center py-8 text-gray-500">
              输入关键词开始搜索
            </div>
          )}
        </div>
      </div>
    </div>
  )
}