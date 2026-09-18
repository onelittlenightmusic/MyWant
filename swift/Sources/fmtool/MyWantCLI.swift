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
/// Reading only, by construction: the schema offers `--safe-only` commands, the
/// ones the CLI itself marks `readOnly`. Deleting a want or opening a world is
/// not something that should follow from a question in a chat bubble, and the
/// model cannot reach them because they were never offered.
struct MyWantCommand: Decodable {
    let path: String
    let short: String?
    let use: String?
    let readOnly: Bool
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

    /// The commands this CLI has that only read, or none when there is no CLI
    /// here to ask.
    static func readOnlyCommands() -> [MyWantCommand] {
        guard let binary = binaryPath() else { return [] }
        guard let result = try? run(binary, ["commands", "--json", "--safe-only"], timeout: 20),
              result.status == 0,
              let data = result.out.data(using: .utf8),
              let commands = try? JSONDecoder().decode([MyWantCommand].self, from: data)
        else { return [] }
        // Two kinds are left out of what the model is offered, and only out of
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
        let superseded = Set(commands.map(\.path).filter { $0.hasSuffix(" point") }
            .map { $0.replacingOccurrences(of: " point", with: " where") })
        return commands.filter { $0.path != "commands" && !superseded.contains($0.path) }
    }
}

/// One tool for every command the CLI can be asked to read.
struct MyWantCLITool: LocalTool {
    let name = "mywant_cli"
    let commands: [MyWantCommand]
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
        + "Pick `command` from the list and give `args` only when the command names an id or a name — "
        + "and give the NAME alone there, never the question it was asked in: args \"新宿\", not \"新宿はどこ\". "
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
                    description: "Extra words the command needs, space-separated — usually an id. Leave empty for a plain list.",
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
            if placeholders > 1 {
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
        if result.status != 0 && argv == commandWords {
            text = "ERROR: '\(command)' needs a name in `args` — none was given, so nothing happened. "
                + "Call it again with `args` set to the name alone."
        }
        if text.isEmpty { return "mywant \(argv.joined(separator: " ")) printed nothing" }
        // The same clip ReadFileTool uses: on-device context is ~8k tokens and a
        // want list can run to tens of KB.
        return text.count > Self.outputLimit
            ? String(text.prefix(Self.outputLimit)) + "\n...(truncated)"
            : text
    }
}
