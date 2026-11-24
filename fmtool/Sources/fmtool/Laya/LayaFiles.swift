// Copied from mywant-ios Common/LayaFiles.swift (44da2f3): the same Laya the apps decide
// with, so a decision is the same whichever worker takes it. Change both.

import Foundation

/// Where Laya lives on this device, and fetching it there.
///
/// The bundle is mlboydaisuke/Laya-Multilingual-CoreAI on Hugging Face (Apache
/// 2.0, Convai Innovations' weights): `macos/wfp16-s256` on a Mac — the Mac
/// Catalyst app included — and `ios/wfp16-s256` on an iPhone; the card says
/// never to load an iOS bundle on a Mac. About 650 MB, fetched once into
/// Application Support/Laya/<variant>. LAYA_BUNDLE, when set, names a folder
/// to use instead (tests, development).
public enum LayaFiles {
    static let repo = "mlboydaisuke/Laya-Multilingual-CoreAI"

    public static var variant: String {
        #if os(macOS) || targetEnvironment(macCatalyst)
        return "macos/wfp16-s256"
        #else
        return "ios/wfp16-s256"
        #endif
    }

    public static var home: URL {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? URL(fileURLWithPath: NSTemporaryDirectory())
        return base.appendingPathComponent("Laya", isDirectory: true)
    }

    public static var folder: URL {
        if let dev = ProcessInfo.processInfo.environment["LAYA_BUNDLE"], !dev.isEmpty {
            return URL(fileURLWithPath: dev)
        }
        return home.appendingPathComponent(variant, isDirectory: true)
    }

    private static var doneMark: URL { folder.appendingPathComponent(".complete") }

    /// The model, its tokenizer and metadata are all on disk.
    public static var ready: Bool {
        if ProcessInfo.processInfo.environment["LAYA_BUNDLE"] != nil {
            return FileManager.default.fileExists(atPath: folder.appendingPathComponent("metadata.json").path)
        }
        return FileManager.default.fileExists(atPath: doneMark.path)
    }

    /// Fetches the bundle (each file of the variant's folder but the
    /// reference and provenance), calling `progress` with bytes done and in
    /// all. Safe to call again: what is already whole is kept.
    public static func fetch(progress: (@Sendable (Int64, Int64) -> Void)? = nil) async throws {
        if ready { return }
        // Not over a phone's cellular data, nor in Low Data Mode.
        let config = URLSessionConfiguration.default
        config.allowsExpensiveNetworkAccess = false
        config.allowsConstrainedNetworkAccess = false
        config.timeoutIntervalForResource = 60 * 60
        let session = URLSession(configuration: config)
        struct Entry: Decodable { let type: String; let path: String; let size: Int64? }
        let list = URL(string: "https://huggingface.co/api/models/\(repo)/tree/main/\(variant)?recursive=true")!
        let (data, _) = try await session.data(from: list)
        let files = try JSONDecoder().decode([Entry].self, from: data).filter {
            $0.type == "file" && !$0.path.contains("/provenance/") && !$0.path.hasSuffix("reference.json")
        }
        let total = files.reduce(Int64(0)) { $0 + ($1.size ?? 0) }
        var done: Int64 = 0
        let fm = FileManager.default
        for file in files {
            let relative = String(file.path.dropFirst(variant.count + 1))
            let target = folder.appendingPathComponent(relative)
            try fm.createDirectory(at: target.deletingLastPathComponent(), withIntermediateDirectories: true)
            let size = file.size ?? 0
            if let have = (try? fm.attributesOfItem(atPath: target.path))?[.size] as? Int64, have == size, size > 0 {
                done += size; progress?(done, total); continue
            }
            let remote = URL(string: "https://huggingface.co/\(repo)/resolve/main/\(file.path)")!
            let (tmp, response) = try await session.download(from: remote)
            guard (response as? HTTPURLResponse)?.statusCode == 200 else {
                throw URLError(.badServerResponse)
            }
            try? fm.removeItem(at: target)
            try fm.moveItem(at: tmp, to: target)
            done += size
            progress?(done, total)
        }
        fm.createFile(atPath: doneMark.path, contents: Data())
    }
}
