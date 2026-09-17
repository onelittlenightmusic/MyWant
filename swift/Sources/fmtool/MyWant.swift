import Foundation
import FoundationModels

/// Bridges to the `mywant-skills` Python SDK scripts (`~/.mywant/custom-types/mywant-skills/<skill>/main.py`).
/// Each script takes one JSON object as argv[1] and prints one or more JSON lines to stdout —
/// progress lines first, then a final result line. We run it as a subprocess and return the
/// last JSON line, truncated the same way ReadFileTool truncates file contents: on-device
/// context is ~8k tokens, and some mywant responses (e.g. types-list) run to tens of KB.
enum MyWantScript {
    struct RunError: LocalizedError {
        let message: String
        var errorDescription: String? { message }
    }

    private static let skillsRoot = NSHomeDirectory() + "/.mywant/custom-types/mywant-skills"
    private static let outputLimit = 4000

    /// Thread-safe accumulator for a pipe's `readabilityHandler` callbacks, which fire on a
    /// background queue while the child process is still running.
    private final class DataBox: @unchecked Sendable {
        private var storage = Data()
        private let lock = NSLock()
        func append(_ chunk: Data) {
            lock.lock(); storage.append(chunk); lock.unlock()
        }
        var data: Data {
            lock.lock(); defer { lock.unlock() }
            return storage
        }
    }

    /// `DispatchWorkItem` isn't `Sendable`, so a plain lock-guarded flag stands in for
    /// "has the process already finished" to decide whether the timeout should still fire.
    private final class CancelFlag: @unchecked Sendable {
        private var cancelled = false
        private let lock = NSLock()
        func cancel() { lock.lock(); cancelled = true; lock.unlock() }
        var isCancelled: Bool { lock.lock(); defer { lock.unlock() }; return cancelled }
    }

    static func run(_ skill: String, timeout: TimeInterval, args: [String: Any]) async throws -> String {
        let scriptPath = "\(skillsRoot)/\(skill)/main.py"
        guard FileManager.default.fileExists(atPath: scriptPath) else {
            throw RunError(message: "mywant skill script not found: \(scriptPath) (is mywant-skills installed/up to date?)")
        }
        let payload = try JSONSerialization.data(withJSONObject: args)
        let jsonArg = String(data: payload, encoding: .utf8) ?? "{}"

        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/python3")
        process.arguments = [scriptPath, jsonArg]

        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        process.standardOutput = stdoutPipe
        process.standardError = stderrPipe

        let stdoutBox = DataBox()
        let stderrBox = DataBox()
        stdoutPipe.fileHandleForReading.readabilityHandler = { handle in
            let chunk = handle.availableData
            if !chunk.isEmpty { stdoutBox.append(chunk) }
        }
        stderrPipe.fileHandleForReading.readabilityHandler = { handle in
            let chunk = handle.availableData
            if !chunk.isEmpty { stderrBox.append(chunk) }
        }

        let cancelFlag = CancelFlag()
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            DispatchQueue.global().asyncAfter(deadline: .now() + timeout) {
                if !cancelFlag.isCancelled, process.isRunning { process.terminate() }
            }
            process.terminationHandler = { _ in
                cancelFlag.cancel()
                stdoutPipe.fileHandleForReading.readabilityHandler = nil
                stderrPipe.fileHandleForReading.readabilityHandler = nil
                continuation.resume()
            }
            do {
                try process.run()
            } catch {
                cancelFlag.cancel()
                continuation.resume(throwing: error)
            }
        }

        let outText = String(data: stdoutBox.data, encoding: .utf8) ?? ""
        let errText = String(data: stderrBox.data, encoding: .utf8) ?? ""
        // The script emits {"_progress": ...} lines before its final result line; only the
        // last line is the actual answer.
        let lastLine = outText
            .split(separator: "\n", omittingEmptySubsequences: true)
            .last
            .map(String.init)

        guard let lastLine, !lastLine.isEmpty else {
            if process.terminationStatus != 0 {
                throw RunError(message: errText.isEmpty
                    ? "mywant-\(skill) exited with status \(process.terminationStatus)"
                    : errText)
            }
            throw RunError(message: "mywant-\(skill) produced no output")
        }
        return lastLine.count > outputLimit
            ? String(lastLine.prefix(outputLimit)) + "\n...(truncated)"
            : lastLine
    }
}

struct MyWantStartTool: LocalTool {
    let name = "mywant_start"
    let description = "Start the local MyWant server (backend API + agent service) in the background, if it isn't already running."
    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(name: "MyWantStartArgs", properties: [])
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/opt/homebrew/bin/mywant")
        process.arguments = ["start", "-D"]
        let stdoutPipe = Pipe()
        process.standardOutput = stdoutPipe
        process.standardError = stdoutPipe
        try process.run()
        process.waitUntilExit()
        let data = stdoutPipe.fileHandleForReading.readDataToEndOfFile()
        let text = String(data: data, encoding: .utf8) ?? ""
        return text.isEmpty ? "mywant start exited with status \(process.terminationStatus)" : text
    }
}

// Wants, agents, capabilities, types, things, worlds, state, logs — everything
// the CLI can be asked to read — now come through one tool whose command list is
// read from the CLI itself. See MyWantCLI.swift; the hand-kept lists that used
// to live here went stale every time the CLI grew.

struct MyWantDeployTool: LocalTool {
    private static let actions = [
        "create", "validate", "recipes-list", "recipe-get", "recipe-create-from-want",
    ]

    let name = "mywant_deploy"
    let description = "Deploy a want from YAML, validate YAML before deploying, or manage recipes (list, inspect, or save one from an existing want)."
    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(
            name: "MyWantDeployArgs",
            properties: [
                .init(
                    name: "action",
                    description: "One of: \(Self.actions.joined(separator: ", ")).",
                    schema: DynamicGenerationSchema(name: "MyWantDeployAction", anyOf: Self.actions)
                ),
                .init(
                    name: "yaml",
                    description: "Want YAML content. Required for create and validate.",
                    schema: .init(type: String.self),
                    isOptional: true
                ),
                .init(
                    name: "name",
                    description: "Recipe name to look up or save, or a display name for a newly created want.",
                    schema: .init(type: String.self),
                    isOptional: true
                ),
                .init(
                    name: "want_id",
                    description: "Want id. Required for recipe-create-from-want.",
                    schema: .init(type: String.self),
                    isOptional: true
                ),
            ]
        )
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let action = try arguments.value(String.self, forProperty: "action")
        var payload: [String: Any] = ["action": action]
        if let yaml = try? arguments.value(String.self, forProperty: "yaml"), !yaml.isEmpty {
            payload["yaml"] = yaml
        }
        if let name = try? arguments.value(String.self, forProperty: "name"), !name.isEmpty {
            payload["name"] = name
        }
        if let wantID = try? arguments.value(String.self, forProperty: "want_id"), !wantID.isEmpty {
            payload["want_id"] = wantID
        }
        return try await MyWantScript.run("mywant-deploy", timeout: 60, args: payload)
    }
}
