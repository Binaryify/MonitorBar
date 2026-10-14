import Foundation
import IOKit

// Apple Silicon exposes the display's I2C bus through these private IOKit calls.
@_silgen_name("IOAVServiceCreateWithService")
private func IOAVServiceCreateWithService(_ allocator: CFAllocator?, _ service: io_service_t) -> Unmanaged<CFTypeRef>?
@_silgen_name("IOAVServiceReadI2C")
private func IOAVServiceReadI2C(_ service: CFTypeRef, _ chip: UInt32, _ offset: UInt32, _ buffer: UnsafeMutableRawPointer, _ size: UInt32) -> IOReturn
@_silgen_name("IOAVServiceWriteI2C")
private func IOAVServiceWriteI2C(_ service: CFTypeRef, _ chip: UInt32, _ address: UInt32, _ buffer: UnsafeMutableRawPointer, _ size: UInt32) -> IOReturn

/// MCCS VCP codes used by the app.
enum VCP {
    static let brightness: UInt8 = 0x10
    static let contrast: UInt8 = 0x12
    static let redGain: UInt8 = 0x16
    static let greenGain: UInt8 = 0x18
    static let blueGain: UInt8 = 0x1A
    static let input: UInt8 = 0x60
    static let volume: UInt8 = 0x62
    static let sharpness: UInt8 = 0x87
    static let mute: UInt8 = 0x8D
}

/// DDC/CI over the external display's IOAVService. All calls block for tens of milliseconds,
/// so callers must serialize them off the main thread.
final class DDCChannel: @unchecked Sendable {
    private let service: CFTypeRef
    private init(service: CFTypeRef) { self.service = service }

    /// The first external display (this app targets a single external monitor).
    static func external() -> DDCChannel? {
        var iterator = io_iterator_t()
        guard IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("DCPAVServiceProxy"), &iterator) == KERN_SUCCESS else { return nil }
        defer { IOObjectRelease(iterator) }
        while case let entry = IOIteratorNext(iterator), entry != 0 {
            defer { IOObjectRelease(entry) }
            let location = IORegistryEntryCreateCFProperty(entry, "Location" as CFString, kCFAllocatorDefault, 0)?.takeRetainedValue() as? String
            if location == "External", let service = IOAVServiceCreateWithService(kCFAllocatorDefault, entry)?.takeRetainedValue() {
                return DDCChannel(service: service)
            }
        }
        return nil
    }

    func read(_ code: UInt8) -> (value: Int, max: Int)? {
        for _ in 0..<3 {
            var reply = [UInt8](repeating: 0, count: 11)
            guard send([0x01, code]), receive(&reply), reply[2] == 0x02, reply[4] == code else { usleep(20_000); continue }
            guard reply[3] == 0 else { return nil } // the monitor answered "unsupported VCP code"
            return (Int(reply[8]) << 8 | Int(reply[9]), Int(reply[6]) << 8 | Int(reply[7]))
        }
        return nil
    }

    @discardableResult
    func write(_ code: UInt8, _ value: Int) -> Bool {
        send([0x03, code, UInt8((value >> 8) & 0xFF), UInt8(value & 0xFF)])
    }

    /// MCCS capability string, e.g. `(prot(monitor)…vcp(10 12 60(0F 11 12))…)`.
    func capabilities() -> String? {
        var bytes = [UInt8]()
        while bytes.count < 4096 {
            var reply = [UInt8](repeating: 0, count: 38)
            let offset = bytes.count
            guard send([0xF3, UInt8(offset >> 8), UInt8(offset & 0xFF)]), receive(&reply), reply[2] == 0xE3 else { break }
            let end = 2 + Int(reply[1] & 0x7F)
            guard end >= 5 else { break }
            let chunk = reply[5..<end]
            bytes += chunk
            if chunk.isEmpty || chunk.contains(0) { break }
        }
        let text = String(decoding: bytes.prefix { $0 != 0 }, as: UTF8.self)
        return text.isEmpty ? nil : text
    }

    private func send(_ payload: [UInt8]) -> Bool {
        var packet = [0x80 | UInt8(payload.count)] + payload
        packet.append(packet.reduce(0x6E ^ 0x51, ^))
        var sent = false
        for _ in 0..<2 { // some monitors drop the first packet after idling
            usleep(10_000)
            sent = IOAVServiceWriteI2C(service, 0x37, 0x51, &packet, UInt32(packet.count)) == kIOReturnSuccess || sent
        }
        return sent
    }

    /// Reads a reply and validates its length and checksum.
    private func receive(_ reply: inout [UInt8]) -> Bool {
        usleep(50_000)
        guard IOAVServiceReadI2C(service, 0x37, 0x51, &reply, UInt32(reply.count)) == kIOReturnSuccess else { return false }
        let length = Int(reply[1] & 0x7F)
        guard length >= 1, length + 2 < reply.count else { return false }
        return reply[0..<(2 + length)].reduce(0x50, ^) == reply[2 + length]
    }
}

enum DDCCapabilities {
    /// Parses the `vcp(...)` section into supported codes and their allowed values.
    static func parse(_ text: String) -> [UInt8: [Int]] {
        guard let start = text.range(of: "vcp(", options: .caseInsensitive)?.upperBound else { return [:] }
        var result: [UInt8: [Int]] = [:]
        var depth = 1, token = "", current: UInt8?
        func flush() {
            // Tolerate strings without separators such as "101214".
            let pairs = token.count > 2 && token.count % 2 == 0
                ? stride(from: 0, to: token.count, by: 2).map { String(Array(token)[$0..<$0 + 2]) } : [token]
            for pair in pairs {
                guard let byte = UInt8(pair, radix: 16) else { continue }
                if depth == 1 { current = byte; result[byte] = [] }
                else if let current { result[current, default: []].append(Int(byte)) }
            }
            token = ""
        }
        for character in text[start...] {
            switch character {
            case "(": flush(); depth += 1
            case ")": flush(); depth -= 1; if depth == 0 { return result }
            case " ": flush()
            default: token.append(character)
            }
        }
        return result
    }
}
