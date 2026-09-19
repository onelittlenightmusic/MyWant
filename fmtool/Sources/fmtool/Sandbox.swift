import Foundation

struct SandboxError: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}

/// Restricts tool file access to a single root directory.
struct Sandbox: Sendable {
    let root: URL

    init(root: URL) {
        self.root = root.standardizedFileURL.resolvingSymlinksInPath()
    }

    func resolve(_ path: String) throws -> URL {
        let candidate = path.hasPrefix("/")
            ? URL(fileURLWithPath: path)
            : root.appendingPathComponent(path)
        let resolved = candidate.standardizedFileURL.resolvingSymlinksInPath()
        let rootPrefix = root.path.hasSuffix("/") ? root.path : root.path + "/"
        guard resolved.path == root.path || resolved.path.hasPrefix(rootPrefix) else {
            throw SandboxError(message: "path escapes sandbox root: \(path)")
        }
        return resolved
    }
}
