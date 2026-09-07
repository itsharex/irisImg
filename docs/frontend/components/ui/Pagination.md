# `frontend/app/components/ui/Pagination.vue`

可复用的分页控件，Nuxt 自动导入标签为 `<UiPagination />`。供日志中心与内容中心的列表分页复用，替代两页此前各自复制的「上一页 / 计数 / 下一页」按钮组。

## 职责

- 上一页 / 下一页按钮（页码边界禁用，`loading` 时整体禁用）。
- 页码按钮 + 省略号折叠：首末页固定可见可点（即首页 / 末页直达），当前页灰色不可按以作区分。
- 页数不多时全量显示，不做折叠。

## Props / Emits

- props：
  - `page: number`：当前页码（从 1 开始）。
  - `totalPages: number`：总页数（调用方用 `Math.max(1, Math.ceil(total / PAGE_SIZE))` 本地计算后传入）。
  - `loading?: boolean`：请求进行中时禁用全部按钮，防止连点（默认 `false`）。
  - `siblingCount?: number`：当前页两侧各显示的页码数（默认 `3`）。
- emits：`change(page: number)`——请求跳到的目标页码。组件不持有状态；越界 / 与当前页相同的守卫由调用方页面的 `goPage` 负责。

## 页码折叠算法

`items` computed 生成 `(number | '…')[]` 序列：

- **全量分支**：`totalPages <= siblingCount * 2 + 5`（默认阈值 11）时返回 `1..totalPages`。
- **折叠分支**：首尾固定 `1` / `totalPages`，中间为 `[page - siblingCount, page + siblingCount]`（收敛到 `[2, totalPages - 1]`）；区间与首页连续（`start <= 2`）时省去左省略号，与末页连续（`end >= totalPages - 1`）时省去右省略号。
- 例：`totalPages=16、page=7、siblingCount=3` → `1 … 4 5 6 7 8 9 10 … 16`；`page=2` → `1 2 3 4 5 … 16`。
- `page` / `totalPages` 的非法输入会被收敛到安全区间（`totalPages` 至少 1，`page` 夹到 `[1, totalPages]`），避免接口数据抖动时序列越界。

## 样式

- 纯 Tailwind，与旧按钮组同风格：`rounded-lg border border-gray-200 text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40`。
- 当前页：`bg-gray-100 font-medium text-gray-400 cursor-not-allowed`，`disabled` 且标注 `aria-current="page"`。
- 页码按钮为紧凑方块（`h-8 min-w-8`）；上一页 / 下一页沿用 `px-3 py-1.5`。

## 与其它文件的关系

- 被 [`pages/logs/index.vue`](../../pages/logs/index.md)、[`pages/content/index.vue`](../../pages/content/index.md) 使用，两页的 `@change` 均接到页面侧 `goPage` 守卫函数。
