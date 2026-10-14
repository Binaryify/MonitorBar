# MonitorBar

macOS 状态栏显示器管理工具（Apple Silicon，macOS 14+），界面参考 [LGwebOS-menubar](https://github.com/gabrielgreco1/LGwebOS-menubar)。

| 分页 | 功能 |
| --- | --- |
| 概览 | 当前分辨率/刷新率、亮度、音量†、输入源切换†、场景（日间/阅读/观影/夜间） |
| 画面 | 软件调光、色温、对比度/锐度/RGB 增益* |
| 分辨率 | 刷新率切换、HiDPI / 标准分辨率列表 |
| 屏幕时间 | 今日亮屏与使用时长、最近 7 天柱状图 |

† 优先走局域网：澎湃 OS 显示器（如 REDMI G Pro 27U 2026）通过 Bonjour `_rc._tcp` 自动发现，调用其 6095 端口的遥控接口（`getsources` / `changesource2` / `getvolume` / `keyevent`）。找不到时再尝试 DDC/CI。

\* 需要显示器响应 DDC/CI。亮度在没有 DDC 时自动退回软件模式（Gamma 调光与色温，不依赖连接线）。

`windows/switch-to-mac.bat`：放到 Windows 上双击，把显示器切回 HDMI1（Mac）。

## 构建

```bash
./scripts/build.sh            # 产出 dist/MonitorBar.app
./scripts/build.sh --install  # 同时复制到 ~/Applications
```

## 诊断

```bash
dist/MonitorBar.app/Contents/MacOS/MonitorBar --probe
```

打印显示器名称、接口、当前模式、局域网发现结果（信号源列表、音量），以及 DDC 读取结果。

## 结构

- `MiTVLink.swift`：澎湃 OS 局域网遥控接口与 Bonjour 发现
- `DDC.swift`：DDC/CI 读写（私有 IOAVService I2C）与能力字符串解析
- `Display.swift`：目标显示器、接口类型、显示模式的查询与切换
- `Gamma.swift`：基于 Gamma 表的软件调光与色温
- `ScreenTime.swift`：每 30 秒采样一次，保留 60 天数据
- `MonitorStore.swift`：状态与动作；DDC 可用时走硬件，否则走软件
- `PanelView.swift` / `main.swift`：SwiftUI 面板与状态栏入口
