# Tailwind CSS v4 使用方案与配置

## 概述

玩盒工坊项目采用了 Tailwind CSS v4 作为样式框架，这是 Tailwind CSS 的最新版本，带来了更简洁的配置方式和更好的性能。

## 依赖安装

### package.json 配置

```json
{
  "devDependencies": {
    "@tailwindcss/postcss": "^4",
    "tailwindcss": "^4"
  }
}
```

## CSS 配置

### 全局样式文件 (src/app/globals.css)

```css
@import "tailwindcss";

:root {
  --background: #ffffff;
  --foreground: #171717;
}

@theme inline {
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --font-sans: var(--font-geist-sans);
  --font-mono: var(--font-geist-mono);
}

@media (prefers-color-scheme: dark) {
  :root {
    --background: #0a0a0a;
    --foreground: #ededed;
  }
}

body {
  background: var(--background);
  color: var(--foreground);
  font-family: Arial, Helvetica, sans-serif;
}
```

### Tailwind CSS v4 的新特性

1. **简化的导入方式**
   - 使用 `@import "tailwindcss"` 替代 v3 的多个 `@import` 语句
   - 自动包含所有必要的样式层

2. **内联主题配置**
   - 使用 `@theme inline` 指令直接在 CSS 中配置主题
   - 支持自定义颜色、字体等设计令牌

3. **CSS 变量集成**
   - 原生支持 CSS 变量作为设计令牌
   - 便于实现动态主题切换

## 实际应用示例

### 1. 基础布局组件

```tsx
// AdminHeader.tsx
<header className="bg-gray-800 text-white shadow-md">
  <nav className="container mx-auto px-6 py-3 flex justify-between items-center">
    <div className="flex items-center space-x-8">
      <Link href="/admin" className="text-xl font-bold">
        Admin Panel
      </Link>
      <div className="hidden md:flex items-center space-x-4">
        {navLinks.map(link => (
          <Link 
            key={link.name} 
            href={link.href} 
            className={`px-3 py-2 rounded-md text-sm font-medium ${
              pathname === link.href 
                ? 'bg-gray-900' 
                : 'hover:bg-gray-700'
            }`}
          >
            {link.name}
          </Link>
        ))}
      </div>
    </div>
  </nav>
</header>
```

### 2. 表单组件样式

```tsx
// DynamicForm.tsx
<div className="mb-4">
  <label className="block text-sm font-medium text-gray-700 mb-2">
    {field.label}
  </label>
  <input
    type={field.type}
    value={fieldValue as string}
    onChange={(e) => handleFormChange(field.key, e.target.value)}
    className="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
  />
</div>
```

### 3. 图片上传组件

```tsx
// 图片预览和操作
<div className="relative inline-block group">
  <Image 
    src={fieldValue as string} 
    alt={field.label} 
    className={`max-w-full h-auto ${
      imageField.mask_shape === 'circle' 
        ? 'rounded-full object-cover' 
        : ''
    }`}
    width={imageField.dimensions?.width || 128}
    height={imageField.dimensions?.height || 128}
  />
  <button
    type="button"
    onClick={() => handleFormChange(field.key, '')}
    className="absolute -top-2 -right-2 bg-red-500 text-white rounded-full w-6 h-6 flex items-center justify-center text-xs opacity-0 group-hover:opacity-100 transition-opacity shadow-lg hover:bg-red-600"
  >
    ×
  </button>
</div>
```

### 4. 响应式设计

```tsx
// 移动端适配
<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
  {items.map(item => (
    <div key={item.id} className="bg-white rounded-lg shadow-md p-4">
      <h3 className="text-lg font-semibold mb-2">{item.title}</h3>
      <p className="text-gray-600 text-sm">{item.description}</p>
    </div>
  ))}
</div>
```

## 主题系统

### 明暗主题支持

项目通过 CSS 变量和媒体查询实现了明暗主题的自动切换：

```css
/* 默认亮色主题 */
:root {
  --background: #ffffff;
  --foreground: #171717;
}

/* 暗色主题 */
@media (prefers-color-scheme: dark) {
  :root {
    --background: #0a0a0a;
    --foreground: #ededed;
  }
}
```

### 自定义颜色使用

```tsx
// 使用自定义的背景和前景色
<div className="bg-[var(--background)] text-[var(--foreground)]">
  内容会根据系统主题自动切换颜色
</div>
```

## 最佳实践

### 1. 响应式断点

- `sm:` 640px 及以上
- `md:` 768px 及以上
- `lg:` 1024px 及以上
- `xl:` 1280px 及以上
- `2xl:` 1536px 及以上

### 2. 状态样式

```tsx
// 悬停和焦点状态
<button className="bg-blue-500 hover:bg-blue-600 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2">
  按钮
</button>

// 禁用状态
<input disabled className="bg-gray-100 cursor-not-allowed" />
```

### 3. 动画和过渡

```tsx
// 过渡效果
<div className="transition-all duration-300 ease-in-out transform hover:scale-105">
  悬停放大效果
</div>

// 透明度动画
<div className="opacity-0 group-hover:opacity-100 transition-opacity">
  悬停显示
</div>
```

### 4. 网格和布局

```tsx
// Flexbox 布局
<div className="flex justify-between items-center space-x-4">
  <div>左侧</div>
  <div>中间</div>
  <div>右侧</div>
</div>

// Grid 网格
<div className="grid grid-cols-3 gap-4">
  <div className="col-span-2">占两列</div>
  <div>占一列</div>
</div>
```

## 性能优化

### 1. JIT 模式

Tailwind CSS v4 默认使用 JIT（Just-In-Time）编译器，只生成实际使用的样式，大幅减少 CSS 体积。

### 2. 简化的配置

移除了 `tailwind.config.js` 文件，配置直接在 CSS 中完成，减少了配置复杂度。

### 3. 优化的样式结构

使用 `@theme inline` 将主题配置内联到 CSS 中，减少了运行时的样式计算。

## 注意事项

1. **类名顺序**: Tailwind v4 对类名顺序更敏感，建议按照以下顺序：
   - 布局类（flex, grid, position）
   - 间距类（p, m, space）
   - 尺寸类（w, h, min/max）
   - 字体和文本类
   - 颜色和背景类
   - 边框和阴影类
   - 状态和动画类

2. **自定义样式**: 对于特殊需求，可以使用 `@layer` 指令添加自定义样式：

```css
@layer components {
  .btn-primary {
    @apply bg-blue-500 text-white px-4 py-2 rounded-md hover:bg-blue-600;
  }
}
```

3. **动态类名**: 使用完整的类名，避免字符串拼接：

```tsx
// 正确做法
<div className={`${isActive ? 'bg-blue-500' : 'bg-gray-200'} text-white`}>

// 避免动态拼接
<div className={`bg-${color}-500 text-white`}> // 不推荐
```

## 总结

Tailwind CSS v4 带来了更简洁的配置方式和更好的开发体验。通过内联主题配置、简化的导入方式和优化的性能，使得样式开发更加高效。项目充分利用了这些新特性，实现了响应式设计、主题切换和丰富的交互效果。