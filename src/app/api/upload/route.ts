import { NextRequest, NextResponse } from "next/server"
import { getServerSession } from "next-auth"
import { authOptions } from "@/lib/auth"
import { prisma } from "@/lib/prisma"
import { writeFile, mkdir } from "fs/promises"
import path from "path"
import { v4 as uuidv4 } from "uuid"

export async function POST(req: NextRequest) {
  try {
    const session = await getServerSession(authOptions)
    
    if (!session) {
      return NextResponse.json(
        { error: "请先登录" },
        { status: 401 }
      )
    }

    const data = await req.formData()
    const file: File | null = data.get("file") as unknown as File
    
    if (!file) {
      return NextResponse.json(
        { error: "没有找到文件" },
        { status: 400 }
      )
    }

    // Validate file type
    const allowedTypes = [
      "image/jpeg",
      "image/png",
      "image/gif",
      "image/webp",
      "video/mp4",
      "video/webm",
      "video/quicktime"
    ]
    
    if (!allowedTypes.includes(file.type)) {
      return NextResponse.json(
        { error: "不支持的文件类型" },
        { status: 400 }
      )
    }

    // Validate file size (max 10MB)
    const maxSize = 10 * 1024 * 1024 // 10MB
    if (file.size > maxSize) {
      return NextResponse.json(
        { error: "文件大小不能超过10MB" },
        { status: 400 }
      )
    }

    // Generate unique filename
    const bytes = await file.arrayBuffer()
    const buffer = Buffer.from(bytes)
    
    const fileExtension = path.extname(file.name)
    const uniqueFilename = `${uuidv4()}${fileExtension}`
    
    // Create uploads directory if it doesn't exist
    const uploadDir = path.join(process.cwd(), "public", "uploads")
    await mkdir(uploadDir, { recursive: true })
    
    // Determine subdirectory based on file type
    let subDir = "others"
    if (file.type.startsWith("image/")) {
      subDir = "images"
    } else if (file.type.startsWith("video/")) {
      subDir = "videos"
    }
    
    const finalDir = path.join(uploadDir, subDir)
    await mkdir(finalDir, { recursive: true })
    
    // Write file to disk
    const filePath = path.join(finalDir, uniqueFilename)
    await writeFile(filePath, buffer)
    
    // Create media record in database
    const media = await prisma.media.create({
      data: {
        filename: uniqueFilename,
        originalName: file.name,
        mimeType: file.type,
        size: file.size,
        url: `/uploads/${subDir}/${uniqueFilename}`,
        uploaderId: session.user.id
      }
    })

    return NextResponse.json({
      id: media.id,
      url: media.url,
      filename: media.originalName,
      size: media.size,
      mimeType: media.mimeType
    })

  } catch (error) {
    console.error("File upload error:", error)
    return NextResponse.json(
      { error: "文件上传失败" },
      { status: 500 }
    )
  }
}