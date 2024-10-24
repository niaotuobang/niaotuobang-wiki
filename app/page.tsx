import Image from "next/image";

export default function Home() {
  return (
    <div className="bg-gray-100 flex flex-col min-h-screen">
      <nav className="bg-gray-800 p-4">
        <div className="container mx-auto flex justify-start items-center space-x-4">
          <div className="text-white text-lg font-bold">鸟托邦 Wiki</div>
          <a href="#" className="text-white px-4">文章</a>
          <a href="#" className="text-white px-4">话题</a>
          <a href="#" className="text-white px-4">Admin</a>
        </div>
      </nav>

      <main className="container mx-auto p-4 flex-grow">
        <h1 className="text-2xl font-bold mb-4">欢迎来到我的网站</h1>
        <p>这里是主要内容区域。</p>
      </main>

      <footer className="bg-gray-800 p-4">
        <div className="container mx-auto text-white text-center">
          版权所有 © 2024
        </div>
      </footer>
    </div >
  );
}
