import { NextRequest, NextResponse } from "next/server"
import { getServerSession } from "next-auth"
import { authOptions } from "@/lib/auth"
import { prisma } from "@/lib/prisma"
import { v4 as uuidv4 } from "uuid"

export async function POST(
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
    const { email, permission = "VIEW", expiresAt } = await req.json()

    // Find the article
    const article = await prisma.article.findUnique({
      where: { slug },
      include: {
        author: true
      }
    })

    if (!article) {
      return NextResponse.json(
        { error: "词条不存在" },
        { status: 404 }
      )
    }

    // Check if user has permission to share
    const canShare = session.user.role === "SUPER_ADMIN" || 
                   session.user.role === "ADMIN" || 
                   article.authorId === session.user.id

    if (!canShare) {
      return NextResponse.json(
        { error: "没有分享权限" },
        { status: 403 }
      )
    }

    // If sharing with email
    if (email) {
      // Check if user exists
      const targetUser = await prisma.user.findUnique({
        where: { email }
      })

      if (targetUser) {
        // Create share for existing user
        const existingShare = await prisma.articleShare.findFirst({
          where: {
            articleId: article.id,
            userId: targetUser.id
          }
        })

        if (existingShare) {
          return NextResponse.json(
            { error: "已经与该用户分享了此词条" },
            { status: 400 }
          )
        }

        const share = await prisma.articleShare.create({
          data: {
            articleId: article.id,
            userId: targetUser.id,
            permission,
            expiresAt: expiresAt ? new Date(expiresAt) : null
          },
          include: {
            user: {
              select: {
                id: true,
                username: true,
                email: true
              }
            }
          }
        })

        return NextResponse.json(share)
      } else {
        // Create share with token for non-user
        const token = uuidv4()
        const share = await prisma.articleShare.create({
          data: {
            articleId: article.id,
            email,
            permission,
            token,
            expiresAt: expiresAt ? new Date(expiresAt) : null
          }
        })

        // TODO: Send email with share link
        return NextResponse.json({
          ...share,
          shareLink: `${process.env.NEXTAUTH_URL}/shared/${token}`
        })
      }
    } else {
      // Create public share link
      const token = uuidv4()
      const share = await prisma.articleShare.create({
        data: {
          articleId: article.id,
          permission,
          token,
          expiresAt: expiresAt ? new Date(expiresAt) : null
        }
      })

      return NextResponse.json({
        ...share,
        shareLink: `${process.env.NEXTAUTH_URL}/shared/${token}`
      })
    }

  } catch (error) {
    console.error("Create share error:", error)
    return NextResponse.json(
      { error: "分享失败" },
      { status: 500 }
    )
  }
}

export async function GET(
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

    // Check if user has permission to view shares
    const canView = session.user.role === "SUPER_ADMIN" || 
                   session.user.role === "ADMIN" || 
                   article.authorId === session.user.id

    if (!canView) {
      return NextResponse.json(
        { error: "没有查看权限" },
        { status: 403 }
      )
    }

    // Get all shares for this article
    const shares = await prisma.articleShare.findMany({
      where: { articleId: article.id },
      include: {
        user: {
          select: {
            id: true,
            username: true,
            email: true
          }
        }
      },
      orderBy: { createdAt: "desc" }
    })

    return NextResponse.json(shares)

  } catch (error) {
    console.error("Get shares error:", error)
    return NextResponse.json(
      { error: "获取分享列表失败" },
      { status: 500 }
    )
  }
}