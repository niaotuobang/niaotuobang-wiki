"use server";

import { auth } from "@/lib/auth";
import { prisma } from "@/lib/prisma";
import { revalidatePath } from "next/cache";
import { CreateArticleSchema, CommentSchema, ShareSchema } from "@/lib/validations";
import { z } from "zod";
import { generateSlug } from "@/lib/utils";
import { ApiError } from "@/lib/errorHandler";

export async function createArticle(values: z.infer<typeof CreateArticleSchema>) {
  const session = await auth();
  if (!session?.user?.id) {
    throw new ApiError(401, "Unauthorized");
  }

  const validatedFields = CreateArticleSchema.safeParse(values);
  if (!validatedFields.success) {
    throw new ApiError(400, "Invalid input");
  }

  const { title, content, topicId, tags } = validatedFields.data;
  const slug = generateSlug(title);

  try {
    const article = await prisma.article.create({
      data: {
        title,
        slug,
        content,
        authorId: session.user.id,
        topicId: topicId || null,
        tags: tags ? {
          create: tags.map(tagId => ({
            tag: {
              connect: { id: tagId }
            }
          }))
        } : undefined,
      },
      include: {
        author: {
          select: {
            id: true,
            username: true,
            avatar: true,
          },
        },
        topic: true,
        tags: {
          include: {
            tag: true,
          },
        },
      },
    });

    revalidatePath("/");
    return { success: true, article };
  } catch (error) {
    console.error("Error creating article:", error);
    throw new ApiError(500, "Failed to create article");
  }
}

export async function addComment(articleId: string, values: z.infer<typeof CommentSchema>) {
  const session = await auth();
  if (!session?.user?.id) {
    throw new ApiError(401, "Unauthorized");
  }

  const validatedFields = CommentSchema.safeParse(values);
  if (!validatedFields.success) {
    throw new ApiError(400, "Invalid input");
  }

  const { content } = validatedFields.data;

  try {
    const comment = await prisma.comment.create({
      data: {
        content,
        authorId: session.user.id,
        articleId,
      },
      include: {
        author: {
          select: {
            id: true,
            username: true,
            avatar: true,
          },
        },
      },
    });

    revalidatePath(`/articles/${articleId}`);
    return { success: true, comment };
  } catch (error) {
    console.error("Error adding comment:", error);
    throw new ApiError(500, "Failed to add comment");
  }
}

export async function shareArticle(articleSlug: string, values: z.infer<typeof ShareSchema>) {
  const session = await auth();
  if (!session?.user?.id) {
    throw new ApiError(401, "Unauthorized");
  }

  const validatedFields = ShareSchema.safeParse(values);
  if (!validatedFields.success) {
    throw new ApiError(400, "Invalid input");
  }

  const { email, permission, expiresAt } = validatedFields.data;

  try {
    const article = await prisma.article.findUnique({
      where: { slug: articleSlug },
    });

    if (!article) {
      throw new ApiError(404, "Article not found");
    }

    if (article.authorId !== session.user.id) {
      throw new ApiError(403, "Forbidden");
    }

    const token = Math.random().toString(36).substring(2, 15);

    const share = await prisma.articleShare.create({
      data: {
        articleId: article.id,
        email: email || null,
        permission,
        token,
        expiresAt: expiresAt ? new Date(expiresAt) : null,
      },
    });

    if (email) {
      // TODO: Send email with share link
    }

    return { success: true, share };
  } catch (error) {
    console.error("Error sharing article:", error);
    if (error instanceof ApiError) {
      throw error;
    }
    throw new ApiError(500, "Failed to share article");
  }
}

export async function deleteComment(commentId: string) {
  const session = await auth();
  if (!session?.user?.id) {
    throw new ApiError(401, "Unauthorized");
  }

  try {
    const comment = await prisma.comment.findUnique({
      where: { id: commentId },
    });

    if (!comment) {
      throw new ApiError(404, "Comment not found");
    }

    if (comment.authorId !== session.user.id) {
      throw new ApiError(403, "Forbidden");
    }

    await prisma.comment.delete({
      where: { id: commentId },
    });

    revalidatePath(`/articles/${comment.articleId}`);
    return { success: true };
  } catch (error) {
    console.error("Error deleting comment:", error);
    if (error instanceof ApiError) {
      throw error;
    }
    throw new ApiError(500, "Failed to delete comment");
  }
}