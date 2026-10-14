import AppKit
import Combine
import ServiceManagement

enum DDCState { case probing, ready, unavailable }

struct InputSource: Identifiable {
    let value: Int
    let name: String
    var id: Int { value }

    /// Named after the MCCS input code reported over DDC.
    static func ddc(_ value: Int) -> InputSource {
        let names = [0x01: "VGA", 0x03: "DVI", 0x0F: "DP", 0x10: "DP 2", 0x11: "HDMI 1", 0x12: "HDMI 2", 0x13: "HDMI 3", 0x19: "USB-C", 0x1B: "USB-C"]
        return InputSource(value: value, name: names[value] ?? String(format: "输入 %02X", value))
    }
}

struct DDCControl: Identifiable {
    let code: UInt8
    let name: String
    var value: Int
    let max: Int
    var id: UInt8 { code }
    /// Optional picture controls shown when the monitor answers them.
    static let catalog: [(UInt8, String)] = [(VCP.contrast, "对比度"), (VCP.sharpness, "锐度"),
                                            (VCP.redGain, "红色增益"), (VCP.greenGain, "绿色增益"), (VCP.blueGain, "蓝色增益")]
}

struct PictureScene: Identifiable {
    let name: String
    let symbol: String
    let brightness: Double
    let warmth: Double
    var id: String { name }
    static let all = [PictureScene(name: "日间", symbol: "sun.max", brightness: 80, warmth: 0),
                      PictureScene(name: "阅读", symbol: "book", brightness: 55, warmth: 25),
                      PictureScene(name: "观影", symbol: "film", brightness: 100, warmth: 5),
                      PictureScene(name: "夜间", symbol: "moon.stars", brightness: 30, warmth: 55)]
}

@MainActor
final class MonitorStore: ObservableObject {
    @Published private(set) var name = "显示器"
    @Published private(set) var connection: String?
    @Published private(set) var ddc = DDCState.probing
    @Published private(set) var network: MiTVLink?          // HyperOS LAN control; preferred for inputs and volume
    @Published private(set) var brightness: Double = 100    // hardware %, meaningful when ddc == .ready
    @Published private(set) var volume: Double?
    @Published private(set) var muted = false
    @Published private(set) var input: Int?
    @Published private(set) var inputs: [InputSource] = []
    @Published private(set) var controls: [DDCControl] = []
    @Published private(set) var softBrightness: Double      // gamma %, 20...100
    @Published private(set) var warmth: Double              // 0...100
    @Published private(set) var modes: [DisplayMode] = []
    @Published private(set) var currentMode: DisplayMode?
    @Published private(set) var usage: [DayUsage] = []
    @Published private(set) var launchAtLogin = SMAppService.mainApp.status == .enabled
    @Published var notice: String?
    @Published var panelHeight: CGFloat = 600
    var maxContentHeight: CGFloat = 640

    private(set) var display = Display.target()
    private var channel: DDCChannel?
    private var maxima: [UInt8: Int] = [:]
    private var pendingWrites: [UInt8: Int] = [:]
    private var flushing = false
    private var networkVolume = 0
    private var volumeTarget: Int?
    private var chasingVolume = false
    private let ddcQueue = DispatchQueue(label: "MonitorBar.ddc")
    private let gamma = GammaTable()
    private let tracker = ScreenTimeTracker()
    private var observers: [NSObjectProtocol] = []
    private var timer: Timer?

    init() {
        softBrightness = UserDefaults.standard.object(forKey: "softBrightness") as? Double ?? 100
        warmth = UserDefaults.standard.double(forKey: "warmth")
        refreshDisplay()
        applyGamma()
        usage = tracker.recent(7)
        let refresh: @Sendable (Notification) -> Void = { [weak self] _ in MainActor.assumeIsolated { self?.screensChanged() } }
        observers = [NotificationCenter.default.addObserver(forName: NSApplication.didChangeScreenParametersNotification, object: nil, queue: .main, using: refresh),
                     NSWorkspace.shared.notificationCenter.addObserver(forName: NSWorkspace.screensDidWakeNotification, object: nil, queue: .main, using: refresh)]
        timer = Timer.scheduledTimer(withTimeInterval: ScreenTimeTracker.interval, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.recordUsage() }
        }
        Task { await refreshControls() }
    }

    // MARK: Unified picture controls (hardware when DDC works, software otherwise)

    var hasHardwareControl: Bool { network != nil || ddc == .ready }
    var canMute: Bool { network == nil && ddc == .ready }
    var effectiveBrightness: Double { ddc == .ready ? brightness : softBrightness }
    var brightnessRange: ClosedRange<Double> { ddc == .ready ? 0...100 : 20...100 }

    func setBrightness(_ percent: Double) {
        guard ddc == .ready else { return setSoftBrightness(percent) }
        brightness = percent
        write(VCP.brightness, percent: percent)
    }

    func apply(_ scene: PictureScene) {
        setBrightness(scene.brightness)
        setWarmth(scene.warmth)
    }

    func isActive(_ scene: PictureScene) -> Bool {
        abs(effectiveBrightness - scene.brightness) < 1 && abs(warmth - scene.warmth) < 1
    }

    // MARK: Software (gamma)

    func setSoftBrightness(_ percent: Double) {
        softBrightness = percent
        UserDefaults.standard.set(percent, forKey: "softBrightness")
        applyGamma()
    }

    func setWarmth(_ percent: Double) {
        warmth = percent
        UserDefaults.standard.set(percent, forKey: "warmth")
        applyGamma()
    }

    func resetSoftware() {
        setSoftBrightness(100)
        setWarmth(0)
    }

    private func applyGamma() {
        gamma.apply(display: display, brightness: softBrightness / 100, warmth: warmth / 100)
    }

    // MARK: Hardware (DDC)

    func setVolume(_ percent: Double) {
        volume = percent
        guard network != nil else { return write(VCP.volume, percent: percent) }
        volumeTarget = Int(percent.rounded())
        chaseVolume()
    }

    func toggleMute() {
        muted.toggle()
        queueWrite(VCP.mute, muted ? 1 : 2)
    }

    func select(_ source: InputSource) {
        input = source.value
        guard let network else { return queueWrite(VCP.input, source.value) }
        Task {
            do { try await network.select(source.value) } catch { notice = "切换到 \(source.name) 失败：显示器没有响应" }
        }
    }

    func setControl(_ code: UInt8, _ value: Int) {
        guard let index = controls.firstIndex(where: { $0.code == code }) else { return }
        controls[index].value = value
        queueWrite(code, value)
    }

    func probeDDC() async {
        ddc = .probing
        channel = DDCChannel.external()
        guard await read(VCP.brightness) != nil else { ddc = .unavailable; return }
        let supported = await onDDC { $0.capabilities() }.map(DDCCapabilities.parse) ?? [:]
        if network == nil {
            inputs = (supported[VCP.input].flatMap { $0.isEmpty ? nil : $0 } ?? [0x0F, 0x11, 0x12, 0x1B]).map(InputSource.ddc)
        }
        var found: [DDCControl] = []
        for (code, name) in DDCControl.catalog {
            if let reading = await read(code) { found.append(DDCControl(code: code, name: name, value: reading.value, max: reading.max)) }
        }
        controls = found
        await readLiveValues()
        ddc = .ready
    }

    /// Values the user may also change with the monitor's own buttons.
    private func readLiveValues() async {
        if let reading = await read(VCP.brightness) { brightness = percent(reading) }
        guard network == nil else { return }
        volume = await read(VCP.volume).map(percent)
        if let reading = await read(VCP.mute) { muted = reading.value == 1 }
        input = await read(VCP.input).map { $0.value & 0xFF }
    }

    // MARK: Network (HyperOS)

    /// Re-detects every control channel: the LAN API first, then DDC.
    func refreshControls() async {
        await connectNetwork()
        await probeDDC()
    }

    private func connectNetwork() async {
        var link = UserDefaults.standard.string(forKey: "networkHost").map(MiTVLink.init)
        if await link?.deviceName() == nil { link = await MiTVLink.discover().map(MiTVLink.init) }
        guard let link, await link.deviceName() != nil, let sources = try? await link.sources(), !sources.isEmpty else {
            network = nil
            return
        }
        UserDefaults.standard.set(link.host, forKey: "networkHost")
        network = link
        inputs = sources.sorted { $0.value < $1.value }
        await readNetworkVolume()
    }

    private func readNetworkVolume() async {
        guard let network, let value = try? await network.volume() else { return }
        networkVolume = value
        if volumeTarget == nil { volume = Double(value) }
    }

    /// The API only has ±1 volume keys, so step toward the latest slider target.
    private func chaseVolume() {
        guard let network, !chasingVolume else { return }
        chasingVolume = true
        Task {
            while let target = volumeTarget, target != networkVolume {
                let up = target > networkVolume
                guard (try? await network.press(up ? "volumeup" : "volumedown")) != nil else { break }
                networkVolume += up ? 1 : -1
            }
            volumeTarget = nil
            chasingVolume = false
            await readNetworkVolume()
        }
    }

    private func read(_ code: UInt8) async -> (value: Int, max: Int)? {
        guard let reading = await onDDC({ $0.read(code) }) else { return nil }
        maxima[code] = Swift.max(reading.max, 1)
        return reading
    }

    private func percent(_ reading: (value: Int, max: Int)) -> Double {
        Double(reading.value) * 100 / Double(Swift.max(reading.max, 1))
    }

    private func write(_ code: UInt8, percent: Double) {
        queueWrite(code, Int((percent / 100 * Double(maxima[code] ?? 100)).rounded()))
    }

    /// Coalesces slider drags: only the latest value per VCP code is sent.
    private func queueWrite(_ code: UInt8, _ value: Int) {
        pendingWrites[code] = value
        guard !flushing else { return }
        flushing = true
        Task {
            while let next = pendingWrites.popFirst() {
                _ = await onDDC { $0.write(next.key, next.value) }
            }
            flushing = false
        }
    }

    private func onDDC<T>(_ work: @escaping @Sendable (DDCChannel) -> T?) async -> T? {
        guard let channel else { return nil }
        return await withCheckedContinuation { continuation in
            ddcQueue.async { continuation.resume(returning: work(channel)) }
        }
    }

    // MARK: Display modes

    func apply(_ mode: DisplayMode) {
        notice = Display.apply(mode, to: display) ? "已切换到 \(mode.title) · \(mode.refreshRate) Hz" : "切换失败，显示器拒绝了该模式"
        refreshDisplay()
    }

    func sleepDisplay() { Display.sleepNow() }

    // MARK: Lifecycle

    func panelOpened() {
        usage = tracker.recent(7)
        Task {
            // Retry here too: the first attempt may have raced the Local Network permission prompt.
            if network == nil { await connectNetwork() } else { await readNetworkVolume() }
            if ddc == .ready { await readLiveValues() }
        }
    }

    func toggleLaunchAtLogin() {
        do {
            if launchAtLogin { try SMAppService.mainApp.unregister() } else { try SMAppService.mainApp.register() }
        } catch {
            notice = "登录项设置失败：\(error.localizedDescription)"
        }
        launchAtLogin = SMAppService.mainApp.status == .enabled
    }

    func restoreColors() { CGDisplayRestoreColorSyncSettings() }

    private func refreshDisplay() {
        display = Display.target()
        name = Display.name(of: display)
        connection = Display.connection()
        modes = Display.modes(of: display)
        currentMode = Display.currentMode(of: display)
    }

    private func screensChanged() {
        let previous = display
        refreshDisplay()
        gamma.invalidate()
        applyGamma()
        if display != previous { Task { await probeDDC() } }
    }

    private func recordUsage() {
        tracker.record(display: display)
        usage = tracker.recent(7)
    }
}
