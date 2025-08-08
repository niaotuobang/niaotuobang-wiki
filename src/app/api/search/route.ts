import { NextRequest, NextResponse } from "next/server"
import { prisma } from "@/lib/prisma"

export async function GET(req: NextRequest) {
  try {
    const { searchParams } = new URL(req.url)
    const query = searchParams.get("q")?.trim()
    const page = parseInt(searchParams.get("page") || "1")
    const limit = parseInt(searchParams.get("limit") || "10")
    const topicId = searchParams.get("topicId")
    const tagId = searchParams.get("tagId")

    if (!query) {
      return NextResponse.json(
        { error: "搜索关键词不能为空" },
        { status: 400 }
      )
    }

    const skip = (page - 1) * limit

    // Build where clause
    const where: any = {
      OR: [
        {
          title: {
            contains: query,
            mode: "insensitive" as const
          }
        },
        {
          content: {
            contains: query,
            mode: "insensitive" as const
          }
        },
        {
          excerpt: {
            contains: query,
            mode: "insensitive" as const
          }
        }
      ],
      status: "PUBLISHED"
    }

    // Add topic filter if provided
    if (topicId) {
      where.topicId = topicId
    }

    // Add tag filter if provided
    if (tagId) {
      where.tags = {
        some: {
          tagId: tagId
        }
      }
    }

    // Get search results
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
              color: true,
              icon: true
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
        orderBy: [
          { viewCount: "desc" },
          { updatedAt: "desc" }
        ],
        skip,
        take: limit
      }),
      prisma.article.count({ where })
    ])

    // Get topic and tag suggestions
    const [topics, tags] = await Promise.all([
      prisma.topic.findMany({
        where: {
          name: {
            contains: query,
            mode: "insensitive" as const
          }
        },
        take: 5
      }),
      prisma.tag.findMany({
        where: {
          name: {
            contains: query,
            mode: "insensitive" as const
          }
        },
        take: 5
      })
    ])

    return NextResponse.json({
      articles,
      topics,
      tags,
      pagination: {
        page,
        limit,
        total,
        pages: Math.ceil(total / limit)
      }
    })

  } catch (error) {
    console.error("Search error:", error)
    return NextResponse.json(
      { error: "搜索失败" },
      { status: 500 }
    )
  }
}