import { NextRequest, NextResponse } from "next/server"
import { getServerSession } from "next-auth"
import { authOptions } from "@/lib/auth"
import { prisma } from "@/lib/prisma"
import { ArticleStatus } from "@prisma/client"
import { generateSlug } from "@/lib/utils"

export async function POST(req: NextRequest) {
  try {
    const session = await getServerSession(authOptions)
    
    if (!session) {
      return NextResponse.json(
        { error: "请先登录" },
        { status: 401 }
      )
    }

    const { title, content, excerpt, topicId, tagIds, isPublic } = await req.json()

    if (!title || !content) {
      return NextResponse.json(
        { error: "标题和内容不能为空" },
        { status: 400 }
      )
    }

    // Generate slug from title
    const slug = generateSlug(title)

    // Check if slug already exists
    const existingArticle = await prisma.article.findUnique({
      where: { slug }
    })

    if (existingArticle) {
      return NextResponse.json(
        { error: "已存在相同标题的词条" },
        { status: 400 }
      )
    }

    // Create article
    const article = await prisma.article.create({
      data: {
        title,
        slug,
        content,
        excerpt,
        authorId: session.user.id,
        topicId,
        status: ArticleStatus.PUBLISHED,
        isPublic: isPublic ?? true,
        publishedAt: new Date(),
        tags: tagIds ? {
          create: tagIds.map((tagId: string) => ({
            tag: {
              connect: { id: tagId }
            }
          }))
        } : undefined
      },
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
            color: true
          }
        },
        tags: {
          include: {
            tag: true
          }
        }
      }
    })

    // Create initial version
    await prisma.articleVersion.create({
      data: {
        articleId: article.id,
        content,
        editorId: session.user.id
      }
    })

    return NextResponse.json(article, { status: 201 })
  } catch (error) {
    console.error("Create article error:", error)
    return NextResponse.json(
      { error: "服务器内部错误" },
      { status: 500 }
    )
  }
}

export async function GET(req: NextRequest) {
  try {
    const { searchParams } = new URL(req.url)
    const page = parseInt(searchParams.get("page") || "1")
    const limit = parseInt(searchParams.get("limit") || "10")
    const topicId = searchParams.get("topicId")
    const tagId = searchParams.get("tagId")
    const search = searchParams.get("search")

    const skip = (page - 1) * limit

    const where: any = {
      status: ArticleStatus.PUBLISHED,
      isPublic: true
    }

    if (topicId) {
      where.topicId = topicId
    }

    if (tagId) {
      where.tags = {
        some: {
          tagId
        }
      }
    }

    if (search) {
      where.OR = [
        { title: { contains: search, mode: "insensitive" } },
        { content: { contains: search, mode: "insensitive" } },
        { excerpt: { contains: search, mode: "insensitive" } }
      ]
    }

    const [articles, total] = await Promise.all([
      prisma.article.findMany({
        where,
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
              color: true
            }
          },
          tags: {
            include: {
              tag: true
            }
          },
          _count: {
            select: {
              comments: true
            }
          }
        },
        orderBy: {
          publishedAt: "desc"
        },
        skip,
        take: limit
      }),
      prisma.article.count({ where })
    ])

    return NextResponse.json({
      articles,
      pagination: {
        page,
        limit,
        total,
        pages: Math.ceil(total / limit)
      }
    })
  } catch (error) {
    console.error("Get articles error:", error)
    return NextResponse.json(
      { error: "服务器内部错误" },
      { status: 500 }
    )
  }
}