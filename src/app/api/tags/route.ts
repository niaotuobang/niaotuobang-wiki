import { NextRequest, NextResponse } from "next/server"
import { getServerSession } from "next-auth"
import { authOptions } from "@/lib/auth"
import { prisma } from "@/lib/prisma"

export async function GET() {
  try {
    const tags = await prisma.tag.findMany({
      orderBy: {
        createdAt: "desc"
      },
      include: {
        _count: {
          select: {
            articles: true
          }
        }
      }
    })

    return NextResponse.json(tags)
  } catch (error) {
    console.error("Get tags error:", error)
    return NextResponse.json(
      { error: "服务器内部错误" },
      { status: 500 }
    )
  }
}

export async function POST(req: NextRequest) {
  try {
    const session = await getServerSession(authOptions)
    
    if (!session || (session.user.role !== "SUPER_ADMIN" && session.user.role !== "ADMIN")) {
      return NextResponse.json(
        { error: "没有权限创建标签" },
        { status: 403 }
      )
    }

    const { name, color } = await req.json()

    if (!name) {
      return NextResponse.json(
        { error: "标签名不能为空" },
        { status: 400 }
      )
    }

    // Check if tag already exists
    const existingTag = await prisma.tag.findUnique({
      where: { name }
    })

    if (existingTag) {
      return NextResponse.json(
        { error: "标签已存在" },
        { status: 400 }
      )
    }

    const tag = await prisma.tag.create({
      data: {
        name,
        color: color || "#10B981"
      }
    })

    return NextResponse.json(tag, { status: 201 })
  } catch (error) {
    console.error("Create tag error:", error)
    return NextResponse.json(
      { error: "服务器内部错误" },
      { status: 500 }
    )
  }
}