# 章八爪 v2.1 任务书：遗留身份深度清理 + 质量闸门（高质量执行 Prompt）

作者：长鱼茶汤茗 ｜ 日期：2026-10-20 ｜ 仓库：pluto-da/zhangbazhua（custom → build）
执行方法：符号—结构驱动（symbol-structure-core）。本任务书即验收依据，逐条执行、逐条勾验，不凭印象简化。

---

## 0. 角色与总目标

你是一名资深桌面软件发布工程师，同时熟悉 Windows 安装器（NSIS）、Wails/WebView2、Go 与 React。

总目标有两个：
1. 让大哥在电脑上**任何能看到的地方**（安装目录、程序文件、开始菜单、桌面快捷方式、“应用和功能”列表、任务管理器、安装日志、数据目录）都只看到「章八爪 / ZhangBaZhua」，不再出现「BeefTV」；
2. 修好「实时最低价」页面的崩溃，并补上一道质量闸门，让同类错误以后在打包阶段就被拦下，不再流到大哥手里。

前提有三条：
- 不丢数据：旧项目、旧设置、关注列表、渠道配置全部保留；
- 老用户覆盖安装能自动迁移，不留残骸；
- 跟随作者更新时不产生合并冲突。

---

## 1. 需求深度解读（符号层）

大哥发了三张照片，说了三句话：
- “这里怎么还有遗留？”
- “之前的活在偷懒？”
- “进行深度修复。”

### 1.1 照片一、二：安装目录叫 BeefTV，主程序叫 BeefTV.exe
- **事实**：安装路径是 `%LOCALAPPDATA%\Programs\BeefTV\BeefTV.exe`。
- **成因**：v1.7 换品牌时，我只改了显示名、快捷方式名、卸载列表名、窗口标题和图标，故意没改 exe 名、安装目录、注册表键和数据目录。当时的理由是保住旧数据、减少与上游合并的冲突。
- **问题**：这个取舍从来没有向大哥讲清楚，验收也只看了“显示名”，没有从**使用者视角**逐处巡查。
- **符号学判断**：对大哥来说，文件夹名和 exe 名就是“这个软件叫什么”的直接符号；只要有一个地方叫 BeefTV，就等于品牌没有换完。他说“偷懒”，指的不是某一行代码，而是**交付标准停在了开发者视角**。

### 1.2 照片三：实时最低价页面报错 `officialMin is not defined`
- **事实**：v2.0 修改 live-prices.tsx 时，有一处字符串替换的匹配目标写成了 `React.ReactNode`，而源码里是 `ReactNode`，于是替换没有生效。卡片里用到的变量 officialMin 因此从未定义，页面一渲染就崩。
- **为什么没被发现**：
  - 我的替换脚本没有断言“每处都命中”；
  - 沙箱内存不够，没跑类型检查；
  - 云端打包用的是 `build:desktop`（只有 vite build，不跑 tsc），而作者自己的 `build` 是 `tsc --noEmit && vite build`。
- **结构判断**：这次的问题不在某一行写错，而在**流程里缺了一道闸门**。只修这一行，下次还会再出。

### 1.3 “深度修复”的完整含义（专业补充）
1. **身份统一要覆盖全部可见触点**：
   - 安装目录、主程序名、开始菜单文件夹、桌面快捷方式；
   - “应用和功能”里的注册表键与图标；
   - 任务管理器里的进程名；
   - 安装日志目录、数据目录、WebView2 缓存目录、更新缓存目录。
2. **改名必须带迁移**，否则会出现三类问题：
   - 老安装残留成两份；
   - 设置丢失：WebView2 的 localStorage 默认放在 `%APPDATA%\<exe名>`，exe 一改名，关注列表、外观设置、模块开关都会丢；
   - 项目找不到：数据目录改名但不迁移，就会打开一个空工作区。
3. **保留的东西要说明理由**，并且只保留**用户在正常使用中看不到的内部标识**：
   - `cli\beeftv.exe`：Agent 提示词和工具都用 `beeftv` 命令调用它，改名会让 AI 助手失灵；
   - 插件文件扩展名 `.beeftv-plugin`：插件格式标识，上游加载器按扩展名识别；
   - 互斥锁名、源码里的包名、测试。
   这些都属于 C 类标识符，保留并写进文档。
4. **防冲突结构**：
   - 身份改动尽量放在 CI 构建时的补丁脚本里，不提交到上游文件；
   - 补丁对每一处替换都断言“必须命中”，上游一改就让构建失败报警，而不是悄悄漏改。
5. **质量闸门**：
   - CI 打包前先跑 `tsc --noEmit`（类型检查）；
   - 打包后巡检安装载荷的顶层文件名，出现 BeefTV 就失败；
   - 补丁脚本本身带 `--check` 审计模式。

---

## 2. 结构模型

### 2.1 核心二元对立
**开发者视角的“改完了” vs 使用者视角的“看不到旧名”**。验收以后者为准。

### 2.2 身份触点表（逐项处理）

| # | 触点 | 现状 | 目标 | 迁移 / 兼容 |
|---|---|---|---|---|
| 1 | 安装目录 | `Programs\BeefTV` | `Programs\ZhangBaZhua` | 安装器从旧注册表键读到旧目录；安装成功后清掉旧载荷并删除空目录 |
| 2 | 主程序 | `BeefTV.exe` | `ZhangBaZhua.exe` | 安装器关闭新旧两个进程；清理时删除旧 exe |
| 3 | 开始菜单 | `开始菜单\BeefTV\` | `开始菜单\章八爪\` | 删除旧文件夹及其中快捷方式 |
| 4 | 桌面快捷方式 | 章八爪.lnk → BeefTV.exe | 章八爪.lnk → ZhangBaZhua.exe | 重建 |
| 5 | 卸载注册表键 | `Uninstall\BeefTV` | `Uninstall\ZhangBaZhua` | 安装成功后删除旧键 |
| 6 | 安装日志 | `%LOCALAPPDATA%\BeefTV\Installer` | `%LOCALAPPDATA%\ZhangBaZhua\Installer` | 不迁移旧日志 |
| 7 | 数据目录 | `%APPDATA%\BeefTV` | `%APPDATA%\ZhangBaZhua` | 首次启动时，若新目录不存在而旧目录存在，用原子 rename 整体搬迁；搬迁失败就继续用旧目录，绝不新建空工作区 |
| 8 | WebView2 数据 | `%APPDATA%\BeefTV.exe`（Wails 默认） | `%APPDATA%\ZhangBaZhua\WebView2` | 首次启动整体搬迁；失败则继续用旧路径 |
| 9 | 更新缓存 | `%LOCALAPPDATA%\BeefTV\updates` | `ZhangBaZhua\updates` | 不迁移 |
| 10 | 版权字段 | LegalCopyright `BeefTV` | `长鱼茶汤茗（基于 BeefTV，MIT）` | MIT 要求保留原作者署名 |
| 11 | 安装器提示文案 | 多处 BeefTV | 章八爪 | — |
| 12 | 选择存储目录对话框 | 标题含 BeefTV，选盘根时自动加 BeefTV 子目录 | 改为章八爪 / ZhangBaZhua | — |

### 2.3 数据迁移是否安全（证据）
- 上游自带的「迁移存储目录」功能（desktopstorage.CopyClosedWorkspace）是原样复制整个工作区目录，**不改写任何路径**。这说明工作区内部用的是相对路径，整体搬迁是上游自己支持的操作。
- 用同一磁盘内的 rename，要么整体成功，要么完全不动，不会出现搬一半的情况。
- 程序启动时数据库还没打开，这时搬迁不存在文件占用。

### 2.4 补丁位置（防冲突）
- 新增**零依赖**小包 `backend/internal/zbzid`：
  - `DataRoot(configDir) string`：带迁移；
  - `WebviewDir() string`：带迁移；
  - `ExeName = "ZhangBaZhua.exe"`。
- 上游文件不提交改动，由 CI 的 `scripts/zbz/identity.py` 在构建时打补丁，逐条断言命中。涉及的文件：
  - desktopstorage/location.go 的 DefaultRoot；
  - cmd/desktop/main.go 的 WebviewUserDataPath（main.go 已是我们的挂载点）；
  - storage.go 的对话框文案；
  - desktopupdate 的 exe 名与缓存目录；
  - wails.json 的 name 与 outputfilename；
  - build-beeftv-windows-release.ps1 的 exe 路径；
  - package-desktop-installer.py 的 Windows exe 名；
  - windows.nsi 的全部身份触点。
- `identity.py --check`：只审计不改。补丁执行后再审计一遍，确认可见触点里不再有 BeefTV。

---

## 3. 实施步骤

1. **修复 live-prices.tsx**：在 ModelCard 里正确定义 officialMin（同单位官方/云厂商最低价），并全文检查 v2.0 那批替换是否每一处都命中，逐处核对引用的变量都有定义。
2. **新增 zbzid 包与单元测试**，覆盖四种情况：
   - 旧目录存在、新目录不存在 → 搬迁；
   - 新目录已存在 → 不动；
   - 搬迁失败 → 返回旧目录；
   - 两者都不存在 → 返回新目录。
3. **编写 identity.py**：替换项表格化；每项写明文件、原文、新文，以及是否允许重复命中；任何一项没命中就让 CI 失败，并输出中文原因。
4. **NSIS 重点改动**（由补丁完成）：
   - 默认目录、子目录名、foreign 检查同时认新旧两个 exe；
   - 关闭进程时同时处理 BeefTV.exe 和 ZhangBaZhua.exe；
   - RemovePayload 删除新旧两个 exe；
   - 读取旧注册表键作为 PreviousInstallDir 的备选；
   - 安装成功后删除旧键和旧开始菜单文件夹；
   - 启动项、快捷方式、DisplayIcon 都指向新 exe；
   - 卸载时清理新旧快捷方式和注册表键。
5. **CI 改动**（windows-build.yml）：
   - rebrand 之后运行 identity.py，以及 identity.py --check；
   - 构建前运行 `bun run typecheck`；
   - 打包后巡检载荷顶层，出现 BeefTV 即失败，白名单为 cli、plugin-packages 目录内部；
   - artifact 名改成 ZhangBaZhua-windows。
6. **跟进任务**：每日合并上游时，identity 补丁一旦失败就停下并如实报告，不强推。
7. **文档**：项目文档写明保留清单（C 类）及理由。

## 4. 验收

一级（不过不交付）：
1. CI 中 typecheck 通过，identity 补丁全部命中，审计结果为 0 处遗留，载荷顶层无 BeefTV。
2. 载荷里存在 ZhangBaZhua.exe，不存在 BeefTV.exe。
3. NSIS 脚本里，默认目录、快捷方式、注册表键、启动项全部指向新身份，并保留对旧身份的清理逻辑。
4. zbzid 单元测试通过；live 页面相关的变量引用全部有定义，并通过 tsc。
5. 上游文件在 custom 分支的提交改动只涉及已有挂载点；Release 发布成功。

二级（如实记录）：
- 真实 Windows 机器上的覆盖安装和数据迁移，需要大哥装好后确认：打开后项目、渠道、关注列表都在；
- 任务栏上固定的旧快捷方式会失效，需要重新固定一次；
- cli\beeftv.exe 与 .beeftv-plugin 按 C 类保留。

## 5. 风险与对策

| 风险 | 对策 |
|---|---|
| 数据搬迁失败 | 回退使用旧目录，绝不新建空工作区 |
| 旧进程未退出导致搬迁失败 | 安装器先关闭新旧两个进程；程序启动时搬迁失败就用旧目录 |
| 上游改了被打补丁的代码 | 断言失败，CI 报红，跟进任务停下报告 |
| 杀毒软件拦截改名后的 exe | 与之前的 BeefTV.exe 同为未签名程序，风险等同；如果出现，按提示放行 |
| WebView2 旧目录被占用 | 回退使用旧路径，设置不丢 |

## 6. 交付前自查
- 我有没有从大哥的视角，把安装目录、开始菜单、任务管理器、应用列表、AppData 逐处走一遍？
- 每一处替换都断言命中了吗？
- 保留项都有理由，并写进文档了吗？
- 旧数据的迁移失败时有回退吗？
- 这次的教训（替换要断言、CI 必须跑类型检查、验收用使用者视角）写进记忆了吗？

## 7. 教训（写入记忆）
- 换品牌、改名、清理遗留，验收必须以“使用者能看到的每一处”为准。故意保留的项要当场向大哥讲清楚理由。
- 用脚本批量替换代码时，每处都要断言命中；交付前必须过类型检查，沙箱跑不了，就让 CI 跑。
