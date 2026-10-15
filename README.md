<p align="center"><img src="resources/icon.png" width="128" alt=""></p>

<h1 align="center">MonitorBar</h1>

<p align="center">macOS 菜单栏 / Windows 托盘里的显示器管理工具，一份 Go 代码，两端原生界面。<br>
A menu bar (macOS) and tray (Windows) app to manage your monitor — <a href="#english">English</a></p>

<p align="center">
<img src="docs/screenshots/overview.png" width="270" alt="概览">
<img src="docs/screenshots/modes.png" width="270" alt="分辨率">
<img src="docs/screenshots/activity.png" width="270" alt="屏幕时间">
</p>

## 为什么做这个

REDMI 显示器不支持 DDC/CI。

Mac 上常见的显示器工具（MonitorControl、BetterDisplay、Lunar 等）都靠 DDC/CI 控制显示器，所以在 REDMI 显示器上切换不了输入源，也调不了音量。

不过 REDMI 显示器内置澎湃 OS，开机时会在局域网提供一个遥控接口。MonitorBar 通过这个接口切换输入源（包括 DP 和 USB-C）、调节音量，不依赖 DDC/CI。软件调光、色温、分辨率、屏幕时间这些通用功能也一起做了进去，并且有 Mac 和 Windows 两个版本：两台电脑接在同一台显示器上时，在任意一台上都能一键切到另一台。

MonitorBar 只在 REDMI 显示器 G Pro 27U 2026 上测试过，不保证在你的显示器上能正常工作：不同型号的接口、信号源编号可能不一样。如果不能用，可以借助 AI 编程工具来适配：先运行 `MonitorBar --probe` 看看它识别到了什么，再让 AI 参照下面的[适配其他显示器](#适配其他显示器)一节修改代码。

## 功能

| 分页 | 功能 |
| --- | --- |
| 概览 | 当前分辨率与刷新率、**切换输入源**、软件亮度、**显示器音量**、场景（日间 / 阅读 / 观影 / 夜间） |
| 画面 | 场景、**跟随 MacBook 内屏亮度**、**按时间自动切换白天 / 夜晚场景**、软件调光、色温（减少蓝光）、一键恢复 |
| 分辨率 | 刷新率切换，HiDPI（仅 macOS）与标准分辨率列表 |
| 屏幕时间 | 今日亮屏与使用时长，最近 7 天柱状图 |

- **切换输入源、调音量不需要 DDC/CI**：澎湃 OS 显示器（如 REDMI 显示器 G Pro 27U 2026）在局域网提供遥控接口，MonitorBar 自动发现并使用它。Mac 和 Windows 接在同一台显示器的不同接口上时，两边都能一键切到对方。
- **支持 DDC/CI 的显示器**：自动用 DDC/CI 调硬件背光，切换输入源、调音量，并显示对比度、锐度、RGB 增益等显示器回应的设置。
- **软件调光与色温**通过显卡的 Gamma 表实现，任何连接线、任何显示器都能用。
- **跟随内屏亮度**（MacBook + 支持 DDC/CI 的外接显示器）：内屏开启 macOS 自动亮度时，外接显示器的背光随之平滑调节，可设定外接亮度范围；手动调整会保留到内屏亮度下次变化。
- **自动切换**：在「画面」里打开后，到了设定的白天、夜晚开始时间就自动切换到对应场景（默认 07:00「日间」、19:00「夜间」，场景可选）。只在时段交替时切换，中途的手动调整会保留到下一次交替。
- **自动更新**：定期检查 GitHub 上的新版本，在面板底部提示，一键下载并重启；更新包经签名校验，只下载变化的部分。
- 显示器不支持的功能不会出现在界面上。
- 单个可执行文件，无 webview，空闲时不占 CPU；中文界面，深色主题。

## 安装

从 [Releases](https://github.com/binaryify/MonitorBar/releases) 或 [CI 产物](https://github.com/binaryify/MonitorBar/actions) 下载：

- **macOS**（Apple Silicon / Intel，macOS 12+）：打开 `.dmg`，把 MonitorBar 拖进「应用程序」。应用未签名，首次打开需右键 → 打开。第一次连接显示器时允许「查找本地网络上的设备」。
- **Windows 10 / 11**：把 `MonitorBar.exe` 和同目录的 `icon.png` 放到任意文件夹（推荐 `%LOCALAPPDATA%\Programs\MonitorBar`）后运行。托盘图标默认在 `^` 溢出区，可拖到任务栏常驻。

开机自启：默认关闭，由用户自己选择开启——面板右上角「⋯」→「登录时启动」，或命令行 `MonitorBar --open-at-login on`（`off` 关闭）。

## 命令行

```bash
MonitorBar --show                 # 启动并打开面板（程序已运行时，再次启动也会打开面板）
MonitorBar --probe                # 打印显示器、接口、分辨率、DDC/CI 读数和局域网发现结果
MonitorBar --open-at-login on     # 开机自启（off 关闭）
MonitorBar --update               # 检查并安装新版本
```

## 工作原理

MonitorBar 有三条控制显示器的通道，启动时在后台探测，哪条可用就用哪条：

| 通道 | 能做什么 | 条件 |
| --- | --- | --- |
| 局域网遥控 | 切换输入源、调音量 | 澎湃 OS 显示器，与电脑在同一网络 |
| DDC/CI | 硬件背光、输入源、音量、对比度等 | 显示器支持 DDC/CI（部分需在显示器菜单开启） |
| 显卡 Gamma 表 | 软件调光、色温 | 任何显示器 |

输入源和音量优先走局域网，其次 DDC/CI；亮度有 DDC/CI 时调硬件背光，否则用软件调光。

- **局域网遥控**：澎湃 OS 的 6095 端口提供 `getsources`（信号源列表）、`changesource2`（按编号切换，能切到 DP / USB-C）、`getvolume`、`keyevent`（遥控按键，音量 ±1）。首次启动扫描所在 /24 网段找到显示器，之后记住地址。
- **DDC/CI**：macOS 通过 IOAVService 读写显示器的 I2C 总线（Apple Silicon）；Windows 通过系统的 Monitor Configuration API（dxva2）。输入源列表取自显示器的 MCCS 能力字符串。
- **macOS**：通过 [purego](https://github.com/ebitengine/purego) 调用 CoreGraphics、IOKit、AppKit（无 cgo）。
- **Windows**：通过 `syscall` 调用 user32、gdi32、dxva2、powrprof。

## 平台差异

- **软件调光**：Windows 只接受离标准曲线不远的 Gamma（最暗约 50%），更暗时退到允许的最暗并保留色温比例，面板会提示。macOS 无此限制。退出时两端都会恢复原始曲线。
- **跟随内屏亮度**：仅 macOS（读取 DisplayServices 的内屏亮度），且需外接显示器支持 DDC/CI 背光；Windows 暂不支持。
- **亮屏统计**：macOS 读显示器休眠状态；Windows 以「空闲时长 ≥ 电源计划的关闭显示器时间」推断。
- **面板位置**：从溢出区打开时面板贴在图标上方；拿不到图标位置时（如再次启动程序打开）贴在屏幕右下角。
- 数据保存在 `~/Library/Application Support/MonitorBar`（macOS）或 `%APPDATA%\MonitorBar`（Windows）。

<p align="center"><img src="docs/screenshots/windows.png" width="300" alt="Windows 11 上的面板"><br><sub>Windows 11 上的面板</sub></p>

## 开发

基于 [mygo](https://mygo.egoist.dev/)，需要 Go 1.27+（`go` 会自动下载对应工具链）。

```bash
go tool mygo dev                                         # 开发，改动后自动重启
go test ./...                                            # 无窗口的界面测试与单元测试
MONITORBAR_SNAPSHOTS=docs/screenshots go test -run TestSnapshots .   # 重新生成截图
go tool mygo build -platform darwin/universal,windows/amd64          # 打包两端（可在 Mac 上交叉编译 Windows 版）
```

发布：先在 `CHANGELOG.md` 写好这个版本的说明（应用内的更新提示会显示它），再推送版本标签。CI 会用仓库 Secret `MYGO_UPDATER_PRIVATE_KEY` 签名，打包 macOS dmg、Windows zip 与更新包，并创建 GitHub Release。

```bash
git tag v1.0.0 && git push origin v1.0.0
```

| 路径 | 职责 |
| --- | --- |
| `main.go` / `app.go` | 入口、托盘图标、弹出面板的尺寸与位置 |
| `store.go` | 状态与动作：只在主线程读写，网络请求在 goroutine 中，结果经 `post` 回到主线程 |
| `panel.go` / `tabs.go` / `widgets.go` / `icons.go` | 界面 |
| `internal/mitv` | 澎湃 OS 局域网接口与发现 |
| `internal/display` | 平台层：显示器信息、分辨率、Gamma、DDC/CI、亮屏与空闲时间 |
| `internal/usage` | 屏幕时间，每 30 秒采样，保留 60 天 |
| `internal/schedule` | 自动切换：按设定时间判断白天或夜晚 |

## 适配其他显示器

先运行 `MonitorBar --probe`，它会打印显示器名称、原生分辨率、DDC/CI 的读数与能力字符串，以及局域网发现结果。再按情况修改：

- **支持 DDC/CI，但某些设置不对**：VCP 编码在 `internal/display/ddc.go`，输入源名称在 `InputName`（个别型号与 MCCS 不一致的，加到 `inputOverrides`），面板显示哪些设置在 `store.go` 的 `controlCatalog`。
- **DDC/CI 没有响应**：先在显示器菜单里找 DDC/CI 开关。macOS 的读写协议在 `internal/display/ddc_darwin.go`，Windows 在 `ddc_windows.go`。注意部分 Mac 的 HDMI 口不传 DDC/CI，可改用 DP 或 USB-C 连接。
- **其他品牌的局域网接口**：参照 `internal/mitv` 写一个新的通道，在 `store.go` 里接入输入源和音量的路由（`sources`、`selectInput`、`setVolume`）。

## 致谢

- [mygo](https://github.com/egoist/mygo)：Go 桌面应用框架
- [LGwebOS-menubar](https://github.com/gabrielgreco1/LGwebOS-menubar)：界面设计的灵感来源
- 图标风格参考 [Lucide](https://lucide.dev)

## English

REDMI monitors do not support DDC/CI, so the usual tools (MonitorControl, BetterDisplay, Lunar) cannot switch their inputs. They run HyperOS, though, whose LAN remote API can, DP and USB-C included. MonitorBar manages an external monitor from the macOS menu bar or the Windows notification area. It switches inputs and changes volume over the LAN remote API of Xiaomi HyperOS monitors, or over DDC/CI on monitors that support it (backlight and picture controls too), dims and warms the picture through the gamma table, switches resolutions and refresh rates, and tracks screen time. It is one Go codebase built with [mygo](https://mygo.egoist.dev/), drawing native UI on both platforms; the Windows build cross-compiles from a Mac. See [Development](#开发) for build commands.

## 许可证

[MIT](LICENSE)
