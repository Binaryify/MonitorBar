import SwiftUI
import AppKit

let panelWidth: CGFloat = 360
let panelBackground = Color(red: 0.072, green: 0.075, blue: 0.084)
private let accent = Color(red: 1, green: 0.41, blue: 0)   // Xiaomi orange
private let mutedInk = Color.white.opacity(0.5)

private enum PanelTab: String, CaseIterable, Identifiable {
    case overview = "概览", picture = "画面", modes = "分辨率", activity = "屏幕时间"
    var id: String { rawValue }
    var symbol: String {
        switch self {
        case .overview: "display"
        case .picture: "slider.horizontal.3"
        case .modes: "aspectratio"
        case .activity: "chart.bar.xaxis"
        }
    }
}

// MARK: - Building blocks

private struct Card<Content: View>: View {
    @ViewBuilder let content: Content
    var body: some View {
        content.padding(12).frame(maxWidth: .infinity, alignment: .leading)
            .background(RoundedRectangle(cornerRadius: 12, style: .continuous).fill(Color.white.opacity(0.065)))
            .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).strokeBorder(Color.white.opacity(0.08), lineWidth: 0.7))
    }
}

private struct ChipStyle: ButtonStyle {
    var selected = false
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .foregroundStyle(selected ? accent : Color.white.opacity(0.82))
            .background(RoundedRectangle(cornerRadius: 9, style: .continuous)
                .fill(selected ? accent.opacity(0.18) : Color.white.opacity(configuration.isPressed ? 0.13 : 0.06)))
            .overlay(RoundedRectangle(cornerRadius: 9, style: .continuous).strokeBorder(selected ? accent.opacity(0.55) : .clear, lineWidth: 0.8))
            .scaleEffect(configuration.isPressed ? 0.97 : 1)
            .contentShape(Rectangle())
    }
}

private struct CardTitle: View {
    let title: String
    let symbol: String
    var trailing: String?
    var body: some View {
        HStack {
            Label(title, systemImage: symbol).font(.system(size: 12, weight: .semibold))
            Spacer()
            if let trailing { Text(trailing).font(.system(size: 10)).foregroundStyle(mutedInk) }
        }
    }
}

private struct SliderCard: View {
    let title: String
    let caption: String
    let symbol: String
    let value: Double
    var range: ClosedRange<Double> = 0...100
    var step: Double = 5
    var display: (Double) -> String = { "\(Int($0.rounded()))" }
    let onChange: (Double) -> Void

    var body: some View {
        Card {
            VStack(spacing: 10) {
                HStack(spacing: 8) {
                    Image(systemName: symbol).foregroundStyle(mutedInk).frame(width: 18)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(title).fontWeight(.semibold)
                        Text(caption).font(.system(size: 10)).foregroundStyle(mutedInk)
                    }
                    Spacer()
                    Text(display(value)).font(.system(size: 20, weight: .semibold, design: .rounded))
                        .monospacedDigit().contentTransition(.numericText())
                }
                HStack(spacing: 8) {
                    stepButton("minus", -step)
                    Slider(value: Binding(get: { value }, set: onChange), in: range).accessibilityLabel(title)
                    stepButton("plus", step)
                }
            }
        }
    }

    private func stepButton(_ symbol: String, _ delta: Double) -> some View {
        Button { onChange(min(max(value + delta, range.lowerBound), range.upperBound)) } label: {
            Image(systemName: symbol).font(.system(size: 10, weight: .semibold)).frame(width: 24, height: 22)
        }.buttonStyle(ChipStyle())
    }
}

// MARK: - Panel

struct PanelView: View {
    @ObservedObject var store: MonitorStore
    @AppStorage("tab") private var tab = PanelTab.overview
    @State private var contentHeight: CGFloat = 400

    var body: some View {
        VStack(spacing: 0) {
            header.padding(.top, 16).padding(.bottom, 14)
            tabs.padding(.bottom, 12)
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    if tab != .overview { sectionTitle(tab.rawValue) }
                    switch tab {
                    case .overview: overview
                    case .picture: picture
                    case .modes: modes
                    case .activity: activity
                    }
                    if let notice = store.notice {
                        Text(notice).font(.system(size: 11)).foregroundStyle(mutedInk).fixedSize(horizontal: false, vertical: true)
                    }
                }
                .padding(.bottom, 4)
                .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { contentHeight = $0 }
            }
            .scrollIndicators(.never)
            .frame(height: min(contentHeight, store.maxContentHeight))
            footer.padding(.vertical, 12)
        }
        .padding(.horizontal, 13)
        .frame(width: panelWidth)
        .background(LinearGradient(colors: [panelBackground, Color(white: 0.04)], startPoint: .topLeading, endPoint: .bottomTrailing))
        .foregroundStyle(Color.white.opacity(0.87))
        .font(.system(size: 12))
        .tint(accent)
        .environment(\.colorScheme, .dark)
        .onChange(of: tab) { store.notice = nil }
    }

    private var header: some View {
        HStack(spacing: 10) {
            Image(systemName: "display").font(.system(size: 15, weight: .semibold)).foregroundStyle(.white)
                .frame(width: 32, height: 32)
                .background(RoundedRectangle(cornerRadius: 9, style: .continuous)
                    .fill(LinearGradient(colors: [accent, Color(red: 1, green: 0.27, blue: 0)], startPoint: .top, endPoint: .bottom)))
            VStack(alignment: .leading, spacing: 3) {
                Text(store.name).font(.system(size: 16, weight: .semibold)).tracking(-0.3).lineLimit(1)
                Text(headerCaption).font(.system(size: 10, weight: .medium)).foregroundStyle(mutedInk)
            }
            Spacer()
            Menu {
                Button("重新检测硬件控制") { Task { await store.refreshControls() } }
                Button("打开系统显示器设置…") {
                    NSWorkspace.shared.open(URL(string: "x-apple.systempreferences:com.apple.Displays-Settings.extension")!)
                }
                Toggle("登录时启动", isOn: Binding(get: { store.launchAtLogin }, set: { _ in store.toggleLaunchAtLogin() }))
                Divider()
                Button("退出 MonitorBar") { NSApp.terminate(nil) }
            } label: {
                Image(systemName: "ellipsis").frame(width: 24, height: 30).foregroundStyle(mutedInk)
            }.menuStyle(.borderlessButton).menuIndicator(.hidden).fixedSize()
            Button { store.sleepDisplay() } label: {
                Image(systemName: "power").font(.system(size: 15, weight: .medium))
                    .frame(width: 32, height: 32).background(Circle().fill(Color.white.opacity(0.07)))
            }.buttonStyle(.plain).help("立即熄屏")
        }
    }

    private var headerCaption: String {
        var parts = ["外接显示器"]
        if let connection = store.connection { parts.append(connection) }
        if let native = store.modes.max(by: { $0.pixelWidth < $1.pixelWidth }) { parts.append(native.pixelWidth >= 3840 ? "4K" : "\(native.pixelWidth)p") }
        if let top = store.modes.map(\.refreshRate).max() { parts.append("\(top)Hz") }
        return parts.joined(separator: " · ")
    }

    private var tabs: some View {
        HStack(spacing: 4) {
            ForEach(PanelTab.allCases) { item in
                Button { tab = item } label: {
                    Image(systemName: item.symbol).font(.system(size: 15, weight: .medium))
                        .foregroundStyle(tab == item ? accent : mutedInk)
                        .frame(maxWidth: .infinity).frame(height: 34)
                        .background(RoundedRectangle(cornerRadius: 9).fill(tab == item ? accent.opacity(0.2) : .clear))
                        .contentShape(Rectangle())
                }.buttonStyle(.plain).help(item.rawValue).accessibilityLabel(item.rawValue)
            }
        }
        .padding(4)
        .background(RoundedRectangle(cornerRadius: 12).fill(Color.white.opacity(0.066)))
        .overlay(RoundedRectangle(cornerRadius: 12).strokeBorder(Color.white.opacity(0.08), lineWidth: 0.7))
    }

    private func sectionTitle(_ title: String) -> some View {
        Text(title).font(.system(size: 11, weight: .bold)).tracking(0.8).foregroundStyle(mutedInk).padding(.horizontal, 1)
    }

    private var footer: some View {
        HStack(spacing: 6) {
            Circle().fill(statusColor).frame(width: 6, height: 6)
            Text(statusText).font(.system(size: 10)).foregroundStyle(mutedInk)
            Spacer()
            if store.ddc == .probing { ProgressView().controlSize(.mini) }
        }
    }

    private var statusColor: Color {
        if store.hasHardwareControl { return .green }
        return store.ddc == .probing ? .yellow : accent
    }

    private var statusText: String {
        if store.network != nil { return "已通过局域网连接 · 澎湃 OS 遥控" }
        switch store.ddc {
        case .ready: return "DDC/CI 已连接 · 硬件控制"
        case .probing: return "正在检测显示器…"
        case .unavailable: return "软件模式 · 未找到硬件控制通道"
        }
    }

    // MARK: Overview

    private var overview: some View {
        VStack(spacing: 12) {
            hero
            if !store.inputs.isEmpty { inputsCard }
            if !store.hasHardwareControl && store.ddc == .unavailable { controlNotice }
            brightnessCard
            if let volume = store.volume { volumeCard(volume) }
            scenesCard
        }
    }

    private var hero: some View {
        let glow = store.effectiveBrightness / 100
        return VStack(spacing: 0) {
            ZStack {
                Ellipse().fill(accent.opacity(0.08 + 0.22 * glow)).frame(width: 240, height: 100).blur(radius: 40).offset(y: 4)
                VStack(spacing: 0) {
                    ZStack {
                        RoundedRectangle(cornerRadius: 8, style: .continuous).fill(Color(white: 0.015))
                        RoundedRectangle(cornerRadius: 5, style: .continuous)
                            .fill(LinearGradient(colors: [accent.opacity(0.05 + 0.3 * glow), Color(white: 0.05), .black],
                                                 startPoint: .topLeading, endPoint: .bottomTrailing))
                            .overlay(LinearGradient(colors: [Color.white.opacity(0.09), .clear], startPoint: .topLeading, endPoint: .center)
                                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous)))
                            .padding(4)
                        VStack(spacing: 5) {
                            Text(store.currentMode?.title ?? "—").font(.system(size: 22, weight: .semibold, design: .rounded)).tracking(-0.4)
                            Text(modeCaption).font(.system(size: 10, weight: .medium)).foregroundStyle(Color.white.opacity(0.6))
                        }.shadow(color: accent.opacity(0.45), radius: 14)
                    }
                    .frame(width: 268, height: 151)
                    .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).strokeBorder(Color.white.opacity(0.16), lineWidth: 0.8))
                    LinearGradient(colors: [Color(white: 0.2), Color(white: 0.1)], startPoint: .top, endPoint: .bottom).frame(width: 30, height: 18)
                    Capsule().fill(Color(white: 0.2)).frame(width: 108, height: 5)
                }
            }.frame(height: 186)
            HStack(spacing: 5) {
                Circle().fill(accent).frame(width: 4, height: 4)
                Text("当前画面").font(.system(size: 9, weight: .bold)).tracking(1.1).foregroundStyle(mutedInk)
                Spacer()
                if let connection = store.connection { Text(connection).font(.system(size: 9, weight: .semibold)).foregroundStyle(accent) }
            }.padding(.horizontal, 6).padding(.top, 8)
        }
    }

    private var modeCaption: String {
        guard let mode = store.currentMode else { return "" }
        return mode.isHiDPI ? "HiDPI · 渲染 \(mode.pixelWidth)×\(mode.pixelHeight) · \(mode.refreshRate) Hz" : "\(mode.refreshRate) Hz"
    }

    private var controlNotice: some View {
        Card {
            VStack(alignment: .leading, spacing: 8) {
                Label("硬件控制未就绪", systemImage: "exclamationmark.triangle").font(.system(size: 12, weight: .semibold)).foregroundStyle(accent)
                Text("显示器不响应 DDC/CI，局域网里也没找到澎湃 OS 显示器，亮度已改用软件调光。请确认显示器已联网，或在 OSD 中开启「DDC/CI」后重新检测。")
                    .font(.system(size: 11)).foregroundStyle(Color.white.opacity(0.7)).fixedSize(horizontal: false, vertical: true)
                Button { Task { await store.refreshControls() } } label: {
                    Text("重新检测").font(.system(size: 11, weight: .medium)).padding(.horizontal, 12).padding(.vertical, 5)
                }.buttonStyle(ChipStyle(selected: true))
            }
        }
    }

    private var brightnessCard: some View {
        SliderCard(title: "亮度", caption: store.ddc == .ready ? "硬件背光 · DDC" : "软件调光 · Gamma",
                   symbol: "sun.max.fill", value: store.effectiveBrightness, range: store.brightnessRange, onChange: store.setBrightness)
    }

    private func volumeCard(_ volume: Double) -> some View {
        Card {
            VStack(spacing: 10) {
                HStack {
                    Button { store.toggleMute() } label: {
                        Image(systemName: store.muted ? "speaker.slash.fill" : "speaker.wave.2.fill")
                            .foregroundStyle(store.muted ? accent : mutedInk).frame(width: 18)
                    }.buttonStyle(.plain).disabled(!store.canMute).help(store.canMute ? (store.muted ? "取消静音" : "静音") : "")
                    Text("显示器音量").fontWeight(.semibold)
                    Spacer()
                    Text("\(Int(volume.rounded()))").font(.system(size: 20, weight: .semibold, design: .rounded)).monospacedDigit()
                }
                Slider(value: Binding(get: { volume }, set: store.setVolume), in: 0...100).accessibilityLabel("显示器音量")
            }
        }
    }

    private var inputsCard: some View {
        Card {
            VStack(spacing: 10) {
                CardTitle(title: "输入源", symbol: "cable.connector", trailing: store.network != nil ? "局域网" : "DDC")
                LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 6), count: 4), spacing: 6) {
                    ForEach(store.inputs) { source in
                        Button { store.select(source) } label: {
                            Text(source.name).font(.system(size: 11, weight: .medium)).frame(maxWidth: .infinity).frame(height: 30)
                        }.buttonStyle(ChipStyle(selected: store.input == source.value))
                    }
                }
            }
        }
    }

    private var scenesCard: some View {
        Card {
            VStack(spacing: 10) {
                CardTitle(title: "场景", symbol: "sparkles", trailing: "亮度 + 色温")
                HStack(spacing: 6) {
                    ForEach(PictureScene.all) { scene in
                        Button { store.apply(scene) } label: {
                            VStack(spacing: 5) {
                                Image(systemName: scene.symbol).font(.system(size: 14))
                                Text(scene.name).font(.system(size: 10, weight: .medium))
                            }.frame(maxWidth: .infinity).frame(height: 50)
                        }.buttonStyle(ChipStyle(selected: store.isActive(scene)))
                    }
                }
            }
        }
    }

    // MARK: Picture

    private var picture: some View {
        VStack(spacing: 12) {
            scenesCard
            SliderCard(title: "软件调光", caption: "在硬件亮度基础上再压暗，适合夜间", symbol: "circle.lefthalf.filled",
                       value: store.softBrightness, range: 20...100, onChange: store.setSoftBrightness)
            SliderCard(title: "色温", caption: "向右更暖，减少蓝光", symbol: "thermometer.sun",
                       value: store.warmth, display: { $0 < 1 ? "标准" : "\(Int($0.rounded()))" }, onChange: store.setWarmth)
            if store.ddc == .ready {
                ForEach(store.controls) { control in
                    SliderCard(title: control.name, caption: "硬件 · DDC", symbol: "dial.medium",
                               value: Double(control.value), range: 0...Double(max(control.max, 1)), step: 1) { store.setControl(control.code, Int($0.rounded())) }
                }
            } else {
                Text("对比度、锐度、RGB 增益需要显示器支持 DDC/CI。").font(.system(size: 11)).foregroundStyle(mutedInk)
            }
            Button { store.resetSoftware() } label: {
                Label("恢复软件调整", systemImage: "arrow.counterclockwise").font(.system(size: 11, weight: .medium))
                    .frame(maxWidth: .infinity).frame(height: 30)
            }.buttonStyle(ChipStyle())
        }
    }

    // MARK: Modes

    private var modes: some View {
        let current = store.currentMode
        let rate = current?.refreshRate ?? 60
        let rates = store.modes.filter { $0.sizeKey == current?.sizeKey }.sorted { $0.refreshRate > $1.refreshRate }
        return VStack(spacing: 12) {
            if rates.count > 1 {
                Card {
                    VStack(spacing: 10) {
                        CardTitle(title: "刷新率", symbol: "speedometer", trailing: current?.title)
                        LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 6), count: 4), spacing: 6) {
                            ForEach(rates) { mode in
                                Button { store.apply(mode) } label: {
                                    Text("\(mode.refreshRate) Hz").font(.system(size: 11, weight: .medium)).frame(maxWidth: .infinity).frame(height: 30)
                                }.buttonStyle(ChipStyle(selected: mode.refreshRate == rate))
                            }
                        }
                    }
                }
            }
            resolutionList("HiDPI（清晰）", symbol: "sparkle.magnifyingglass", modes: Display.resolutions(in: store.modes, hiDPI: true, refreshRate: rate))
            resolutionList("标准", symbol: "rectangle.on.rectangle", modes: Display.resolutions(in: store.modes, hiDPI: false, refreshRate: rate))
        }
    }

    @ViewBuilder
    private func resolutionList(_ title: String, symbol: String, modes: [DisplayMode]) -> some View {
        if !modes.isEmpty {
            Card {
                VStack(spacing: 4) {
                    CardTitle(title: title, symbol: symbol, trailing: "\(modes.count) 种").padding(.bottom, 4)
                    ForEach(modes) { mode in
                        let selected = mode.sizeKey == store.currentMode?.sizeKey
                        Button { store.apply(mode) } label: {
                            HStack {
                                Text(mode.title).font(.system(size: 12, weight: selected ? .semibold : .regular)).monospacedDigit()
                                Spacer()
                                Text(verbatim: mode.isHiDPI ? "渲染 \(mode.pixelWidth)×\(mode.pixelHeight)" : "\(mode.refreshRate) Hz")
                                    .font(.system(size: 10)).foregroundStyle(mutedInk)
                                Image(systemName: "checkmark").font(.system(size: 10, weight: .bold)).foregroundStyle(accent).opacity(selected ? 1 : 0)
                            }.padding(.horizontal, 9).frame(height: 28)
                                .background(RoundedRectangle(cornerRadius: 8).fill(selected ? accent.opacity(0.14) : .clear))
                                .contentShape(Rectangle())
                        }.buttonStyle(.plain)
                    }
                }
            }
        }
    }

    // MARK: Activity

    private var activity: some View {
        let today = store.usage.last ?? DayUsage(day: "")
        let peak = max(store.usage.map(\.awake).max() ?? 0, 3600)
        let recorded = store.usage.filter { $0.awake > 0 }
        return VStack(spacing: 12) {
            HStack(spacing: 10) {
                statTile("今日亮屏", duration(today.awake), "sun.max")
                statTile("今日使用", duration(today.active), "keyboard")
            }
            Card {
                VStack(spacing: 12) {
                    CardTitle(title: "最近 7 天", symbol: "calendar", trailing: "日均 \(duration(recorded.map(\.awake).reduce(0, +) / max(recorded.count, 1)))")
                    HStack(alignment: .bottom, spacing: 8) {
                        ForEach(store.usage) { day in
                            VStack(spacing: 6) {
                                ZStack(alignment: .bottom) {
                                    RoundedRectangle(cornerRadius: 4).fill(Color.white.opacity(0.06)).frame(height: 110)
                                    RoundedRectangle(cornerRadius: 4).fill(accent.opacity(0.3)).frame(height: 110 * CGFloat(day.awake) / CGFloat(peak))
                                    RoundedRectangle(cornerRadius: 4).fill(accent).frame(height: 110 * CGFloat(day.active) / CGFloat(peak))
                                }
                                Text(weekday(day.day)).font(.system(size: 9, weight: day.id == today.id ? .bold : .regular))
                                    .foregroundStyle(day.id == today.id ? accent : mutedInk)
                            }.frame(maxWidth: .infinity)
                                .help("\(day.day)  亮屏 \(duration(day.awake)) · 使用 \(duration(day.active))")
                        }
                    }
                    HStack(spacing: 12) {
                        legend(accent.opacity(0.3), "亮屏")
                        legend(accent, "有键鼠操作")
                        Spacer()
                    }
                }
            }
            Text("亮屏：显示器点亮的时长；使用：两分钟内有键盘或鼠标操作的时长。MonitorBar 运行时才会记录。")
                .font(.system(size: 10)).foregroundStyle(mutedInk).fixedSize(horizontal: false, vertical: true)
        }
    }

    private func statTile(_ title: String, _ value: String, _ symbol: String) -> some View {
        Card {
            VStack(alignment: .leading, spacing: 6) {
                Label(title, systemImage: symbol).font(.system(size: 10, weight: .medium)).foregroundStyle(mutedInk)
                Text(value).font(.system(size: 22, weight: .semibold, design: .rounded)).monospacedDigit()
            }
        }
    }

    private func legend(_ color: Color, _ title: String) -> some View {
        HStack(spacing: 4) {
            RoundedRectangle(cornerRadius: 2).fill(color).frame(width: 8, height: 8)
            Text(title).font(.system(size: 9)).foregroundStyle(mutedInk)
        }
    }

    private func duration(_ seconds: Int) -> String {
        let minutes = seconds / 60
        return minutes < 60 ? "\(minutes) 分钟" : "\(minutes / 60) 小时 \(minutes % 60) 分"
    }

    private func weekday(_ day: String) -> String {
        let parser = DateFormatter()
        parser.locale = Locale(identifier: "en_US_POSIX")
        parser.dateFormat = "yyyy-MM-dd"
        guard let date = parser.date(from: day) else { return "" }
        if Calendar.current.isDateInToday(date) { return "今天" }
        let label = DateFormatter()
        label.locale = Locale(identifier: "zh_CN")
        label.dateFormat = "EEE"
        return label.string(from: date)
    }
}
