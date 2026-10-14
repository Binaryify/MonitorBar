import AppKit
import CoreGraphics
import IOKit

struct DisplayMode: Identifiable {
    let mode: CGDisplayMode
    var width: Int { mode.width }
    var height: Int { mode.height }
    var pixelWidth: Int { mode.pixelWidth }
    var pixelHeight: Int { mode.pixelHeight }
    var refreshRate: Int { mode.refreshRate < 1 ? 60 : Int(mode.refreshRate.rounded()) }
    var isHiDPI: Bool { pixelWidth > width }
    var sizeKey: String { "\(width)x\(height)\(isHiDPI ? "@2x" : "")" }
    var id: String { "\(sizeKey)@\(refreshRate)" }
    var title: String { "\(width) × \(height)" }
}

/// Read-only facts about the target monitor plus mode switching, via CoreGraphics and IOKit.
enum Display {
    /// The first external display, falling back to the main one.
    static func target() -> CGDirectDisplayID {
        var ids = [CGDirectDisplayID](repeating: 0, count: 8)
        var count: UInt32 = 0
        CGGetActiveDisplayList(UInt32(ids.count), &ids, &count)
        return ids.prefix(Int(count)).first { CGDisplayIsBuiltin($0) == 0 } ?? CGMainDisplayID()
    }

    static func name(of display: CGDirectDisplayID) -> String {
        NSScreen.screens.first {
            ($0.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? CGDirectDisplayID) == display
        }?.localizedName ?? "显示器"
    }

    /// Cable type reported by the display port, e.g. "HDMI" or "DisplayPort".
    static func connection() -> String? {
        var iterator = io_iterator_t()
        guard IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("IOPortTransportStateDisplayPort"), &iterator) == KERN_SUCCESS else { return nil }
        defer { IOObjectRelease(iterator) }
        while case let entry = IOIteratorNext(iterator), entry != 0 {
            defer { IOObjectRelease(entry) }
            let metadata = IORegistryEntryCreateCFProperty(entry, "Metadata" as CFString, kCFAllocatorDefault, 0)?.takeRetainedValue() as? [String: Any]
            if let type = metadata?["DFP Type Description"] as? String { return type }
        }
        return nil
    }

    static func modes(of display: CGDirectDisplayID) -> [DisplayMode] {
        let options = [kCGDisplayShowDuplicateLowResolutionModes: kCFBooleanTrue] as CFDictionary
        let all = CGDisplayCopyAllDisplayModes(display, options) as? [CGDisplayMode] ?? []
        var seen = Set<String>()
        return all.filter { $0.isUsableForDesktopGUI() }.map(DisplayMode.init).filter { seen.insert($0.id).inserted }
    }

    static func currentMode(of display: CGDirectDisplayID) -> DisplayMode? {
        CGDisplayCopyDisplayMode(display).map(DisplayMode.init)
    }

    static func apply(_ mode: DisplayMode, to display: CGDirectDisplayID) -> Bool {
        var config: CGDisplayConfigRef?
        guard CGBeginDisplayConfiguration(&config) == .success else { return false }
        CGConfigureDisplayWithDisplayMode(config, display, mode.mode, nil)
        return CGCompleteDisplayConfiguration(config, .permanently) == .success
    }

    /// One mode per resolution, preferring the given refresh rate.
    static func resolutions(in modes: [DisplayMode], hiDPI: Bool, refreshRate: Int) -> [DisplayMode] {
        Dictionary(grouping: modes.filter { $0.isHiDPI == hiDPI }, by: \.sizeKey).values
            .compactMap { group in group.first { $0.refreshRate == refreshRate } ?? group.max { $0.refreshRate < $1.refreshRate } }
            .sorted { ($0.width, $0.height) > ($1.width, $1.height) }
    }

    static func sleepNow() {
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/usr/bin/pmset")
        task.arguments = ["displaysleepnow"]
        try? task.run()
    }
}
