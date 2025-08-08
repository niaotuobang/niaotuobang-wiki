import { UserRole } from "@prisma/client"
import NextAuth from "next-auth"

declare module "next-auth" {
  interface Session {
    user: {
      id: string
      username: string
      email: string
      role: UserRole
      avatar?: string
    }
  }

  interface User {
    id: string
    username: string
    email: string
    role: UserRole
    avatar?: string
  }
}

declare module "next-auth/jwt" {
  interface JWT {
    role: UserRole
    username: string
  }
}