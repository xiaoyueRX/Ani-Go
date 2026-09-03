# Ani-Go Web 前端

基于 Vue3 + Vite + TypeScript 构建的 Ani-Go 管理面板前端。

## 技术栈

- **框架**: Vue 3.5+ (Composition API + `<script setup>`)
- **构建工具**: Vite 8
- **语言**: TypeScript 6
- **CSS**: TailwindCSS v4 + DaisyUI v5（暗色主题）
- **路由**: Vue Router 4（History 模式 + JWT 路由守卫）
- **HTTP**: Axios（拦截器自动注入 Bearer Token、401 重定向）

## 开发

```bash
cd web

# 安装依赖（首次）
npm install

# 启动开发服务器（热更新）
npm run dev

# 生产构建（输出到 dist/）
npm run build

# 类型检查
vue-tsc -b
```

开发服务器默认运行在 `http://localhost:5173`，API 请求代理到 Go 后端（需在 `vite.config.ts` 中配置 proxy）。

## 项目结构

```
web/
├── src/
│   ├── App.vue              # 根组件
│   ├── main.ts              # 入口（挂载 App + Router）
│   ├── style.css            # 全局样式（Tailwind + DaisyUI）
│   ├── router/index.ts      # 路由定义（登录 / 订阅 / 下载 / 设置）
│   ├── utils/request.ts     # Axios 封装（Token 注入 + 401 拦截）
│   ├── components/          # 可复用组件
│   │   └── SubscriptionEditForm.vue  # 订阅编辑表单
│   └── views/               # 页面组件
│       ├── Login.vue        # 登录页
│       ├── Layout.vue       # 布局壳（侧边栏导航 + 顶栏）
│       ├── Subscriptions.vue      # 订阅列表
│       ├── SubscriptionDetail.vue # 订阅详情 + 剧集
│       ├── SubscriptionForm.vue   # 新建订阅
│       ├── Downloads.vue    # 下载队列
│       └── SettingsPage.vue # 设置管理
├── index.html               # HTML 入口
├── vite.config.ts           # Vite 配置
├── tsconfig.json            # TypeScript 配置
└── package.json             # 依赖与脚本
```

## 生产部署

前端构建产物（`dist/`）通过 Go 后端的 `//go:embed` 嵌入到二进制中，无需单独部署。详见项目根目录的 README.md。

## 注意事项

- DaisyUI v5 在 Node 24 下，CSS 中的 `@plugin "daisyui"` 会报错，已在 `style.css` 中改用 `@import "daisyui/daisyui.css"`
- `index.html` 设置 `data-theme="dark"` 以启用 DaisyUI 暗色模式
- Vue Router 使用 History 模式，Go 后端的静态文件处理器会将所有非 `/api/*` 路径回退到 `index.html`
