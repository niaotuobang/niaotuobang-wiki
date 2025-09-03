import NextAuth from "next-auth"
import { config } from "../../auth.config"

export const {
  handlers: { GET, POST },
  auth,
  signIn,
  signOut,
} = NextAuth(config)