import AppKit
import SwiftUI
import Combine

final class FloatingPanel: NSPanel {
    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }
}

private struct PanelArrow: Shape {
    func path(in rect: CGRect) -> Path {
        var path = Path()
        path.move(to: CGPoint(x: rect.minX, y: rect.maxY))
        path.addQuadCurve(to: CGPoint(x: rect.midX, y: rect.minY), control: CGPoint(x: rect.midX - 3, y: rect.maxY - 2))
        path.addQuadCurve(to: CGPoint(x: rect.maxX, y: rect.maxY), control: CGPoint(x: rect.midX + 3, y: rect.maxY - 2))
        path.closeSubpath()
        return path
    }
}

/// Arrow + rounded panel; reports its natural height so the window can hug the content.
private struct PanelShell: View {
    @ObservedObject var store: MonitorStore
    let arrowX: CGFloat
    var body: some View {
        VStack(spacing: 0) {
            PanelArrow().fill(panelBackground).frame(width: 22, height: 10)
                .frame(width: panelWidth, alignment: .leading).offset(x: arrowX - 11)
            PanelView(store: store)
                .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).strokeBorder(Color.white.opacity(0.12), lineWidth: 0.7))
        }
        .fixedSize(horizontal: false, vertical: true)
        .onGeometryChange(for: CGFloat.self) { $0.size.height } action: { store.panelHeight = $0 }
        .frame(width: panelWidth)
        .frame(maxHeight: .infinity, alignment: .top)
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    let store = MonitorStore()
    private var statusItem: NSStatusItem!
    private let panel = FloatingPanel(contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
    private var host: NSHostingController<PanelShell>?
    private var eventMonitors: [Any] = []
    private var heightSubscription: AnyCancellable?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        statusItem.button?.image = NSImage(systemSymbolName: "display", accessibilityDescription: "MonitorBar")
        statusItem.button?.target = self
        statusItem.button?.action = #selector(toggle)

        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = true
        panel.level = .popUpMenu
        panel.isReleasedWhenClosed = false
        panel.hidesOnDeactivate = false
        panel.collectionBehavior = [.transient, .fullScreenAuxiliary, .canJoinAllSpaces]
        panel.appearance = NSAppearance(named: .darkAqua)

        heightSubscription = store.$panelHeight.removeDuplicates().sink { [weak self] _ in
            DispatchQueue.main.async { if self?.panel.isVisible == true { self?.position() } }
        }
        if let monitor = NSEvent.addGlobalMonitorForEvents(matching: [.leftMouseDown, .rightMouseDown], handler: { [weak self] _ in
            DispatchQueue.main.async { self?.hide() }
        }) { eventMonitors.append(monitor) }
        if let monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown, handler: { [weak self] event in
            guard event.keyCode == 53, self?.panel.isVisible == true else { return event } // Esc
            self?.hide()
            return nil
        }) { eventMonitors.append(monitor) }

        // Pop the panel on first launch (or with --show) so new users find the menu bar icon.
        let firstLaunch = !UserDefaults.standard.bool(forKey: "launchedBefore")
        UserDefaults.standard.set(true, forKey: "launchedBefore")
        if firstLaunch || CommandLine.arguments.contains("--show") {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) { self.show() }
        }
    }

    @objc private func toggle() { panel.isVisible ? hide() : show() }

    private func show() {
        store.panelOpened()
        position()
        NSApp.activate()
        panel.makeKeyAndOrderFront(nil)
    }

    private func hide() { panel.orderOut(nil) }

    private func position() {
        guard let button = statusItem.button, let window = button.window else { return }
        let anchor = window.convertToScreen(button.convert(button.bounds, to: nil))
        let visible = (window.screen ?? NSScreen.main!).visibleFrame
        store.maxContentHeight = visible.height - 240
        let top = min(anchor.minY - 3, visible.maxY - 3)
        let height = min(store.panelHeight, top - visible.minY - 8)
        let x = min(max(anchor.midX - panelWidth / 2, visible.minX + 8), visible.maxX - panelWidth - 8)
        let root = PanelShell(store: store, arrowX: min(panelWidth - 18, max(18, anchor.midX - x)))
        if let host {
            host.rootView = root
        } else {
            let controller = NSHostingController(rootView: root)
            controller.sizingOptions = []
            host = controller
            panel.contentViewController = controller
        }
        panel.setFrame(NSRect(x: x, y: top - height, width: panelWidth, height: height), display: true)
    }

    func applicationWillTerminate(_ notification: Notification) {
        store.restoreColors()
        eventMonitors.forEach(NSEvent.removeMonitor)
    }
}

/// `MonitorBar --probe`: print what the app can see and control, without UI.
private func probe() {
    let display = Display.target()
    print("显示器: \(Display.name(of: display))  接口: \(Display.connection() ?? "未知")  ID: \(display)")
    if let mode = Display.currentMode(of: display) { print("当前模式: \(mode.title) \(mode.isHiDPI ? "HiDPI" : "") \(mode.refreshRate)Hz") }
    print("可用模式: \(Display.modes(of: display).count) 个")
    let network = DispatchSemaphore(value: 0)
    Task.detached {
        if let host = await MiTVLink.discover(), let name = await MiTVLink(host: host).deviceName() {
            let link = MiTVLink(host: host)
            let sources = (try? await link.sources())?.map { "\($0.name)=\($0.value)" }.joined(separator: " ") ?? "读取失败"
            print("局域网: \(name) @ \(host)  信号源: \(sources)  音量: \((try? await link.volume()).map(String.init) ?? "?")")
        } else {
            print("局域网: 未发现澎湃 OS 显示器")
        }
        network.signal()
    }
    network.wait()
    guard let channel = DDCChannel.external() else { return print("DDC: 找不到外接显示器的 AV 服务") }
    for (code, name) in [(VCP.brightness, "亮度"), (VCP.contrast, "对比度"), (VCP.volume, "音量"), (VCP.input, "输入源")] {
        let reading = channel.read(code)
        print(String(format: "DDC 0x%02X %@: ", code, name) + (reading.map { "\($0.value) / \($0.max)" } ?? "无响应"))
    }
    print("能力字符串: \(channel.capabilities() ?? "无响应")")
}

if CommandLine.arguments.contains("--probe") {
    probe()
} else {
    let delegate = MainActor.assumeIsolated { AppDelegate() }
    NSApplication.shared.delegate = delegate
    withExtendedLifetime(delegate) { NSApplication.shared.run() }
}
