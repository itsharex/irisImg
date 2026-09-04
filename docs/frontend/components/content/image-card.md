# `frontend/app/components/content/ImageCard.vue`

内容中心图片网格中的单张卡片。Nuxt 自动导入标签为 `<ContentImageCard />`。

## 职责

- 展示缩略图（`resolveImageUrl(image.url)` + `object-cover` 正方形）、文件名、大小与上传时间。
- 点击 emit `click(image)`。**点击语义由父页面分叉**（多选模式切换选中、默认模式打开详情弹窗），卡片本身保持哑组件、不感知业务。
- 多选模式下展示右下角勾选徽标与选中态边框。

## Props / Emits

- props：
  - `image: ImageItem`（类型见 [`useImages`](../../composables/useImages.md)）；
  - `selectable?: boolean` — 是否处于多选模式：显示右下角勾选徽标，hover 边框提示「点击是选择」；
  - `selected?: boolean` — 多选模式下该卡片是否被选中：主色（`iris-dark`）描边 + 柔和 ring（`iris-violet/30`）+ 实底勾。
- emits：`click(image: ImageItem)`。

## 选中态样式实现

- 边框三态互斥（computed `borderClass`）：选中 > 多选未选中 > 默认。选中态与 hover 放在互斥分支而非叠加类，避免 hover 把选中边框撤掉；多选未选中用 `hover:border-iris-dark/50` 替代默认的紫色 hover，提示当前点击语义是选择。
- 勾选徽标（`selectable` 时渲染）落在缩略图 `relative` 容器内 `absolute bottom-2 right-2`，**常驻占位两态切换**（未选中 = 半透明空圈 `bg-black/30 backdrop-blur-sm`，选中 = 实底 `bg-iris-dark text-white`），切换不引起布局跳动。对勾 path 为 `M4.5 12.75l6 6 9-13.5`（与全站 Heroicons 风格一致）。
- 根 button 带 `:aria-pressed="selectable ? selected : undefined"` 增强可访问性。

## 与其它文件的关系

- 父组件：[`pages/content/index.vue`](../../pages/content/index.md)（传入 `selectable` / `selected`，并做点击分叉）。
- 依赖 [`useImages`](../../composables/useImages.md) 的 `resolveImageUrl` / `formatBytes` / `formatDate`（自动导入）。
