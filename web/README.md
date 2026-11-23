# Ani-Go Web 前端

[English](README_EN.md) · [安装与运行](../README.md)

管理界面使用 Vue 3、Vue Router、TypeScript 和 Vite。设置页的「插件与自动化」负责清单导入、启停和来源筛选。`PluginPanel.vue` 按 Ani-Go 面板声明渲染凭据、操作和任务列表。[作者指南](../docs/02_PLUGIN_SPEC.md) · [导入示例](../examples/plugins/cloud-tasks.json)

## 本地开发

在仓库的 `web` 目录使用 Node 24。

```sh
npm ci
npm run dev
```

默认从 npm 官方注册表安装。使用组织镜像时，可为 `npm ci` 指定 `--registry`，或通过本地 npm 配置设置 `registry`。

Vite 默认从 `http://localhost:5173` 提供页面，已配置将 `/api` 代理到 `http://localhost:20001`。与运行中的 Go 服务联调时使用该服务的管理员登录。API 客户端在 `src/utils/request.ts` 中附加 JWT 并处理过期会话。

同一站点的浏览器页签同步登录状态。一个页签退出时，其他管理页会关闭当前内容并返回登录页；再次登录后，各页签会回到原先打开的站内页面。管理员账户由后端在首次启动时创建。

从搜索结果追番时，普通单集会先显示原始发布标题与建议的作品名、季数、字幕组和分辨率。核对后确认，可选填系列 RSS。批量追番会合并同一作品同一季的发布，混合字幕组或画质相应留空，提交失败的作品保留在对话框内。蜜柑系列条目可直接挑选字幕组。

剧集列表分别显示实际下载或整理状态和手动标记。单集可用下拉框设置或清除标记，勾选多集可批量操作。

订阅详情在可见时每 30 秒刷新，切回页签时也会刷新。编辑订阅或执行剧集操作期间会暂停后台刷新。设置页在重新取得 Bangumi 授权状态时保留未保存的本地修改。

在下载页，新建任务后可按 Enter 提交表单。弹窗空闲时按 Escape 可关闭并返回原按钮。启用自动刷新后，只有可见页签会轮询下载队列；上一次读取尚未返回时会跳过本次轮询。

## 测试与构建

```sh
npm test
npm run typecheck
npm run build
```

`npm run build` 会先执行类型检查，再输出 `web/dist`。从仓库根目录构建 Go 二进制会嵌入这份资源。界面版本来自运行服务的 `/api/health`。[构建配置](vite.config.ts) · [路由](src/router/index.ts) · [HTTP 接口](../docs/API.md)
