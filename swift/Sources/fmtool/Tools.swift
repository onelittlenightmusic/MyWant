import Foundation
import FoundationModels

/// Counts native tool invocations so the caller can tell, after a turn
/// finishes, whether the framework's own tool-calling loop actually fired.
actor CallTracker {
    private(set) var count = 0
    private(set) var lastToolName: String?

    func record(_ name: String) {
        count += 1
        lastToolName = name
    }
}

/// A tool whose arguments and output types are fixed to `GeneratedContent`
/// and `String`. This sidesteps the `@Generable` macro (its compiler plugin
/// isn't available outside Xcode) by building the schema at runtime from
/// `DynamicGenerationSchema` instead of macro-synthesized code, and by
/// reading arguments out of `GeneratedContent` directly.
///
/// Fixing `Arguments` lets `any LocalTool` call `call(arguments:)` directly,
/// which the rescue path relies on to execute a tool outside the framework's
/// own dispatch loop.
protocol LocalTool: Tool, Sendable where Arguments == GeneratedContent, Output == String {
    var argsSchema: DynamicGenerationSchema { get }
}

extension LocalTool {
    var parameters: GenerationSchema {
        try! GenerationSchema(root: argsSchema, dependencies: [])
    }
    var includesSchemaInInstructions: Bool { true }
}

/// Wraps a `LocalTool` to record every native invocation on a shared tracker.
struct TrackedTool<Base: LocalTool>: LocalTool {
    let base: Base
    let tracker: CallTracker

    var name: String { base.name }
    var description: String { base.description }
    var argsSchema: DynamicGenerationSchema { base.argsSchema }

    func call(arguments: GeneratedContent) async throws -> String {
        await tracker.record(base.name)
        return try await base.call(arguments: arguments)
    }
}

struct GetTimeTool: LocalTool {
    let name = "get_time"
    let description = "Get the current date and time on this Mac."
    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(name: "GetTimeArgs", properties: [])
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy-MM-dd HH:mm:ss ZZZZ"
        return formatter.string(from: Date())
    }
}

struct CalcTool: LocalTool {
    let name = "calc"
    let description = "Evaluate a basic arithmetic expression using +, -, *, /, and parentheses."
    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(
            name: "CalcArgs",
            properties: [
                .init(
                    name: "expression",
                    description: "Arithmetic expression, e.g. (3 + 4) * 2",
                    schema: .init(type: String.self)
                )
            ]
        )
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let expression = try arguments.value(String.self, forProperty: "expression")
        let result = try CalcParser.evaluate(expression)
        return String(result)
    }
}

struct ListDirTool: LocalTool {
    let sandbox: Sandbox
    let name = "list_dir"
    let description = "List files and directories at a path inside the sandbox root."
    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(
            name: "ListDirArgs",
            properties: [
                .init(
                    name: "path",
                    description: "Directory path, relative to the sandbox root. Use \".\" for the root itself.",
                    schema: .init(type: String.self)
                )
            ]
        )
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let path = try arguments.value(String.self, forProperty: "path")
        let url = try sandbox.resolve(path)
        let entries = try FileManager.default.contentsOfDirectory(atPath: url.path)
        return entries.isEmpty ? "(empty directory)" : entries.sorted().joined(separator: "\n")
    }
}

struct ReadFileTool: LocalTool {
    let sandbox: Sandbox
    let name = "read_file"
    let description = "Read the text contents of a file inside the sandbox root."
    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(
            name: "ReadFileArgs",
            properties: [
                .init(
                    name: "path",
                    description: "File path, relative to the sandbox root.",
                    schema: .init(type: String.self)
                )
            ]
        )
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let path = try arguments.value(String.self, forProperty: "path")
        let url = try sandbox.resolve(path)
        let data = try Data(contentsOf: url)
        guard let text = String(data: data, encoding: .utf8) else {
            throw SandboxError(message: "file is not valid UTF-8 text: \(path)")
        }
        let limit = 4000
        return text.count > limit ? String(text.prefix(limit)) + "\n...(truncated)" : text
    }
}

struct SearchTool: LocalTool {
    let sandbox: Sandbox
    let name = "search"
    let description = "Search for a text query across files inside the sandbox root and return matching lines."
    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(
            name: "SearchArgs",
            properties: [
                .init(
                    name: "query",
                    description: "Text to search for.",
                    schema: .init(type: String.self)
                )
            ]
        )
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let query = try arguments.value(String.self, forProperty: "query")
        var matches: [String] = []
        let fm = FileManager.default
        guard let enumerator = fm.enumerator(at: sandbox.root, includingPropertiesForKeys: [.isRegularFileKey]) else {
            return "(nothing found)"
        }
        let rootPrefix = sandbox.root.path.hasSuffix("/") ? sandbox.root.path : sandbox.root.path + "/"
        outer: while let fileURL = enumerator.nextObject() as? URL {
            guard (try? fileURL.resourceValues(forKeys: [.isRegularFileKey]))?.isRegularFile == true else { continue }
            guard let text = try? String(contentsOf: fileURL, encoding: .utf8) else { continue }
            let relPath = fileURL.path.hasPrefix(rootPrefix)
                ? String(fileURL.path.dropFirst(rootPrefix.count))
                : fileURL.path
            for (i, line) in text.split(separator: "\n", omittingEmptySubsequences: false).enumerated() {
                if line.localizedCaseInsensitiveContains(query) {
                    matches.append("\(relPath):\(i + 1): \(line.trimmingCharacters(in: .whitespaces))")
                    if matches.count >= 50 { break outer }
                }
            }
        }
        return matches.isEmpty ? "(no matches)" : matches.joined(separator: "\n")
    }
}

struct HostInfoTool: LocalTool {
    let name = "host_info"
    let description = "Get basic information about this Mac: hostname, OS version, and CPU architecture."
    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(name: "HostInfoArgs", properties: [])
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let info = ProcessInfo.processInfo
        return """
        hostname: \(info.hostName)
        os: \(info.operatingSystemVersionString)
        architecture: \(Self.currentArchitecture)
        cpu_cores: \(info.processorCount)
        """
    }

    private static var currentArchitecture: String {
        #if arch(arm64)
        return "arm64"
        #elseif arch(x86_64)
        return "x86_64"
        #else
        return "unknown"
        #endif
    }
}
