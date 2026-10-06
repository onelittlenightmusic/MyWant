import Foundation
import FoundationModels

/// fmtool as a browser's native messaging host: the Mac's model, asked from a
/// page in Chrome.
///
/// A page on a server with no model of its own (Fly: Linux) still runs on a
/// Mac when it is open in that Mac's browser. The MyWant extension
/// (mywant-guiex webext) keeps this process open — chrome.runtime.connectNative
/// — and sends it the robot's questions; it answers them as a phone answers
/// them: the server's instructions and tools (/api/v1/fm/manifest), every call
/// run there as a step of a turn (fm_turns.go), the turn written into the
/// robot's chat with its cards. Only the model is this Mac's.
///
/// Chrome starts the host with the extension's origin as its only argument
/// ("chrome-extension://<id>/") and talks over stdin/stdout: each message is a
/// 32-bit length in the machine's byte order, then that much UTF-8 JSON.
/// Nothing else may be written to stdout — a stray line ends the connection.
///
///   {"type":"status"}                       → {"type":"status","available":true}
///   {"type":"ask","id":n,"server":url,
///    "authorization":"Basic …","question":q} → {"type":"answer","id":n,"text":…,"cards":[…]}
///   {"type":"reset","server":url}            → {"type":"reset","id":n}
///
/// One conversation per server, kept while the extension keeps the port: 「は
/// い」 to 「復元しますか？」 is answered with the question it answers.
enum NativeHost {
    static func launchedByBrowser(_ args: [String]) -> Bool {
        args.first?.hasPrefix("chrome-extension://") == true
    }

    static func run() async -> Never {
        let host = Conversations()
        while let message = readMessage() {
            write(await host.handle(message))
        }
        exit(0)
    }

    /// One message's JSON, as bytes: Data crosses into the actor, a dictionary
    /// of Any does not.
    private static func readMessage() -> Data? {
        let stdin = FileHandle.standardInput
        let header = stdin.readData(ofLength: 4)
        guard header.count == 4 else { return nil }
        let length = header.withUnsafeBytes { $0.loadUnaligned(as: UInt32.self) }
        let body = stdin.readData(ofLength: Int(length))
        guard body.count == Int(length) else { return nil }
        return body
    }

    private static func write(_ body: Data) {
        var length = UInt32(body.count)
        let header = Data(bytes: &length, count: 4)
        FileHandle.standardOutput.write(header + body)
    }
}

/// The conversations this host keeps, one per server (and the account it was
/// asked as).
private actor Conversations {
    private struct Conversation {
        let target: ServerTarget
        let box: SessionBox
        let tools: [any LocalTool]
        let tracker: CallTracker
    }

    private var conversations: [String: Conversation] = [:]

    func handle(_ data: Data) async -> Data {
        let message = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] ?? [:]
        let reply = await reply(to: message)
        return (try? JSONSerialization.data(withJSONObject: reply)) ?? Data("{}".utf8)
    }

    private func reply(to message: [String: Any]) async -> [String: Any] {
        let id = message["id"] ?? 0
        switch message["type"] as? String ?? "" {
        case "status":
            switch SystemLanguageModel.default.availability {
            case .available:
                return ["type": "status", "id": id, "available": true]
            case .unavailable(let reason):
                return ["type": "status", "id": id, "available": false, "reason": "\(reason)"]
            }
        case "reset":
            if let target = Self.target(message) { conversations[Self.key(target)] = nil }
            return ["type": "reset", "id": id]
        case "ask":
            return await ask(message, id: id)
        default:
            return ["type": "error", "id": id, "error": "unknown message type"]
        }
    }

    private func ask(_ message: [String: Any], id: Any) async -> [String: Any] {
        guard case .available = SystemLanguageModel.default.availability else {
            return ["type": "answer", "id": id, "error": "この Mac の Apple FM は使えません"]
        }
        let question = (message["question"] as? String ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !question.isEmpty, let target = Self.target(message) else {
            return ["type": "answer", "id": id, "error": "question and server are required"]
        }
        guard let conversation = await conversation(for: target) else {
            return ["type": "answer", "id": id, "error": "\(target.baseURL.absoluteString) の手順書（/api/v1/fm/manifest）を読めませんでした"]
        }

        // The turn, as a phone opens it: written into the robot's chat, so the
        // board shows the question, what the robot did and its answer.
        let turnID = await ServerTools.openTurn(question: question, target: target, by: "browser", chat: true)
        await TurnBox.shared.set(turnID)
        let answer = await servedRespond(prompt: question, box: conversation.box,
                                         tools: conversation.tools, tracker: conversation.tracker)
        await TurnBox.shared.set(nil)
        var cards: [[String: Any]] = []
        if let turnID {
            cards = await ServerTools.closeTurn(turnID, answer: answer.text, target: target)
        }
        _ = await conversation.box.finishedTurn()
        return ["type": "answer", "id": id, "text": answer.text, "cards": cards]
    }

    private func conversation(for target: ServerTarget) async -> Conversation? {
        let key = Self.key(target)
        if let kept = conversations[key] { return kept }
        guard let manifest = await ServerTools.fetch(target) else { return nil }
        let tracker = CallTracker()
        // The server's tools, and of the Mac's own only what has nothing to do
        // with this Mac's files: a page on Fly asking about them would be
        // asking about a machine it is not on.
        let tools: [any LocalTool] = manifest.tools.map {
            TrackedTool(base: ServerTool(spec: $0, target: target), tracker: tracker)
        } + [
            TrackedTool(base: GetTimeTool(), tracker: tracker),
            TrackedTool(base: CalcTool(), tracker: tracker),
        ]
        let conversation = Conversation(target: target,
                                        box: SessionBox(tools: tools.map { $0 as any Tool }, instructions: manifest.instructions),
                                        tools: tools, tracker: tracker)
        conversations[key] = conversation
        return conversation
    }

    private static func target(_ message: [String: Any]) -> ServerTarget? {
        guard let raw = message["server"] as? String, let url = URL(string: raw),
              let scheme = url.scheme, ["http", "https"].contains(scheme) else { return nil }
        let authorization = (message["authorization"] as? String).flatMap { $0.isEmpty ? nil : $0 }
        return ServerTarget(baseURL: url, authorization: authorization)
    }

    private static func key(_ target: ServerTarget) -> String {
        target.baseURL.absoluteString + "\u{0}" + (target.authorization ?? "")
    }
}
