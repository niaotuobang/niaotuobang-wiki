import { NextRequest, NextResponse } from "next/server"
import { getServerSession } from "next-auth"
import { authOptions } from "@/lib/auth"
import { prisma } from "@/lib/prisma"
import { ArticleStatus } from "@prisma/client"

export async function GET(
  req: NextRequest,
  { params }: { params: { slug: string } }
) {
  try {
    const { slug } = params

    const article = await prisma.article.findUnique({
      where: { slug },
      include: {
        author: {
          select: {
            id: true,
            username: true,
            avatar: true,
            bio: true
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
          orderBy: {
            createdAt: "asc"
          }
        },
        _count: {
          select: {
            comments: true
          }
        }
      }
    })

    if (!article) {
      return NextResponse.json(
        { error: "词条不存在" },
        { status: 404 }
      )
    }

    // Increment view count
    await prisma.article.update({
      where: { id: article.id },
      data: {
        viewCount: {
          increment: 1
        }
      }
    })

    return NextResponse.json(article)
  } catch (error) {
    console.error("Get article error:", error)
    return NextResponse.json(
      { error: "服务器内部错误" },
      { status: 500 }
    )
  }
}

export async function PUT(
  req: NextRequest,
  { params }: { params: { slug: string } }
) {
  try {
    const session = await getServerSession(authOptions)
    
    if (!session) {
      return NextResponse.json(
        { error: "请先登录" },
        { status: 401 }
      )
    }

    const { slug } = params
    const { title, content, excerpt, topicId, tagIds, isPublic } = await req.json()

    // Find the article
    const article = await prisma.article.findUnique({
      where: { slug }
    })

    if (!article) {
      return NextResponse.json(
        { error: "词条不存在" },
        { status: 404 }
      )
    }

    // Check permissions
    if (session.user.role !== "SUPER_ADMIN" && 
        session.user.role !== "ADMIN" && 
        article.authorId !== session.user.id) {
      return NextResponse.json(
        { error: "没有权限修改此词条" },
        { status: 403 }
      )
    }

    // Update article
    const updatedArticle = await prisma.article.update({
      where: { id: article.id },
      data: {
        title,
        content,
        excerpt,
        topicId,
        isPublic,
        tags: tagIds ? {
          deleteMany: {},
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

    // Create new version if content changed
    if (content !== article.content) {
      await prisma.articleVersion.create({
        data: {
          articleId: article.id,
          content,
          editorId: session.user.id
        }
      })
    }

    return NextResponse.json(updatedArticle)
  } catch (error) {
    console.error("Update article error:", error)
    return NextResponse.json(
      { error: "服务器内部错误" },
      { status: 500 }
    )
  }
}

export async function DELETE(
  req: NextRequest,
  { params }: { params: { slug: string } }
) {
  try {
    const session = await getServerSession(authOptions)
    
    if (!session) {
      return NextResponse.json(
        { error: "请先登录" },
        { status: 401 }
      )
    }

    const { slug } = params

    // Find the article
    const article = await prisma.article.findUnique({
      where: { slug }
    })

    if (!article) {
      return NextResponse.json(
        { error: "词条不存在" },
        { status: 404 }
      )
    }

    // Check permissions
    if (session.user.role !== "SUPER_ADMIN" && 
        session.user.role !== "ADMIN" && 
        article.authorId !== session.user.id) {
      return NextResponse.json(
        { error: "没有权限删除此词条" },
        { status: 403 }
      )
    }

    // Delete article
    await prisma.article.delete({
      where: { id: article.id }
    })

    return NextResponse.json({ message: "删除成功" })
  } catch (error) {
    console.error("Delete article error:", error)
    return NextResponse.json(
      { error: "服务器内部错误" },
      { status: 500 }
    )
  }
}