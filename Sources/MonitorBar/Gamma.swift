import CoreGraphics

/// Software dimming and warmth by scaling the display's gamma table.
/// Works over any cable, independent of DDC; macOS resets it when the app quits.
final class GammaTable {
    private var base: (red: [CGGammaValue], green: [CGGammaValue], blue: [CGGammaValue])?

    /// - Parameters: brightness 0...1 (1 = untouched), warmth 0...1 (0 = neutral).
    func apply(display: CGDirectDisplayID, brightness: Double, warmth: Double) {
        guard brightness < 0.999 || warmth > 0.001 else { CGDisplayRestoreColorSyncSettings(); return }
        let table = base ?? capture(display)
        base = table
        let gains = (red: brightness, green: brightness * (1 - 0.2 * warmth), blue: brightness * (1 - 0.55 * warmth))
        var red = table.red.map { $0 * CGGammaValue(gains.red) }
        var green = table.green.map { $0 * CGGammaValue(gains.green) }
        var blue = table.blue.map { $0 * CGGammaValue(gains.blue) }
        CGSetDisplayTransferByTable(display, UInt32(red.count), &red, &green, &blue)
    }

    /// Call after display reconfiguration so the next adjustment starts from the fresh ColorSync curve.
    func invalidate() { base = nil }

    private func capture(_ display: CGDirectDisplayID) -> (red: [CGGammaValue], green: [CGGammaValue], blue: [CGGammaValue]) {
        CGDisplayRestoreColorSyncSettings()
        let capacity = CGDisplayGammaTableCapacity(display)
        var red = [CGGammaValue](repeating: 0, count: Int(capacity))
        var green = red, blue = red
        var count: UInt32 = 0
        CGGetDisplayTransferByTable(display, capacity, &red, &green, &blue, &count)
        let n = Int(count)
        return (Array(red.prefix(n)), Array(green.prefix(n)), Array(blue.prefix(n)))
    }
}
