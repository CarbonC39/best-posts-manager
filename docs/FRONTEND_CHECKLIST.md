# Frontend 人工检查清单

这份清单用于每次 UI 交付前重复进行人工检查。只有实际操作并检查截图或行为后才能勾选；构建成功或 DOM 数值本身不代表视觉验收。记录截图位置、使用的数据状态和未覆盖项，未执行的项目保持未勾选。

## 检查前准备

- [ ] 阅读 `AGENTS.md`、本次 UI 需求和 `design.jpg`；列出当前页面实际使用的组件、状态和主要比例。
- [ ] 检查并保留工作区已有改动；仅使用隔离数据副本，不编辑真实 `.data`。
- [ ] 记录浏览器、服务地址、数据副本位置、时间和截图目录。
- [ ] 准备真实数据页面状态：无账号/无话题适用时检查空状态；另检查真实账号、空标题、长标题、草稿及媒体状态。不得伪造账号、状态或检查结果。

## 视口与页面结构

- [ ] 在 1440px 桌面宽度检查 `design.jpg`：品牌与主编辑器对齐；编辑器、稿纸、版本栏、工具栏、侧栏尺寸及内部比例符合已确认规格；背景渐变、主框蓝灰、aqua/lavender/peach 控件和纯白稿纸辨识清楚。
- [ ] 在 320px、390px、430px 手机宽度分别检查布局；逐个确认页面无横向溢出、字段互相遮挡、媒体/utility/菜单盖住正文或主操作。
- [ ] 检查编辑器结构顺序：版本选择 → 纯白正文稿纸 → 有内容时显示的媒体 → 工具与唯一发布 CTA；无媒体时没有媒体空卡片或重复上传入口。
- [ ] 检查 desktop 与 phone 的媒体尺寸、比例和布局；分别检查无媒体、1、2、3、4 张真实图片；图片拖动顺序、删除与 ALT 状态只在对应真实能力可用时检查。
- [ ] 检查 utility 在桌面的位置；手机滚动到主内容和媒体末尾，确认保留区让 utility 不遮挡页面内容。

## 字体、颜色和交互状态

- [ ] 检查正文、placeholder、次级信息、状态和按钮文字对比度；任何文字不得小于 10px，正文通常不低于 12–14px，标题相对正文至少 1.5 倍；10px 仅用于极少量紧凑标签/metadata。
- [ ] 检查编辑器工具图标为白色且能从蓝灰背景识别；其他浅色表面上的图标仍清晰；检查按钮文字、disabled、hover、pressed、focus 状态。
- [ ] 用键盘逐项检查 Tab 顺序、可见焦点、菜单/dialog 的 Escape 与关闭、焦点返回，以及 disabled/loading/error 状态下的真实行为。
- [ ] 检查标题很长时截断/换行；打开账号选择、话题菜单和其它 popover，确认定位不被滚动容器裁切；打开互斥菜单/dialog，确认先前菜单关闭。
- [ ] 打开“选择发布账号”，检查标题、搜索、真实账号 handle/平台名、logo default/hover/selected、已添加状态及“添加/切换”动作；确认列表滚动且 footer 操作始终可达。
- [ ] 在桌面及 320px/390px 手机检查三角形侧栏：桌面侧栏常驻/折叠耳朵与dock同色相接、折叠后原位可展开；手机触发区是透明40px，打开紧凑锚定菜单后 composer 不移动，搜索/列表/New可达，外部点击/Escape/选话题后关闭并恢复焦点。
- [ ] 检查 header 话题、sidebar 话题行两处 Ellipsis：默认无描边/背景/阴影，hitbox对齐，focus可见，popover贴边且不裁切。编辑器 footer 不应有 Ellipsis。
- [ ] 在主稿与真实 X 版本分别打开版本 picker：稿件列表可独立滚动，当前版本操作区在视口内；检查预览、站外关联、复制和移除仅按真实状态出现，并互斥关闭其它菜单。

## 手机侧栏、媒体和浮窗

- [ ] 手机打开/关闭紧凑侧栏菜单；确认其锚定在 header 裸三角触发点附近、不推动主编辑器；检查透明点击外部关闭、三角关闭、选择话题后的关闭、New 操作、内部列表滚动和焦点返回。
- [ ] 检查真实 ALT badge：空值显示 `+ ALT`，有已保存值显示 `✔ ALT`；分别打开编辑、取消、保存、清空与错误状态。确认未保存输入不会伪装成已保存状态。
- [ ] 检查 Preview 和 Translate 固定标题/关闭控件；内容区可滚动，320px/390px 下所有现有字段与实际动作可达。仅检查真实翻译配置/结果；不为验收触发外部翻译。
- [ ] 检查真实预览账号标识、正文和媒体；不添加 schema 不支持的头像、互动数或占位媒体。

## 发布设置与发布前检查

- [ ] 打开版本 picker 检查“可见性”在当前版本操作区；切换主稿、版本、账号后只显示当前平台 capability 声明的选项。public-only 账号也显示其唯一公开选项；主稿不显示平台可见性。
- [ ] 核对 visibility 选项中文名与真实语义：Mastodon `unlisted` 是公开但不列入公共时间线，Misskey `home` 是公开的主页时间线；不把它们标成私密，不添加 backend 未声明的收件人选项。
- [ ] 确认可见性只有 picker 一处；发布设置和发布确认区域不再重复可见性。修改后沿现有自动保存路径持久化，并用现有 API 读回核实。
- [ ] 对发布前检查的每个真实状态分别观察：图标固定、正文可读、状态标签完整横向显示，不竖排、不重叠、不被裁切；桌面、390px 和 320px 都检查。
- [ ] 检查条目状态色区分通过、建议与错误；状态背景只包裹状态文字，正文行没有胶囊背景。
- [ ] 验证每个检查结果来自真实业务状态；检查摘要计数与异常项一致。不得为了截图修改数据库状态或伪造账号连接、ALT、发布结果。
- [ ] 确认每个页面流程只有一个发布主 CTA；检查仍走现有校验/发布确认路径。没有实际发布授权时，不点击最终确认发布。
- [ ] 确认页面没有重复上传入口、重复发布动作、无用说明文案、占位数据或其它 phantom UI。

## 结束记录模板

复制下方记录并填写。没有执行的项目不得标记通过。

```text
日期/时间：
代码版本或工作区状态：
数据来源（隔离副本路径；不得记录凭证）：
浏览器/视口：
截图目录：
实际检查的状态：
已发现问题及修复：
已执行并通过的人工检查：
未覆盖项（明确写出）：
构建命令及结果（如本次要求构建）：
```

## 本轮最小验收记录

- 日期：2026-10-09。
- 数据来源：隔离副本 `/tmp/bpm-apple-review-v7/data`，真实 `hello` 话题及其 X 版本；未触碰真实 `.data`，未触发发布、翻译、连接或配对操作。
- 修复前证据：[/tmp/bpm-apple-review-v7/preflight-show/desktop-preflight.png](</tmp/bpm-apple-review-v7/preflight-show/desktop-preflight.png>)、[/tmp/bpm-apple-review-v7/preflight-show/phone390-preflight.png](</tmp/bpm-apple-review-v7/preflight-show/phone390-preflight.png>)。原结果列发生竖排、背景污染并导致末尾内容被截。
- 修复后截图：桌面 [desktop-preflight.png](</tmp/bpm-apple-review-v7/preflight-fixed/desktop-preflight.png>)；320px [phone320-preflight.png](</tmp/bpm-apple-review-v7/preflight-fixed/phone320-preflight.png>)；390px [phone390-preflight.png](</tmp/bpm-apple-review-v7/preflight-fixed/phone390-preflight.png>)；430px [phone430-preflight.png](</tmp/bpm-apple-review-v7/preflight-fixed/phone430-preflight.png>)。四张图均显示标题、五项真实检查和完整状态；桌面宽屏、320px 状态标签位于正文下方第二行且文字保持横向、390/430px 右侧状态标签。
- 真实状态：摘要“3 项需处理”；正文与格式/43 字符/通过；图片替代文本/有媒体尚未填写替代文本/建议补充；发布账号/X · @_carbonc_/可用；发布通道/未检测到 X 页面/需连接；主稿更新/当前版本基于较早的主稿/请核对。
- 已覆盖：根 agent 已查看并确认桌面、320/390/430px 四张截图的发布检查局部验收通过；五项图标/正文/状态完整，状态标签横向可读、状态色区分、条目正文无背景胶囊；320px 无横向溢出，页面视口内检查项与手机 utility 未相互遮挡；局部 frontend build 与 Go embed build。
- 构建：`npm --prefix frontend run build && CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o build/bpm ./cmd/bpm` 成功；binary 时间 2026-10-09 15:19:19 PDT。
- 未覆盖：键盘焦点/交互、loading/error 分支和其它实际数据状态；此记录不代表侧栏、ALT、媒体拖动、preview、translate 或其他账号/话题状态的当轮回归已通过。没有检查的通用条目仍保持未勾选。

## 菜单与侧栏修订记录

> 以下前一轮“三个 ellipsis”记录已被 2026-10-09 这轮重做取代；不要将 footer More、旧三角按钮描边或旧菜单层级视为当前通过标准。

- 本轮检查：编辑器添加目标菜单、桌面/手机侧栏，以及三个既有 ellipsis 菜单。
- 数据来源：隔离副本 `/tmp/bpm-apple-review-v7/menu-data`；未接触真实 `.data`，未发布、翻译、连接或配对。
- 截图目录：[/tmp/bpm-apple-review-v7/menu-refinement](</tmp/bpm-apple-review-v7/menu-refinement>)。桌面添加目标菜单 [default](</tmp/bpm-apple-review-v7/menu-refinement/desktop-add-default.png>)、[真实 hover](</tmp/bpm-apple-review-v7/menu-refinement/desktop-add-hover.png>)、[selected](</tmp/bpm-apple-review-v7/menu-refinement/desktop-add-selected.png>)；桌面侧栏 [展开](</tmp/bpm-apple-review-v7/menu-refinement/desktop-sidebar-open.png>)、[折叠](</tmp/bpm-apple-review-v7/menu-refinement/desktop-sidebar-collapsed.png>)；三个菜单 [header](</tmp/bpm-apple-review-v7/menu-refinement/desktop-header-more.png>)、[sidebar](</tmp/bpm-apple-review-v7/menu-refinement/desktop-sidebar-more.png>)、[footer](</tmp/bpm-apple-review-v7/menu-refinement/desktop-footer-more.png>)；手机 [320px 侧栏](</tmp/bpm-apple-review-v7/menu-refinement/phone320-small-menu.png>)、[390px 侧栏](</tmp/bpm-apple-review-v7/menu-refinement/phone390-small-menu.png>)、[390px footer More](</tmp/bpm-apple-review-v7/menu-refinement/phone390-footer-more.png>)。
- 已实际检查：X、Mastodon、Bluesky 等真实平台标识在默认及鼠标悬停截图中持续可见，hover 使用浅中性色；菜单行无黑色底块。添加目标菜单中的 selected 状态已截图，但没有提交账号选择。桌面侧栏紧凑展开/折叠、手机 320/390px 锚定菜单均有截图；打开侧栏前后 editor y 坐标不变，320px 页面宽度未溢出。三个 ellipsis 连续执行 header→sidebar→footer 时每次仅当前菜单打开；Escape 关闭当前菜单。点击添加目标 `+` 会关闭 header 菜单并打开目标选择器；手机打开侧栏后点击 header More 会关闭侧栏，之后打开 footer More 仍只保留 footer 菜单；Escape 可关闭它。三个桌面菜单截图中 visibility 仍位于编辑器下方“发布设置”，不属于菜单项。
- 构建：前端及 Go embed build 成功；binary 时间 2026-10-09 16:52:36 PDT。
- 最后局部修正：复用 Lucide `ArrowRight` 显示“切换到版本”，新目标仍用 `Plus`；桌面侧栏展开时 header More 仅显示“归档话题”，折叠时仍提供“新建话题”。截图：[目标 selected 状态](</tmp/bpm-apple-review-v7/menu-refinement/desktop-add-selected.png>)、[展开侧栏 header More](</tmp/bpm-apple-review-v7/menu-refinement/desktop-header-more.png>)、[折叠侧栏 header More](</tmp/bpm-apple-review-v7/menu-refinement/desktop-collapsed-header-more.png>)。前端及 Go embed build 成功；binary 时间 2026-10-09 16:55:52 PDT。
- 根 agent 已查看添加目标菜单 default/hover/selected、桌面与 320px 侧栏、三类桌面 ellipsis 及 390px footer More 截图，确认原菜单/侧栏问题通过；展开/折叠下 header 创建入口按单一动作规则复核通过。
- 未覆盖：账号选择最终提交、真实 focus 样式截图、完整键盘 Tab 遍历/焦点返回、外部点击关闭和窗口 resize 后的菜单重新定位；长标题截断与多话题列表滚动也未实测。没有检查的通用条目仍保持未勾选。

## 版本操作、Ellipsis 与折叠耳朵重做记录

> 本节初始记录是历史证据；下方“最终修正记录”覆盖当前标准和结果。

- 发现：footer More 与设计草图不符，且版本动作脱离其版本选择上下文；Ellipsis summary 带默认边框/悬浮底；侧栏折叠控件是与dock分离的方形按钮；添加发布目标文案带内部术语且选择状态不直观。
- 本轮目标：footer 移除 Ellipsis；将现有预览/站外关联/复制/移除 handler 按原条件放入版本 picker 下部；picker 上部稿件列表独立滚动、操作区固定可见。添加账号改为“选择发布账号”和“搜索账号”，用真实状态显示“已添加”或不显示状态文案；主动作简化为“添加/切换”。header/话题行 Ellipsis 默认透明无边框、无背景/阴影；侧栏折叠tab与dock同色连接、30×24，并在手机用无可见样式的12px三角。
- 修复后截图目录：`/tmp/bpm-apple-review-v7/menu-correction`（尚未验收；截图路径待填写）。需要包含 desktop target default/hover/selected、dock open/closed全景与近景、topic/sidebar More、master/X picker、phone320/390侧栏/target/header/version状态。
- 本轮尚未执行视觉/交互验收；下方记录补充当前实际截图、视口和状态，旧截图不沿用为通过。

## 可见性入口与版本菜单最终修正

- 后端 capability 矩阵（依据 `internal/platform/capability.go`、store 校验与实际 adapters）：

  | 项目账号平台 | 后端可见性值 | 仅本地 | 前端显示 |
  | --- | --- | --- | --- |
  | Mastodon / mastodon-compatible | `public`, `unlisted`, `private`, `direct` | 不支持 | 公开；不列入公共时间线（公开）；关注者及提及用户；仅提及用户 |
  | Misskey | `public`, `home`, `followers` | 支持 | 公开；主页时间线（公开）；仅关注者；仅本地复选框 |
  | Bluesky | `public` | 不支持 | 公开 |
  | X、微博、小红书、QQ 空间 bridge/manual | `public` | 不支持 | 公开；项目没有这些平台的原生发布 adapter，能力不代表项目有平台发布 API |
  | 未知平台 | 无 | 不支持 | 不渲染可见性控件 |

- 当前隔离副本实际账号平台：X、Mastodon、Misskey、Bluesky、微博、QQ 空间；没有小红书账号或 mastodon-compatible 别名账号。账号 capability 按 account ID 单独读取；`variantCapability` 以 `variant.account_id` 查询同一 ID，因此别名不会造成 capability key 不匹配。Mastodon `direct` 沿用文本提及用户的接收者语义；Misskey 没有 `specified` 值或额外 recipient 字段，未增加此类控件。
- 修复：可见性与仅本地从编辑器下方/发布确认重复字段移至版本 picker 当前版本操作区；option value 保持 backend 原值，中文 label 按平台真实含义显示。仅本地只在 `supports_local_only` 为真时渲染，与可见性字段绑定已有 `activeVariant` 自动保存；锁定版本禁用。仅 capability 返回非空列表时显示可见性，public-only 平台显示唯一“公开”；主稿无平台设置。确认区保留 capability 支持的内容警告。
- 截图目录：[/tmp/bpm-apple-review-v7/menu-correction](</tmp/bpm-apple-review-v7/menu-correction>)。该目录保留前几轮有效布局图；账号选择菜单 default/hover/selected 与 phone320 以最终修订目录 `menu-correction-final/` 为准，旧 `desktop-plus-*` 图已移到 `menu-correction/before/stale/`，不可作为当前菜单验收图。侧栏高清连接近景见 [desktop-sidebar-attached-flap-hires.png](</tmp/bpm-apple-review-v7/menu-correction/desktop-sidebar-attached-flap-hires.png>)；Misskey可见性与禁用态“仅本地”见 [phone390-version-menu-misskey-local-only.png](</tmp/bpm-apple-review-v7/menu-correction/phone390-version-menu-misskey-local-only.png>)；Mastodon短标签在320/390px完整显示见 [phone320](</tmp/bpm-apple-review-v7/menu-correction/phone320-version-menu-mastodon-label-check.png>) 与 [phone390](</tmp/bpm-apple-review-v7/menu-correction/phone390-version-menu-mastodon-label-check.png>)。
- 隔离副本真实持久化：用 Mastodon `@carbonc@defcon.social` 的版本菜单 select 从 `unlisted` 改为 `private`，等现有 autosave 完成后 `GET /api/variants?topic_id=…` 读回 `private`；再用同一 select 恢复 `unlisted` 并二次 GET 读回 `unlisted`。未直接 PATCH、未触发发布或外部平台操作。Misskey 与 X 的 hello 版本已 publication-locked，所以只检查真实 options/disabled 状态，不绕过锁定写入；Misskey `home` 原值及 X `public` 单项均保留。
- 实际几何/行为自查：picker桌面宽334、最大高420，手机版390宽318且右边距11px；actions、visibility select在面板内，稿件列表独立滚动。手机话题菜单为220×166，打开前后composer y=114一致，document scrollWidth等于390视口宽。
- 侧栏/工具自查：桌面展开dock为180px宽；收起耳朵最终tab为30×24，top与dock底部仅1px overlap用于盖住底描边，不与New话题40px点击区相交；桌面工具组computed为4列95px、按钮中心均匀，publish CTA仍104px并靠右。手机320/390侧栏打开、版本菜单截图无横向溢出。
- 菜单清单：全 `frontend/src` 检索后，仅 App.vue 存在两个真实 Ellipsis：topic header menu 与 sidebar row menu；footer More已移除，picker/menu/管理设置没有其它 Ellipsis trigger。旧 `preview-alignment.css` 未被生产入口引用，不作为有效样式来源。header展开状态只有归档；折叠状态有新建与归档。截图见 `desktop-header-more.png` 与 `desktop-collapsed-header-more.png`。
- 本轮实际结果：Mastodon value绑定/自动保存/读回/恢复通过；Misskey三项与X单项截图有真实账号状态。Misskey 的 `仅本地` 复选框按 capability 显示；本隔离副本的 hello Misskey 版本处于 publication-locked，截图核对为禁用态，没有绕过锁定写入。目标账号真实平台logo在默认/hover/selected截图持续显示，hover浅中性；手机小菜单打开不推动composer。其余清单项不因此视为通过。
- 最新截图：[/tmp/bpm-apple-review-v7/menu-correction/phone390-version-menu-misskey-local-only.png](</tmp/bpm-apple-review-v7/menu-correction/phone390-version-menu-misskey-local-only.png>)，显示真实 Misskey visibility 与仅本地字段及锁定状态；Mastodon `unlisted` 中文选项缩短为“不列入公共时间线”，[320px](</tmp/bpm-apple-review-v7/menu-correction/phone320-version-menu-mastodon-label-check.png>) 与 [390px](</tmp/bpm-apple-review-v7/menu-correction/phone390-version-menu-mastodon-label-check.png>) screenshot 都完整显示。桌面dock左下角修正为方形底角接耳朵，真实高清截图见上方 `desktop-sidebar-attached-flap-hires.png`。
- 构建：`npm --prefix frontend run build && CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o build/bpm ./cmd/bpm` 成功；binary 时间 2026-10-09 19:08:37 PDT。
- 未覆盖：Misskey `local_only` 变更的持久化写入（当前版本锁定，未绕过）、长标题、多话题滚动、完整键盘遍历/focus回返、触屏三角点击和窗口resize后popover重新定位；Misskey/X现有版本锁定，未执行visibility写入。其它未检查的通用条目不视为通过。

## 账号选择弹层重叠修复与平台可见性截图

- 根因：旧账号选择器用 `grid-template-rows: auto 40px minmax(0, 1fr) 40px`，长 `ul` 在其 grid item 布局中实际从标题行下方开始并超出列表轨道，覆盖标题/搜索/footer。修正为固定最大高度的 flex column：标题自然高、搜索固定40px、`ul` 独立滚动且 `min-height:0`、底部动作固定40px；行仍保持正常48px文档流。手机320px三角按钮的 hover 背景也改为透明，保留40px hit area和 focus-visible outline。
- 真实几何检查：桌面弹层334×360；标题 y231–257、搜索 y263–303、滚动列表 y309–519、footer y525–565；列表 scrollHeight339/clientHeight210。320px弹层296×360；标题 y200–226、搜索 y232–272、列表 y278–488、footer y494–534；document宽度320。QQ空间/微博继续在列表内滚动，未覆盖footer。桌面与phone320截图由实际Chromium截图并视觉检查。
- 当前截图目录：[/tmp/bpm-apple-review-v7/menu-correction-final](</tmp/bpm-apple-review-v7/menu-correction-final>)。账号菜单 [default](</tmp/bpm-apple-review-v7/menu-correction-final/desktop-add-default.png>)、[hover](</tmp/bpm-apple-review-v7/menu-correction-final/desktop-add-hover.png>)、[selected](</tmp/bpm-apple-review-v7/menu-correction-final/desktop-add-selected.png>)、[phone320](</tmp/bpm-apple-review-v7/menu-correction-final/phone320-add-default.png>)；[phone320裸三角与展开小菜单](</tmp/bpm-apple-review-v7/menu-correction-final/phone320-side-open.png>)。desktop-add的真实平台logo在默认/hover/selected持续可见。
- 可见性截图补充：现有 hello Bluesky draft [public-only](</tmp/bpm-apple-review-v7/menu-correction-final/desktop-version-menu-bluesky-public-only.png>)显示唯一“公开”且可编辑；现有 Weibo locked variant [public-only](</tmp/bpm-apple-review-v7/menu-correction-final/desktop-version-menu-weibo-public-only.png>)显示同一唯一选项且禁用。Misskey值与仅本地见上方手机390菜单截图。QQ空间账号存在，但此话题没有 QQ 空间版本，本轮未造版本，故未覆盖其 picker visibility 状态。
- Mastodon现有 `@carbonc@defcon.social` hello版本实际走 picker select：初值 `unlisted` → UI选择 `private` → 等 autosave 后 GET variants 读到 `private` → 用同一 UI select 恢复 `unlisted` → GET读到 `unlisted`。该版本可编辑、未发布；没有直接 PATCH、发帖或修改已发布锁定版本。Misskey/X/Weibo locked版本未绕锁写入。
- 视觉验收：Root 已看并确认桌面账号菜单default/hover/selected、phone320账号菜单、phone320裸三角与侧栏菜单，以及 Bluesky/Weibo visibility 截图通过。未检查项仍保持未覆盖。
- 构建：前端生产 build 与 Go embed build 成功；binary时间 2026-10-09 19:14:03 PDT。没有运行测试套件。
