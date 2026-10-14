import Foundation
import Network

/// LAN remote API of Xiaomi HyperOS displays and TVs (the HTTP server on port 6095),
/// used when the monitor has no DDC/CI. Discovered through its `_rc._tcp` Bonjour service.
struct MiTVLink: Sendable {
    let host: String

    struct Rejected: Error {}

    func deviceName() async -> String? {
        try? await call("request", ["action": "isalive"])["devicename"] as? String
    }

    func sources() async throws -> [InputSource] {
        let list = try await call("controller", ["action": "getsources"])["sourceList"] as? [[String: Any]] ?? []
        return list.compactMap { item in
            guard let id = item["sourceId"] as? Int else { return nil }
            return InputSource(value: id, name: item["sourceName"] as? String ?? "输入 \(id)")
        }
    }

    /// `changesource2` takes a numeric source id, so it also reaches DP and USB-C,
    /// which the name-based `changesource` action rejects.
    func select(_ sourceID: Int) async throws {
        _ = try await call("controller", ["action": "changesource2", "sourceid": "\(sourceID)"])
    }

    /// Volume on a 0...100 scale.
    func volume() async throws -> Int {
        let data = try await call("controller", ["action": "getvolume"])
        let value = data["volume"] as? Int ?? 0, max = data["maxVolume"] as? Int ?? 100
        return value * 100 / Swift.max(max, 1)
    }

    /// Remote keys such as "volumeup", "volumedown", "home", "back".
    func press(_ key: String) async throws {
        _ = try await call("controller", ["action": "keyevent", "keycode": key])
    }

    private func call(_ path: String, _ query: [String: String]) async throws -> [String: Any] {
        var components = URLComponents()
        components.scheme = "http"
        components.host = host
        components.port = 6095
        components.path = "/" + path
        components.queryItems = query.map { URLQueryItem(name: $0.key, value: $0.value) }
        guard let url = components.url else { throw Rejected() }
        let (data, _) = try await URLSession.shared.data(for: URLRequest(url: url, timeoutInterval: 4))
        guard let json = try JSONSerialization.jsonObject(with: data) as? [String: Any], json["msg"] as? String == "success" else {
            throw Rejected()
        }
        return json["data"] as? [String: Any] ?? [:]
    }

    /// IPv4 address of the first HyperOS display (preferred) or TV advertising `_rc._tcp`.
    static func discover(timeout: TimeInterval = 4) async -> String? {
        await withCheckedContinuation { continuation in
            Discovery(timeout: timeout) { continuation.resume(returning: $0) }.start()
        }
    }
}

/// One-shot Bonjour lookup that resolves the service to an IPv4 address.
/// All mutable state is touched only on `queue`.
private final class Discovery: @unchecked Sendable {
    private let queue = DispatchQueue(label: "MonitorBar.discovery")
    private let browser = NWBrowser(for: .bonjour(type: "_rc._tcp", domain: nil), using: .tcp)
    private let timeout: TimeInterval
    private var probe: NWConnection?
    private var completion: ((String?) -> Void)?

    init(timeout: TimeInterval, completion: @escaping (String?) -> Void) {
        self.timeout = timeout
        self.completion = completion
    }

    func start() {
        browser.browseResultsChangedHandler = { [self] results, _ in resolve(results) }
        browser.start(queue: queue)
        queue.asyncAfter(deadline: .now() + timeout) { [self] in finish(nil) }
    }

    private func resolve(_ results: Set<NWBrowser.Result>) {
        let isDisplay: (NWBrowser.Result) -> Bool = {
            guard case let .service(name, _, _, _) = $0.endpoint else { return false }
            return name.contains("显示器") || name.localizedCaseInsensitiveContains("monitor")
        }
        guard probe == nil, let result = results.first(where: isDisplay) ?? results.first else { return }
        let parameters = NWParameters.tcp
        (parameters.defaultProtocolStack.internetProtocol as? NWProtocolIP.Options)?.version = .v4
        let connection = NWConnection(to: result.endpoint, using: parameters)
        connection.stateUpdateHandler = { [self] state in
            // Format the raw bytes: the endpoint's description carries an interface suffix like "%en1".
            if case .ready = state, case let .hostPort(.ipv4(address), _)? = connection.currentPath?.remoteEndpoint {
                finish(address.rawValue.map(String.init).joined(separator: "."))
            }
        }
        probe = connection
        connection.start(queue: queue)
    }

    private func finish(_ host: String?) {
        guard let completion else { return }
        self.completion = nil
        browser.cancel()
        probe?.cancel()
        completion(host)
    }
}
