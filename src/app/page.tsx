'use client'

import { useState, useEffect } from "react"
import { Navbar } from "@/components/navbar"
import Link from "next/link"
import { Badge } from "@/components/ui/badge"
import { BookOpen, Edit, Star, Eye, TrendingUp, FileText, MessageSquare } from "lucide-react"

interface Article {
  id: string
  title: string
  slug: string
  excerpt: string | null
  content: string
  publishedAt: string
  author: {
    id: string
    username: string
    avatar: string | null
  }
  topic: {
    id: string
    name: string
    color: string
  } | null
  tags: {
    tag: {
      id: string
      name: string
    }
  }[]
  _count: {
    comments: number
  }
}

interface Topic {
  id: string
  name: string
  color: string
  icon?: string
  _count: {
    articles: number
  }
}

export default function Home() {
  const [featuredArticles, setFeaturedArticles] = useState<Article[]>([])
  const [recentArticles, setRecentArticles] = useState<Article[]>([])
  const [topics, setTopics] = useState<Topic[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchData = async () => {
      try {
        // Fetch featured articles (most viewed or recently updated)
        const featuredRes = await fetch('/api/articles?limit=6')
        const featuredData = await featuredRes.json()
        setFeaturedArticles(featuredData.articles || [])

        // Fetch recent articles
        const recentRes = await fetch('/api/articles?limit=5')
        const recentData = await recentRes.json()
        setRecentArticles(recentData.articles || [])

        // Fetch topics
        const topicsRes = await fetch('/api/topics')
        const topicsData = await topicsRes.json()
        setTopics(topicsData.topics || [])
      } catch (error) {
        console.error('Failed to fetch data:', error)
      } finally {
        setLoading(false)
      }
    }

    fetchData()
  }, [])

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    })
  }

  const stripHtml = (html: string) => {
    const tmp = document.createElement('div')
    tmp.innerHTML = html
    return tmp.textContent || tmp.innerText || ''
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <Navbar />
      
      <main className="container mx-auto px-4 py-6">
        {/* Wikipedia-style Header */}
        <div className="bg-white rounded-lg shadow-sm p-6 mb-6 border border-gray-200">
          <div className="flex flex-col lg:flex-row gap-6">
            {/* Main Content Area */}
            <div className="flex-1">
              {/* Welcome Section */}
              <div className="mb-6">
                <h1 className="text-3xl font-bold text-gray-900 mb-2">欢迎来到鸟托邦 Wiki</h1>
                <p className="text-gray-600 text-lg">
                  鸟托邦 Wiki 是一个自由的百科全书，由像您这样的志愿者共同编写和维护。
                  目前共有 <span className="font-semibold text-blue-600">1,234</span> 篇条目。
                </p>
              </div>

              {/* Featured Article */}
              <div className="mb-6">
                <div className="flex items-center gap-2 mb-3">
                  <Star className="w-5 h-5 text-yellow-500" />
                  <span className="text-sm font-semibold text-gray-700">今日特色条目</span>
                </div>
                <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
                  {loading ? (
                    <div className="animate-pulse">
                      <div className="h-6 bg-gray-300 rounded w-3/4 mb-2"></div>
                      <div className="h-4 bg-gray-300 rounded w-full mb-2"></div>
                      <div className="h-4 bg-gray-300 rounded w-5/6"></div>
                    </div>
                  ) : featuredArticles.length > 0 ? (
                    <>
                      <h2 className="text-xl font-bold mb-2">
                        <Link href={`/articles/${featuredArticles[0].slug}`} className="text-blue-800 hover:text-blue-600">
                          {featuredArticles[0].title}
                        </Link>
                      </h2>
                      <p className="text-gray-700 text-sm mb-2">
                        {featuredArticles[0].excerpt || stripHtml(featuredArticles[0].content).substring(0, 200) + '...'}
                      </p>
                      <div className="flex items-center gap-4 text-xs text-gray-600">
                        <span>作者: {featuredArticles[0].author.username}</span>
                        <span>•</span>
                        <span>{formatDate(featuredArticles[0].publishedAt)}</span>
                        {featuredArticles[0].topic && (
                          <>
                            <span>•</span>
                            <Badge variant="secondary" style={{ backgroundColor: featuredArticles[0].topic.color + '20', color: featuredArticles[0].topic.color }}>
                              {featuredArticles[0].topic.name}
                            </Badge>
                          </>
                        )}
                      </div>
                    </>
                  ) : (
                    <p className="text-gray-600">暂无特色条目</p>
                  )}
                </div>
              </div>

              {/* News Section */}
              <div className="mb-6">
                <div className="flex items-center gap-2 mb-3">
                  <TrendingUp className="w-5 h-5 text-green-600" />
                  <span className="text-sm font-semibold text-gray-700">站内新闻</span>
                </div>
                <div className="bg-green-50 border border-green-200 rounded-lg p-4">
                  <ul className="space-y-2 text-sm">
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 mt-1">•</span>
                      <span>欢迎新来的编辑者！请查看<a href="/help:editing" className="text-blue-600 hover:underline">编辑指南</a></span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 mt-1">•</span>
                      <span>新的<a href="/topic:技术" className="text-blue-600 hover:underline">技术专题</a>已经创建，欢迎大家贡献内容</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-green-600 mt-1">•</span>
                      <span>网站正在进行维护，可能会影响访问速度</span>
                    </li>
                  </ul>
                </div>
              </div>

              {/* Did You Know */}
              <div>
                <div className="flex items-center gap-2 mb-3">
                  <Eye className="w-5 h-5 text-purple-600" />
                  <span className="text-sm font-semibold text-gray-700">你知道吗</span>
                </div>
                <div className="bg-purple-50 border border-purple-200 rounded-lg p-4">
                  <ul className="space-y-2 text-sm text-gray-700">
                    <li className="flex items-start gap-2">
                      <span className="text-purple-600 mt-1">•</span>
                      <span>鸟脱邦最初是由一群热爱分享的朋友创建的</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-purple-600 mt-1">•</span>
                      <span>最热门的条目是关于<a href="/articles:programming" className="text-blue-600 hover:underline">编程技巧</a>的</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-purple-600 mt-1">•</span>
                      <span>我们已经有超过 <span className="font-semibold">500</span> 位注册用户</span>
                    </li>
                  </ul>
                </div>
              </div>
            </div>

            {/* Sidebar */}
            <div className="lg:w-80">
              {/* Navigation */}
              <div className="bg-gray-100 rounded-lg p-4 mb-4">
                <h3 className="font-bold text-gray-900 mb-3">导航</h3>
                <ul className="space-y-2 text-sm">
                  <li><Link href="/explore" className="text-blue-600 hover:underline">浏览条目</Link></li>
                  <li><Link href="/topics" className="text-blue-600 hover:underline">主题分类</Link></li>
                  <li><Link href="/recent-changes" className="text-blue-600 hover:underline">最近更改</Link></li>
                  <li><Link href="/random" className="text-blue-600 hover:underline">随机条目</Link></li>
                  <li><Link href="/help" className="text-blue-600 hover:underline">帮助</Link></li>
                </ul>
              </div>

              {/* Topics */}
              <div className="bg-gray-100 rounded-lg p-4 mb-4">
                <div className="flex items-center justify-between mb-3">
                  <h3 className="font-bold text-gray-900">主题分类</h3>
                  <Link href="/topics" className="text-xs text-blue-600 hover:underline">更多</Link>
                </div>
                <div className="space-y-2">
                  {loading ? (
                    <div className="space-y-2">
                      {[...Array(5)].map((_, i) => (
                        <div key={i} className="h-6 bg-gray-300 rounded animate-pulse"></div>
                      ))}
                    </div>
                  ) : topics.slice(0, 5).map((topic) => (
                    <Link key={topic.id} href={`/topics/${topic.id}`} className="flex items-center justify-between text-sm hover:bg-gray-200 p-2 rounded">
                      <span>{topic.name}</span>
                      <Badge variant="secondary" className="text-xs">
                        {topic._count.articles}
                      </Badge>
                    </Link>
                  ))}
                </div>
              </div>

              {/* Stats */}
              <div className="bg-gray-100 rounded-lg p-4">
                <h3 className="font-bold text-gray-900 mb-3">统计信息</h3>
                <div className="space-y-2 text-sm text-gray-700">
                  <div className="flex justify-between">
                    <span>条目总数</span>
                    <span className="font-semibold">1,234</span>
                  </div>
                  <div className="flex justify-between">
                    <span>页面总数</span>
                    <span className="font-semibold">5,678</span>
                  </div>
                  <div className="flex justify-between">
                    <span>上传文件</span>
                    <span className="font-semibold">890</span>
                  </div>
                  <div className="flex justify-between">
                    <span>注册用户</span>
                    <span className="font-semibold">567</span>
                  </div>
                  <div className="flex justify-between">
                    <span>活跃用户</span>
                    <span className="font-semibold">89</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Recent Articles Grid */}
        <div className="bg-white rounded-lg shadow-sm p-6 mb-6 border border-gray-200">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-2xl font-bold text-gray-900">最新条目</h2>
            <Link href="/explore" className="text-sm text-blue-600 hover:underline">查看全部 →</Link>
          </div>
          
          {loading ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {[...Array(6)].map((_, i) => (
                <div key={i} className="border border-gray-200 rounded-lg p-4 animate-pulse">
                  <div className="h-6 bg-gray-300 rounded w-3/4 mb-2"></div>
                  <div className="h-4 bg-gray-300 rounded w-full mb-2"></div>
                  <div className="h-4 bg-gray-300 rounded w-2/3"></div>
                </div>
              ))}
            </div>
          ) : recentArticles.length > 0 ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              {recentArticles.map((article) => (
                <div key={article.id} className="border border-gray-200 rounded-lg p-4 hover:shadow-md transition-shadow">
                  <h3 className="font-bold text-lg mb-2">
                    <Link href={`/articles/${article.slug}`} className="text-blue-800 hover:text-blue-600">
                      {article.title}
                    </Link>
                  </h3>
                  <p className="text-gray-600 text-sm mb-3 line-clamp-2">
                    {article.excerpt || stripHtml(article.content).substring(0, 150) + '...'}
                  </p>
                  <div className="flex items-center justify-between text-xs text-gray-500">
                    <span>{formatDate(article.publishedAt)}</span>
                    <div className="flex items-center gap-2">
                      {article._count.comments > 0 && (
                        <>
                          <MessageSquare className="w-3 h-3" />
                          <span>{article._count.comments}</span>
                        </>
                      )}
                    </div>
                  </div>
                  {article.tags.length > 0 && (
                    <div className="flex flex-wrap gap-1 mt-2">
                      {article.tags.slice(0, 3).map(({ tag }) => (
                        <Badge key={tag.id} variant="outline" className="text-xs">
                          {tag.name}
                        </Badge>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <div className="text-center py-12">
              <FileText className="w-16 h-16 mx-auto text-gray-400 mb-4" />
              <h3 className="text-xl font-semibold text-gray-600 mb-2">还没有条目</h3>
              <p className="text-gray-500 mb-4">成为第一个创建条目的人吧！</p>
              <Link href="/create" className="btn-primary">
                <Edit className="mr-2 w-4 h-4" />
                创建条目
              </Link>
            </div>
          )}
        </div>

        {/* Contribute Section */}
        <div className="bg-gradient-to-r from-blue-600 to-purple-600 rounded-lg p-8 text-white text-center">
          <h2 className="text-3xl font-bold mb-4">为鸟托邦 Wiki 做贡献</h2>
          <p className="text-lg mb-6 max-w-2xl mx-auto">
            每个人都可以编辑。您的知识可以帮助他人，让这个世界变得更好。
          </p>
          <div className="flex flex-col sm:flex-row gap-4 justify-center">
            <Link href="/create" className="bg-white text-blue-600 px-6 py-3 rounded-lg font-semibold hover:bg-gray-100 transition-colors">
              <Edit className="inline mr-2 w-5 h-5" />
              创建新条目
            </Link>
            <Link href="/help:editing" className="bg-transparent border-2 border-white text-white px-6 py-3 rounded-lg font-semibold hover:bg-white hover:text-blue-600 transition-colors">
              <BookOpen className="inline mr-2 w-5 h-5" />
              了解如何编辑
            </Link>
          </div>
        </div>
      </main>
    </div>
  )
}