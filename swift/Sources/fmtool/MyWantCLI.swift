import Foundation
import FoundationModels

/// The whole MyWant CLI as one tool, described by the CLI itself.
///
/// The tools beside this one name their actions in Swift: a list of strings
/// compiled into the binary, which is a copy of the CLI kept by hand. Every such
/// copy goes stale — the CLI grew things, worlds, state and logs while this
/// agent could still only ask about wants and agents.
///
/// So the command list is not written here. At startup `mywant commands --json`
/// is asked what this CLI can do, and the answer becomes the tool's own schema:
/// the model picks from the commands the binary actually has. A command added to
/// the CLI is available the next time fmtool starts, with nothing to change here.
///
/// What is offered is decided by the CLI too. Every command says what it costs
/// if it was the wrong one — "read", "change" or "destroy" — and the three are
/// treated differently rather than the whole write half being shut off:
///
///   read     always offered; a question cannot break anything.
///   change   offered, for the board: moving a tile, pinning a thing, making a
///            want. A misheard name here costs a shrug and `mywant undo`.
///   destroy  a separate tool that refuses to run until the person has said yes
///            (see Destructive.swift). A deleted want is not coming back, and
///            nothing in a chat bubble should be able to reach one by accident.
struct MyWantCommand: Decodable {
    let path: String
    let short: String?
    let use: String?
    let readOnly: Bool
    let risk: String?
}

enum MyWantCLI {
    /// Where the CLI is: MYWANT_BIN, then PATH, then ~/.local/bin, which is
    /// where `make install` puts it.
    static func binaryPath() -> String? {
        let fm = FileManager.default
        if let named = ProcessInfo.processInfo.environment["MYWANT_BIN"], !named.isEmpty,
           fm.isExecutableFile(atPath: named) {
            return named
        }
        for dir in ["/opt/homebrew/bin", "/usr/local/bin"] {
            let candidate = dir + "/mywant"
            if fm.isExecutableFile(atPath: candidate) { return candidate }
        }
        let home = NSHomeDirectory() + "/.local/bin/mywant"
        if fm.isExecutableFile(atPath: home) { return home }
        return nil
    }

    /// Run the CLI and hand back what it printed.
    static func run(_ binary: String, _ args: [String], timeout: TimeInterval = 60) throws -> (out: String, err: String, status: Int32) {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: binary)
        process.arguments = args
        let outPipe = Pipe(), errPipe = Pipe()
        process.standardOutput = outPipe
        process.standardError = errPipe
        try process.run()

        // Read before waiting: a command with more output than a pipe buffer
        // holds would otherwise block forever with the parent waiting on exit.
        let outData = outPipe.fileHandleForReading.readDataToEndOfFile()
        let errData = errPipe.fileHandleForReading.readDataToEndOfFile()
        process.waitUntilExit()
        return (
            String(data: outData, encoding: .utf8) ?? "",
            String(data: errData, encoding: .utf8) ?? "",
            process.terminationStatus
        )
    }

    /// Everything the CLI says it can do, or nothing when there is no CLI here
    /// to ask. Plugins included: the core CLI collects them (`gui tile set` is
    /// how a tile moves, and it lives in another binary entirely).
    static func allCommands() -> [MyWantCommand] {
        guard let binary = binaryPath() else { return [] }
        guard let result = try? run(binary, ["commands", "--json"], timeout: 20),
              result.status == 0,
              let data = result.out.data(using: .utf8),
              let commands = try? JSONDecoder().decode([MyWantCommand].self, from: data)
        else { return [] }
        return commands
    }

    /// The groups a guide to a board has business in.
    ///
    /// Read commands are offered whatever group they are in — asking is free.
    /// Writing is not, and this CLI can also install plugins, rewrite config and
    /// create want types, none of which is canvas work and all of which a small
    /// model would sometimes pick when it meant something else. So the writing
    /// half is narrowed to what the board is made of.
    static let boardGroups: Set<String> = ["wants", "thing", "world", "state", "gui", "undo"]

    /// Starting and stopping the GUI server is not arranging a canvas; it is
    /// turning off the screen the canvas is on.
    static let neverOffered: Set<String> = ["gui start", "gui stop"]

    /// What to offer the model: the ones it may run, and the ones it must ask
    /// about first.
    static func offered(writes: Bool) -> (safe: [MyWantCommand], dangerous: [MyWantCommand]) {
        let commands = allCommands()
        let risk = { (c: MyWantCommand) in c.risk ?? (c.readOnly ? "read" : "change") }
        let inBoard = { (c: MyWantCommand) in
            MyWantCLI.boardGroups.contains(c.path.split(separator: " ").first.map(String.init) ?? "")
                && !MyWantCLI.neverOffered.contains(c.path)
        }
        var safe = commands.filter { risk($0) == "read" || (writes && risk($0) == "change" && inBoard($0)) }
        let dangerous = writes ? commands.filter { risk($0) == "destroy" && inBoard($0) } : []
        safe = trimmed(safe)
        return (safe, dangerous)
    }

    /// Two kinds are left out of what the model is offered, and only out of
        // THAT — both stay in the CLI for people and scripts.
        //
        //   commands   lists what the CLI can do, which this tool's description
        //              already carries; offering it invites a call that answers
        //              nobody's question, and it did.
        //   … where    is the half of an answer its `point` sibling gives whole:
        //              the same cell, without going to show it. Offered both,
        //              the model picked `where` about half the time and the
        //              asker got coordinates and a robot standing where it was.
        //              A guide shows; anybody who wants the quiet form can run
        //              it themselves.
    static func trimmed(_ commands: [MyWantCommand]) -> [MyWantCommand] {
        let superseded = Set(commands.map(\.path).filter { $0.hasSuffix(" point") }
            .map { $0.replacingOccurrences(of: " point", with: " where") })
        return commands.filter { $0.path != "commands" && !superseded.contains($0.path) }
    }
}

/// What the board calls something, if it calls anything that.
enum BoardName {
    case exact(String)
    /// The same name, spelled the way the board spells it.
    case corrected(String)
    /// Nothing close enough, with whatever was nearest for the asking.
    case unknown([String])
}

extension MyWantCLI {
    /// Matches a name against everything standing on the board.
    ///
    /// Exactly first, then ignoring case — "Nakanoのweather" and
    /// "NakanoのWeather" are the same want and only one of them exists — then
    /// by containment, which is what turns "Nakano" into a list to choose
    /// from rather than a silent miss.
    static func boardName(matching name: String) -> BoardName {
        let wanted = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !wanted.isEmpty, let binary = binaryPath(),
              let result = try? run(binary, ["board", "--json"], timeout: 20),
              result.status == 0,
              let data = result.out.data(using: .utf8),
              let entries = try? JSONDecoder().decode([BoardEntry].self, from: data)
        else { return .exact(name) } // no board to check against: let the CLI answer

        let names = entries.map(\.name)
        if names.contains(wanted) { return .exact(wanted) }
        if let same = names.first(where: { $0.lowercased() == wanted.lowercased() }) {
            return .corrected(same)
        }
        let near = names.filter {
            $0.lowercased().contains(wanted.lowercased()) || wanted.lowercased().contains($0.lowercased())
        }
        if near.count == 1 { return .corrected(near[0]) }
        return .unknown(Array(near.prefix(5)))
    }
}

private struct BoardEntry: Decodable {
    let name: String
}

/// One tool for every command the CLI can be asked to read.
struct MyWantCLITool: LocalTool {
    let name = "mywant_cli"
    let commands: [MyWantCommand]
    /// Whether the offered list includes commands that change the board, which
    /// decides whether the description bothers to say what they are.
    var canWrite: Bool { commands.contains { ($0.risk ?? "read") == "change" } }
    private let binary: String
    private static let outputLimit = 4000

    init?(commands: [MyWantCommand]) {
        guard let binary = MyWantCLI.binaryPath(), !commands.isEmpty else { return nil }
        self.binary = binary
        self.commands = commands
    }

    var description: String {
        "THE tool for any question about MyWant: wants, things (named values), the canvas/board, worlds, "
        + "state, agents, recipes, logs, server status. Use it — never the file search — whenever MyWant, "
        + "a want, a thing or the board is mentioned. "
        + "Pick `command` from the list and give `args` everything that command needs and nothing else — "
        + "a name alone for a question about one thing (args \"新宿\", never \"新宿はどこ\"), and a name "
        + "followed by the numbers when the command places something (args \"新宿 5 0\"). Never the sentence "
        + "it was asked in. "
        // The confusions worth naming, each one seen: a question about the
        // board answered from the filesystem, and processes counted as wants.
        // What each command IS, not what order to call them in: the order is the
        // model's to work out (see Plan.swift), and procedures written here go
        // stale as fast as the CLI grows.
        + "'board' names everything on the canvas, spelled as 'point' expects. 'point' takes one name and "
        + "says where it is AND walks the robot there, so the asker can see it. "
        // The board is a graph, not a pile: this is the edge between two of its
        // tiles, and the question "what is X connected to" has one answer that
        // knows about both kinds of edge.
        + "Things and wants are CONNECTED to each other: a want reads the things it names and the fields of "
        + "other wants, and feeds its own fields on. 'relations' takes one name and lists those connections "
        + "in both directions — use it for 'what is X connected to', 'what feeds X', 'what uses X'. "
        + "A want is something on the board — 'wants list'. 'ps' is the server's own processes, not wants. "
        + "Named values are things — 'thing list'."
        // What the writing verbs ARE. The model chooses from bare command
        // paths — the descriptions the CLI carries never reach the schema — so
        // "Parasomniaを(9,-9)に移動して" met a list in which nothing said "move"
        // and picked 'point', which walked over and reported the old cell.
        // Still no procedures: which of these to call, and in what order, is
        // the model's to work out.
        + (canWrite
           ? " Asked to take back, revert or undo what was just done (元に戻す), call 'undo' with no args — "
             + "never work out the reverse yourself, it is recorded. "
             + "To PLACE or MOVE a thing: 'thing pin' with args \"<name> <x> <y>\"; to take it off the board: "
             + "'thing unpin'. To move a want's tile: 'gui tile set' with args \"<name> <x> <y>\". "
             + "To make a want: 'wants create' with args \"--type <type> --at <x>,<y>\". "
             + "To take back the last change: 'undo', with no args."
           : "")
    }

    var argsSchema: DynamicGenerationSchema {
        // The catalogue the model chooses from IS the CLI's own, read at
        // startup — see the note at the top of this file.
        let paths = commands.map(\.path)
        // Short on purpose. Every word here rides in the prompt of every
        // request, and this model has 8k tokens for the whole conversation —
        // a full catalogue with its descriptions left so little room that a
        // four-word question could overflow the window mid-turn. The paths
        // alone say most of it; `commands --json` has the rest for anyone who
        // needs it.
        let summary = commands
            .prefix(24)
            .map(\.path)
            .joined(separator: ", ")
        return DynamicGenerationSchema(
            name: "MyWantCLIArgs",
            properties: [
                .init(
                    name: "command",
                    description: "The command to run. \(summary)",
                    schema: DynamicGenerationSchema(name: "MyWantCLICommand", anyOf: paths)
                ),
                .init(
                    name: "args",
                    description: "Everything the command needs, space-separated and in order: a name, or a name then numbers. Leave empty for a plain list.",
                    schema: .init(type: String.self),
                    isOptional: true
                ),
            ]
        )
    }

    func call(arguments: GeneratedContent) async throws -> String {
        let command = try arguments.value(String.self, forProperty: "command")
        // Only what was offered: the model is asked to choose from the read-only
        // list, and a command that is not on it does not run.
        guard commands.contains(where: { $0.path == command }) else {
            return "mywant cannot do that, or it changes something: \(command)"
        }
        var argv = command.split(separator: " ").map(String.init)
        if let extra = try? arguments.value(String.self, forProperty: "args"), !extra.isEmpty {
            // One argument, unless the command's usage line asks for more.
            //
            // Split on spaces, "transit search" reached a command that takes
            // exactly one name as two of them, and the CLI refused it — for a
            // want whose tile was on the board the whole time. A name with a
            // space in it is still one name; only a command whose usage names
            // two placeholders gets the words handed over separately.
            let placeholders = (commands.first { $0.path == command }?.use ?? "")
                .filter { $0 == "<" || $0 == "[" }
                .count
            // Flags are words of their own however the usage line reads:
            // `wants create` takes no placeholders and everything it needs is
            // flags, so handing it "--type button --at 3,4" as one argument
            // gave the CLI one very long type name.
            let hasFlags = extra.hasPrefix("-") || extra.contains(" -")
            if placeholders > 1 || hasFlags {
                argv.append(contentsOf: extra.split(separator: " ").map(String.init))
            } else {
                argv.append(extra)
            }
        }
        // What was actually run, on stderr beside the "[tool: …]" line. Without
        // it a wrong answer is a mystery: the tool fired, and nothing says
        // whether the model asked for the wrong command or passed the whole
        // question where a name belonged.
        FileHandle.standardError.write(("[mywant " + argv.joined(separator: " ") + "]\n").data(using: .utf8)!)
        let result = try MyWantCLI.run(binary, argv)
        let commandWords = command.split(separator: " ").map(String.init)
        var text = result.status == 0
            ? result.out.trimmingCharacters(in: .whitespacesAndNewlines)
            : "ERROR: mywant \(argv.joined(separator: " ")) failed: "
              + result.err.trimmingCharacters(in: .whitespacesAndNewlines)
        // The failure worth naming, because it is the one the model makes and
        // then reports as a success: a command that names something, called
        // with nothing to name. Said plainly and first, so it is not lost in a
        // page of usage text.
        // The failure worth naming, because it is the one the model makes and
        // then reports as a success: a command called with less than it names.
        // The usage line says what it wanted, so the correction is exact rather
        // than an invitation to try a different command — which is what
        // happened when it only said "failed": three commands in a row, none of
        // them given a cell.
        if result.status != 0 {
            let usage = commands.first { $0.path == command }?.use ?? ""
            let wanted = usage.split(separator: " ").dropFirst().joined(separator: " ")
            let given = argv.count > commandWords.count
                ? argv.suffix(from: commandWords.count).joined(separator: " ")
                : ""
            if !wanted.isEmpty {
                text = "ERROR: '\(command)' takes \(wanted) — "
                    + (given.isEmpty ? "nothing was given" : "you gave \"\(given)\"")
                    + ", so nothing happened. Call '\(command)' again with `args` holding all of it, "
                    + "space-separated, and nothing else."
            }
        }
        if text.isEmpty { return "mywant \(argv.joined(separator: " ")) printed nothing" }
        // The same clip ReadFileTool uses: on-device context is ~8k tokens and a
        // want list can run to tens of KB.
        return text.count > Self.outputLimit
            ? String(text.prefix(Self.outputLimit)) + "\n...(truncated)"
            : text
    }
}
