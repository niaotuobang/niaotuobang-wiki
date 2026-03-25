import { NextRequest, NextResponse } from "next/server"
import { auth } from "@/lib/auth"
import { prisma } from "@/lib/prisma"

export async function DELETE(
  req: NextRequest,
  { params }: { params: Promise<{ slug: string; shareId: string }> }
) {
  try {
    const { slug, shareId } = await params
    const session = await auth()
    
    if (!session) {
      return NextResponse.json(
        { error: "请先登录" },
        { status: 401 }
      )
    }

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