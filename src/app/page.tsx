import { Navbar } from "@/components/navbar"
import Link from "next/link"
import { Button } from "@/components/ui/button"
import { BookOpen, Users, Search, Edit } from "lucide-react"

export default function Home() {
  return (
    <div className="min-h-screen bg-gray-50">
      <Navbar />
      
      <main className="container mx-auto px-4 py-12">
        {/* Hero Section */}
        <section className="text-center mb-16">
          <h1 className="text-4xl md:text-6xl font-bold text-gray-900 mb-6">
            鸟脱邦 <span className="text-blue-600">Wiki</span>
          </h1>
          <p className="text-xl text-gray-600 mb-8 max-w-2xl mx-auto">
            记录群聊中的每个成员，留存群聊文化记忆。共建属于你们的知识库。
          </p>
          <div className="flex gap-4 justify-center">
            <Link href="/auth/signup">
              <Button size="lg">开始创建</Button>
            </Link>
            <Link href="/explore">
              <Button variant="outline" size="lg">浏览词条</Button>
            </Link>
          </div>
        </section>

        {/* Features */}
        <section className="grid md:grid-cols-3 gap-8 mb-16">
          <div className="bg-white p-6 rounded-lg shadow-sm">
            <div className="w-12 h-12 bg-blue-100 rounded-lg flex items-center justify-center mb-4">
              <Users className="h-6 w-6 text-blue-600" />
            </div>
            <h3 className="text-lg font-semibold mb-2">人物画像</h3>
            <p className="text-gray-600">
              为群成员创建专属词条，记录特点、经历和经典语录
            </p>
          </div>
          
          <div className="bg-white p-6 rounded-lg shadow-sm">
            <div className="w-12 h-12 bg-green-100 rounded-lg flex items-center justify-center mb-4">
              <Edit className="h-6 w-6 text-green-600" />
            </div>
            <h3 className="text-lg font-semibold mb-2">协作编辑</h3>
            <p className="text-gray-600">
              多人共同编辑，版本历史记录，让内容更加完善
            </p>
          </div>
          
          <div className="bg-white p-6 rounded-lg shadow-sm">
            <div className="w-12 h-12 bg-purple-100 rounded-lg flex items-center justify-center mb-4">
              <Search className="h-6 w-6 text-purple-600" />
            </div>
            <h3 className="text-lg font-semibold mb-2">快速搜索</h3>
            <p className="text-gray-600">
              强大的搜索功能，快速找到需要的词条和内容
            </p>
          </div>
        </section>

        {/* Recent Articles */}
        <section className="bg-white rounded-lg p-8 shadow-sm">
          <h2 className="text-2xl font-bold mb-6">最新词条</h2>
          <div className="space-y-4">
            {/* Placeholder for recent articles */}
            <div className="text-gray-500 text-center py-8">
              <BookOpen className="h-12 w-12 mx-auto mb-4 text-gray-300" />
              <p>还没有词条，成为第一个创建者吧！</p>
            </div>
          </div>
        </section>
      </main>
    </div>
  )
}
