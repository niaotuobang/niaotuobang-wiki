import { NextRequest, NextResponse } from "next/server"
import { getServerSession } from "next-auth"
import { authOptions } from "@/lib/auth"
import { prisma } from "@/lib/prisma"

export async function GET() {
  try {
    const topics = await prisma.topic.findMany({
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

    return NextResponse.json(topics)
  } catch (error) {
    console.error("Get topics error:", error)
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
        { error: "没有权限创建话题" },
        { status: 403 }
      )
    }

    const { name, description, color, icon } = await req.json()

    if (!name) {
      return NextResponse.json(
        { error: "话题名不能为空" },
        { status: 400 }
      )
    }

    // Check if topic already exists
    const existingTopic = await prisma.topic.findUnique({
      where: { name }
    })

    if (existingTopic) {
      return NextResponse.json(
        { error: "话题已存在" },
        { status: 400 }
      )
    }

    const topic = await prisma.topic.create({
      data: {
        name,
        description,
        color: color || "#3B82F6",
        icon
      }
    })

    return NextResponse.json(topic, { status: 201 })
  } catch (error) {
    console.error("Create topic error:", error)
    return NextResponse.json(
      { error: "服务器内部错误" },
      { status: 500 }
    )
  }
}