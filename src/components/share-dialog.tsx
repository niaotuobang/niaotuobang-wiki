"use client"

import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Badge } from "@/components/ui/badge"
import { Share2, Copy, Mail, Link, Calendar, X } from "lucide-react"
import { format } from "date-fns"
import { zhCN } from "date-fns/locale"

interface Share {
  id: string
  email?: string
  permission: "VIEW" | "EDIT" | "COMMENT"
  token?: string
  expiresAt?: string
  createdAt: string
  user?: {
    id: string
    username: string
    email: string
  }
}

interface ShareDialogProps {
  articleSlug: string
  shares: Share[]
  onShare: (data: {
    email?: string
    permission: "VIEW" | "EDIT" | "COMMENT"
    expiresAt?: string
  }) => Promise<void>
  onRemoveShare: (shareId: string) => Promise<void>
  children: React.ReactNode
}

export function ShareDialog({ 
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  articleSlug, 
  shares, 
  onShare, 
  onRemoveShare, 
  children 
}: ShareDialogProps) {
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [formData, setFormData] = useState<{
    email: string
    permission: "VIEW" | "EDIT" | "COMMENT"
    expiresAt: string
  }>({
    email: "",
    permission: "VIEW",
    expiresAt: ""
  })

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    
    try {
      await onShare({
        email: formData.email || undefined,
        permission: formData.permission,
        expiresAt: formData.expiresAt || undefined
      })
      
      // Reset form
      setFormData({
        email: "",
        permission: "VIEW",
        expiresAt: ""
      })
      
      setOpen(false)
    } catch (error) {
      console.error("Share error:", error)
    } finally {
      setLoading(false)
    }
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
  }

  const getShareLink = (token: string) => {
    return `${window.location.origin}/shared/${token}`
  }

  const getPermissionLabel = (permission: string) => {
    switch (permission) {
      case "VIEW": return "查看"
      case "EDIT": return "编辑"
      case "COMMENT": return "评论"
      default: return permission
    }
  }

  const getPermissionColor = (permission: string) => {
    switch (permission) {
      case "VIEW": return "bg-blue-100 text-blue-800"
      case "EDIT": return "bg-green-100 text-green-800"
      case "COMMENT": return "bg-yellow-100 text-yellow-800"
      default: return "bg-gray-100 text-gray-800"
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {children}
      </DialogTrigger>
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Share2 className="h-5 w-5" />
            分享词条
          </DialogTitle>
          <DialogDescription>
            与他人分享此词条，并设置访问权限
          </DialogDescription>
        </DialogHeader>

        {/* Share Form */}
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="email">邮箱地址（可选）</Label>
            <Input
              id="email"
              type="email"
              value={formData.email}
              onChange={(e) => setFormData(prev => ({ ...prev, email: e.target.value }))}
              placeholder="输入邮箱地址或留空生成公开链接"
            />
          </div>

          <div className="space-y-2">
            <Label>权限</Label>
            <Select
              value={formData.permission}
              onValueChange={(value) => setFormData(prev => ({ ...prev, permission: value as "VIEW" | "EDIT" | "COMMENT" }))}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="VIEW">仅查看</SelectItem>
                <SelectItem value="COMMENT">查看和评论</SelectItem>
                <SelectItem value="EDIT">查看、评论和编辑</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="expiresAt">过期时间（可选）</Label>
            <Input
              id="expiresAt"
              type="datetime-local"
              value={formData.expiresAt}
              onChange={(e) => setFormData(prev => ({ ...prev, expiresAt: e.target.value }))}
            />
          </div>

          <DialogFooter>
            <Button type="submit" disabled={loading}>
              {loading ? "创建中..." : "创建分享"}
            </Button>
          </DialogFooter>
        </form>

        {/* Existing Shares */}
        {shares.length > 0 && (
          <div className="mt-6 border-t pt-6">
            <h3 className="text-sm font-medium mb-3">已分享的用户</h3>
            <div className="space-y-2 max-h-60 overflow-y-auto">
              {shares.map((share) => (
                <div key={share.id} className="flex items-center justify-between p-3 bg-gray-50 rounded-lg">
                  <div className="flex items-center gap-3 flex-1">
                    <div className="flex items-center gap-2">
                      {share.user ? (
                        <Mail className="h-4 w-4 text-gray-500" />
                      ) : (
                        <Link className="h-4 w-4 text-gray-500" />
                      )}
                      <span className="text-sm">
                        {share.user ? `${share.user.username} (${share.user.email})` : share.email || "公开链接"}
                      </span>
                    </div>
                    <Badge className={getPermissionColor(share.permission)}>
                      {getPermissionLabel(share.permission)}
                    </Badge>
                    {share.expiresAt && (
                      <div className="flex items-center gap-1 text-xs text-gray-500">
                        <Calendar className="h-3 w-3" />
                        <span>
                          {format(new Date(share.expiresAt), "MM/dd", { locale: zhCN })}
                        </span>
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    {share.token && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => share.token && copyToClipboard(getShareLink(share.token))}
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => onRemoveShare(share.id)}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}