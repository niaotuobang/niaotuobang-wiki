import { NextRequest, NextResponse } from "next/server"
import { getServerSession } from "next-auth"
import { authOptions } from "@/lib/auth"
import { prisma } from "@/lib/prisma"

export async function DELETE(
  req: NextRequest,
  { params }: { params: { slug: string; shareId: string } }
) {
  try {
    const session = await getServerSession(authOptions)
    
    if (!session) {
      return NextResponse.json(
        { error: "请先登录" },
        { status: 401 }
      )
    }

    const { slug, shareId } = params

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

    // Check if user has permission to remove share
    const canRemove = session.user.role === "SUPER_ADMIN" || 
                     session.user.role === "ADMIN" || 
                     article.authorId === session.user.id

    if (!canRemove) {
      return NextResponse.json(
        { error: "没有删除权限" },
        { status: 403 }
      )
    }

    // Delete the share
    await prisma.articleShare.delete({
      where: { id: shareId }
    })

    return NextResponse.json({ success: true })

  } catch (error) {
    console.error("Delete share error:", error)
    return NextResponse.json(
      { error: "删除失败" },
      { status: 500 }
    )
  }
}