import Foundation
import CoreGraphics

struct DayUsage: Codable, Identifiable {
    let day: String       // yyyy-MM-dd
    var awake = 0         // seconds the monitor was lit
    var active = 0        // seconds with keyboard/mouse input in the last 2 minutes
    var id: String { day }
}

/// Samples the monitor every `interval` seconds and keeps 60 days of totals in UserDefaults.
final class ScreenTimeTracker {
    static let interval: TimeInterval = 30
    private let key = "screenTime.v1"
    private var days: [String: DayUsage]
    private let formatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter
    }()

    init() {
        days = UserDefaults.standard.data(forKey: key).flatMap { try? JSONDecoder().decode([String: DayUsage].self, from: $0) } ?? [:]
    }

    func record(display: CGDirectDisplayID) {
        guard CGDisplayIsAsleep(display) == 0 else { return }
        let today = formatter.string(from: Date())
        var usage = days[today] ?? DayUsage(day: today)
        usage.awake += Int(Self.interval)
        let idle = CGEventSource.secondsSinceLastEventType(.combinedSessionState, eventType: CGEventType(rawValue: ~0)!)
        if idle < 120 { usage.active += Int(Self.interval) }
        days[today] = usage
        for old in days.keys.sorted().dropLast(60) { days[old] = nil }
        UserDefaults.standard.set(try? JSONEncoder().encode(days), forKey: key)
    }

    /// The last `count` days, oldest first, including days without data.
    func recent(_ count: Int) -> [DayUsage] {
        (0..<count).reversed().map { offset in
            let day = formatter.string(from: Calendar.current.date(byAdding: .day, value: -offset, to: Date())!)
            return days[day] ?? DayUsage(day: day)
        }
    }
}
