import Foundation
import FoundationModels

/// The robot's tools and instructions, as the server hands them out.
///
/// The server keeps one set (engine/server/fm.go): what the robot can do on
/// the board, and how it is to behave. A phone's model takes it from
/// /api/v1/fm/manifest; so does this one. A tool added there is in both hands
/// the next time each starts, and the two answer the same question the same
/// way. What only a Mac has — its clock, its files, its hostname — stays here
/// as tools of this agent's own, offered beside the server's.
///
/// Every call goes back to the server (/api/v1/fm/call) and runs there, as a
/// step of the turn this agent opened for the question (see `TurnBox` and
/// serve()), so the robot's record has what it did whichever model did it.
struct ServerManifest: Decodable {
    struct ToolSpec: Decodable {
        struct Argument: Decodable {
            let name: String
            let description: String
            let required: Bool
            let choices: [String]?
        }
        let name: String
        let description: String
        let arguments: [Argument]?
    }
    let instructions: String
    let tools: [ToolSpec]
}

enum ServerTools {
    /// Where the server is: MYWANT_SERVER_URL, which the server sets when it
    /// starts this agent; otherwise the default local address.
    static var baseURL: URL {
        let named = ProcessInfo.processInfo.environment["MYWANT_SERVER_URL"] ?? ""
        return URL(string: named.isEmpty ? "http://localhost:8080" : named)!
    }

    /// The manifest, or nil when the server cannot be reached — then this
    /// agent falls back to the tools it carries itself.
    static func fetch() async -> ServerManifest? {
        do {
            let (data, _) = try await request("api/v1/fm/manifest", method: "GET", body: nil)
            return try JSONDecoder().decode(ServerManifest.self, from: data)
        } catch {
            printErr("[fmtool] no tools from the server (\(error.localizedDescription))")
            return nil
        }
    }

    static func call(tool: String, arguments: [String: String], turn: String?) async throws -> String {
        var payload: [String: Any] = ["tool": tool, "arguments": arguments]
        if let turn { payload["turn"] = turn }
        let (data, _) = try await request("api/v1/fm/call", method: "POST",
                                          body: try JSONSerialization.data(withJSONObject: payload))
        struct Reply: Decodable { let output: String }
        return try JSONDecoder().decode(Reply.self, from: data).output
    }

    /// Opens a turn for a question. Quiet: the robot want that asked already
    /// writes the question, the tool used and the answer into its chat, so the
    /// turn is only kept, not written there again.
    static func openTurn(question: String) async -> String? {
        let payload: [String: Any] = ["question": question, "by": "robot", "chat": false]
        guard let body = try? JSONSerialization.data(withJSONObject: payload),
              let (data, _) = try? await request("api/v1/fm/turns", method: "POST", body: body) else { return nil }
        struct Turn: Decodable { let id: String }
        return (try? JSONDecoder().decode(Turn.self, from: data))?.id
    }

    static func closeTurn(_ id: String, answer: String) async {
        guard let body = try? JSONSerialization.data(withJSONObject: ["answer": answer]) else { return }
        _ = try? await request("api/v1/fm/turns/\(id)/answer", method: "POST", body: body)
    }

    private static func request(_ path: String, method: String, body: Data?) async throws -> (Data, HTTPURLResponse) {
        var req = URLRequest(url: baseURL.appending(path: path))
        req.httpMethod = method
        req.timeoutInterval = 20
        if let body {
            req.httpBody = body
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let (data, resp) = try await URLSession.shared.data(for: req)
        guard let http = resp as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw NSError(domain: "fmtool", code: (resp as? HTTPURLResponse)?.statusCode ?? -1,
                          userInfo: [NSLocalizedDescriptionKey: String(data: data, encoding: .utf8) ?? ""])
        }
        return (data, http)
    }
}

/// The turn being answered now, read by every server tool the model calls.
actor TurnBox {
    static let shared = TurnBox()
    private(set) var id: String?
    func set(_ id: String?) { self.id = id }
}

/// One of the server's tools, as a tool of this agent's: same schema the phone
/// builds, run on the server.
struct ServerTool: LocalTool {
    let name: String
    let description: String
    let spec: ServerManifest.ToolSpec

    init(spec: ServerManifest.ToolSpec) {
        self.name = spec.name
        self.description = spec.description
        self.spec = spec
    }

    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(
            name: spec.name + "_args",
            properties: (spec.arguments ?? []).map { arg in
                let schema: DynamicGenerationSchema
                if let choices = arg.choices, !choices.isEmpty {
                    // An enum: guided generation cannot write anything else,
                    // which is how a want type the server does not know is
                    // ruled out.
                    schema = DynamicGenerationSchema(name: "\(spec.name)_\(arg.name)", anyOf: choices)
                } else {
                    schema = DynamicGenerationSchema(type: String.self)
                }
                return .init(name: arg.name, description: arg.description, schema: schema, isOptional: !arg.required)
            }
        )
    }

    func call(arguments: GeneratedContent) async throws -> String {
        var values: [String: String] = [:]
        for arg in spec.arguments ?? [] {
            if let v = try? arguments.value(String.self, forProperty: arg.name) { values[arg.name] = v }
        }
        do {
            return try await ServerTools.call(tool: name, arguments: values, turn: await TurnBox.shared.id)
        } catch {
            return "Error: \(error.localizedDescription)"
        }
    }
}
