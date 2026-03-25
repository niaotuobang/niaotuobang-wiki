import { z } from "zod";

export const LoginSchema = z.object({
  email: z.string().email({ message: "请输入有效的邮箱地址" }),
  password: z.string().min(6, { message: "密码至少需要6个字符" }),
});

export const RegisterSchema = z.object({
  username: z.string().min(2, { message: "用户名至少需要2个字符" }).max(20, { message: "用户名最多20个字符" }),
  email: z.string().email({ message: "请输入有效的邮箱地址" }),
  password: z.string().min(6, { message: "密码至少需要6个字符" }),
  confirmPassword: z.string(),
}).refine((data) => data.password === data.confirmPassword, {
  message: "两次输入的密码不一致",
  path: ["confirmPassword"],
});

export const CreateArticleSchema = z.object({
  title: z.string().min(1, { message: "标题不能为空" }).max(100, { message: "标题最多100个字符" }),
  content: z.string().min(1, { message: "内容不能为空" }),
  topicId: z.string().optional(),
  tags: z.array(z.string()).optional(),
});

export const CommentSchema = z.object({
  content: z.string().min(1, { message: "评论内容不能为空" }).max(1000, { message: "评论最多1000个字符" }),
});

export const ShareSchema = z.object({
  email: z.string().email({ message: "请输入有效的邮箱地址" }).optional().or(z.literal("")),
  permission: z.enum(["VIEW", "COMMENT", "EDIT"]),
  expiresAt: z.string().optional(),
});

export type LoginFormData = z.infer<typeof LoginSchema>;
export type RegisterFormData = z.infer<typeof RegisterSchema>;
export type CreateArticleFormData = z.infer<typeof CreateArticleSchema>;
export type CommentFormData = z.infer<typeof CommentSchema>;
export type ShareFormData = z.infer<typeof ShareSchema>;