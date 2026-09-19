import Foundation
import FoundationModels

/// The commands that cannot be taken back, behind a person saying yes.
///
/// Everything else the robot does to a board can be undone — a tile goes back
/// where it was, a want that was just made is deleted again (`mywant undo`).
/// Deleting a want is not in that class: it is gone, with whatever it was
/// keeping. Between a model that mishears a name and a board somebody has been
/// building for months, there has to be a person.
///
/// So these live in their own tool, and the tool refuses to run until two
/// separate things are true: the model says the person agreed, AND the person's
/// own last message reads as agreement. The first alone is a model marking its
/// own homework — it is the same model that decided to call this. The second
/// alone would let any "はい" in a conversation fire whatever was pending. Both
/// together mean the words were asked for and then said.

/// Whether the person's most recent message was a yes.
///
/// Set from the incoming request (see Serve.swift), not from anything the model
/// produced: it is the one signal in the conversation the model does not write.
actor ConsentGate {
    private var lastMessageWasConsent = false
    /// What was offered and refused, so that the yes has something to attach
    /// to.
    ///
    /// "delete-me-test という want を削除して" / "はい" is the whole exchange, and
    /// the yes carries no name — the model, asked to fill the arguments again a
    /// turn later, sent the command with nothing to act on and the CLI refused
    /// it. What the person agreed to is what they were told, so it is kept here
    /// rather than asked for twice.
    private var pending: (command: String, args: String)?

    func note(prompt: String) {
        lastMessageWasConsent = ConsentGate.looksLikeConsent(prompt)
    }

    func consented() -> Bool { lastMessageWasConsent }

    func remember(command: String, args: String) {
        pending = (command, args)
    }

    /// The arguments last offered for this command, if it is the one waiting.
    func pendingArgs(for command: String) -> String? {
        guard let pending, pending.command == command, !pending.args.isEmpty else { return nil }
        return pending.args
    }

    func clearPending() { pending = nil }

    /// The command a person is being asked about, as they would read it, or ""
    /// when nothing is waiting.
    ///
    /// Read by the serving loop after each turn so the asker's screen can put
    /// the question as a question — a sentence and two buttons — instead of
    /// leaving it buried in what the robot said.
    func pendingSentence() -> String {
        guard let pending else { return "" }
        return "mywant " + pending.command + (pending.args.isEmpty ? "" : " " + pending.args)
    }

    /// A short message that is agreement and little else.
    ///
    /// Short on purpose: "はい" is consent, and "はい、でも先に天気を見せて" is a
    /// new request that happens to start with one. A yes buried in a paragraph
    /// is not the yes this asks for.
    static func looksLikeConsent(_ prompt: String) -> Bool {
        let text = prompt
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
            // The canvas context the server appends is not part of what was
            // said; it would make every message too long to count.
            .components(separatedBy: "\n").first?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard text.count <= 24 else { return false }
        let yeses = ["はい", "うん", "ok", "okay", "yes", "y", "どうぞ", "いいよ", "お願い",
                     "おねがい", "やって", "消して", "削除して", "確認", "そうして", "go ahead", "do it"]
        return yeses.contains { text == $0 || text.hasPrefix($0) }
    }
}

/// One tool for the commands that take something away.
struct MyWantDestructiveTool: LocalTool {
    let name = "mywant_delete"
    let commands: [MyWantCommand]
    let gate: ConsentGate
    private let binary: String
    private static let outputLimit = 2000

    init?(commands: [MyWantCommand], gate: ConsentGate) {
        guard let binary = MyWantCLI.binaryPath(), !commands.isEmpty else { return nil }
        self.binary = binary
        self.commands = commands
        self.gate = gate
    }

    var description: String {
        "Commands that DELETE or REPLACE something and cannot be undone: deleting a want or a thing, "
        + "clearing state, switching or importing a world, stopping or suspending a want. "
        + "Never call this to answer a question — only when the person asked for the thing to be removed. "
        + "Call it FIRST with confirmed=false: nothing runs, and you are told exactly what would. "
        + "Tell the person that sentence and ask them to say yes. Call again with confirmed=true only "
        + "after they have said yes in their own message."
    }

    var argsSchema: DynamicGenerationSchema {
        DynamicGenerationSchema(
            name: "MyWantDeleteArgs",
            properties: [
                .init(
                    name: "command",
                    description: "Which one. \(commands.prefix(12).map(\.path).joined(separator: ", "))",
                    schema: DynamicGenerationSchema(name: "MyWantDeleteCommand", anyOf: commands.map(\.path))
                ),
                // Required, not optional. Left optional, the model filled in
                // `confirmed` and skipped this, and every call arrived as
                // "delete something" with no something — a deletion nobody
                // could confirm because nobody could be told what it was.
                .init(
                    name: "args",
                    description: "The name or id to act on, alone — never the sentence it was asked in. Always fill this in.",
                    schema: .init(type: String.self)
                ),
                .init(
                    name: "confirmed",
                    description: "True ONLY when the person has already said yes to this exact command in their own last message.",
                    schema: .init(type: Bool.self)
                ),
            ]
        )
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let command = try arguments.value(String.self, forProperty: "command")
        guard commands.contains(where: { $0.path == command }) else {
            return "Not something mywant can delete: \(command)"
        }
        var argv = command.split(separator: " ").map(String.init)
        var extra = (try? arguments.value(String.self, forProperty: "args")) ?? ""
        // A yes is an answer to what was offered, so the offer's own arguments
        // stand in when the confirming call arrives without them.
        if extra.isEmpty, let remembered = await gate.pendingArgs(for: command) {
            extra = remembered
        }
        if !extra.isEmpty { argv.append(extra) }
        let sentence = "mywant " + argv.joined(separator: " ")

        // Nothing named, nothing to offer: asked to confirm "delete a want",
        // the person has no way to know which, and neither has this.
        let namesSomething = (commands.first { $0.path == command }?.use ?? "").contains("<")
        if extra.isEmpty && namesSomething {
            return "NOT DONE — '\(command)' has to be told which one, and `args` was empty. "
                + "Call it again with `args` set to the exact name from the board."
        }

        let claimed = (try? arguments.value(Bool.self, forProperty: "confirmed")) ?? false
        let consented = await gate.consented()
        if !claimed || !consented {
            // Both halves are named, because the two are told apart by what
            // happens next: a model that never asked should ask, and a model
            // that asked and got no answer should wait rather than ask again.
            await gate.remember(command: command, args: extra)
            FileHandle.standardError.write("[mywant WOULD RUN \(sentence) — waiting for a yes]\n".data(using: .utf8)!)
            return "NOT DONE — nothing was run. This would run `\(sentence)`, and it cannot be undone. "
                + "Say exactly that to the person, in their language, and ask them to answer yes or no. "
                + (claimed && !consented
                   ? "They have not said yes yet in their own message."
                   : "Then call this again with confirmed=true.")
        }

        await gate.clearPending()
        FileHandle.standardError.write("[\(sentence)]\n".data(using: .utf8)!)
        let result = try MyWantCLI.run(binary, argv)
        let text = result.status == 0
            ? "Done: \(sentence)\n" + result.out.trimmingCharacters(in: .whitespacesAndNewlines)
            : "ERROR: \(sentence) failed: " + result.err.trimmingCharacters(in: .whitespacesAndNewlines)
        return text.count > Self.outputLimit
            ? String(text.prefix(Self.outputLimit)) + "\n...(truncated)"
            : text
    }
}
