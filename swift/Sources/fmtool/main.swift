import Foundation
import FoundationModels

func printErr(_ s: String) {
    FileHandle.standardError.write((s + "\n").data(using: .utf8)!)
}

let systemInstructions = """
You are a local assistant running entirely on this Mac. You have tools available \
for getting the time, doing arithmetic, listing directories, reading files, \
searching text, and reading host info. Use a tool whenever the request needs \
live data instead of guessing. Keep answers short.
"""

// MARK: - CLI arguments

var arguments = Array(CommandLine.arguments.dropFirst())
var rootPath = FileManager.default.currentDirectoryPath
var evalCount: Int?
var forceRescue = false
var promptParts: [String] = []

var argIndex = 0
while argIndex < arguments.count {
    let arg = arguments[argIndex]
    switch arg {
    case "--root":
        argIndex += 1
        guard argIndex < arguments.count else { printErr("--root requires a path"); exit(1) }
        rootPath = arguments[argIndex]
    case "--eval":
        argIndex += 1
        guard argIndex < arguments.count, let n = Int(arguments[argIndex]) else {
            printErr("--eval requires a number")
            exit(1)
        }
        evalCount = n
    case "--rescue":
        forceRescue = true
    default:
        promptParts.append(arg)
    }
    argIndex += 1
}

let availability = SystemLanguageModel.default.availability
guard case .available = availability else {
    printErr("System model unavailable: \(availability)")
    exit(1)
}

let sandbox = Sandbox(root: URL(fileURLWithPath: rootPath))

func makeTools(tracker: CallTracker) -> (localTools: [any LocalTool], tools: [any Tool]) {
    let localTools: [any LocalTool] = [
        TrackedTool(base: GetTimeTool(), tracker: tracker),
        TrackedTool(base: CalcTool(), tracker: tracker),
        TrackedTool(base: ListDirTool(sandbox: sandbox), tracker: tracker),
        TrackedTool(base: ReadFileTool(sandbox: sandbox), tracker: tracker),
        TrackedTool(base: SearchTool(sandbox: sandbox), tracker: tracker),
        TrackedTool(base: HostInfoTool(), tracker: tracker),
        TrackedTool(base: MyWantStatusTool(), tracker: tracker),
        TrackedTool(base: MyWantStartTool(), tracker: tracker),
        TrackedTool(base: MyWantWantsTool(), tracker: tracker),
        TrackedTool(base: MyWantAgentsTool(), tracker: tracker),
        TrackedTool(base: MyWantDeployTool(), tracker: tracker),
    ]
    let tools: [any Tool] = localTools.map { $0 as any Tool }
    return (localTools, tools)
}

struct RunOutcome {
    let native: Bool
    let text: String
    let toolUsed: String?
}

/// Native-first, rescue-on-miss: try the framework's own Tool dispatch, and
/// only fall back to Guided Generation if nothing fired by the time the
/// model finished responding (tracker.count stayed at 0).
func run(prompt: String, forceRescue: Bool = false) async throws -> RunOutcome {
    let tracker = CallTracker()
    let (localTools, tools) = makeTools(tracker: tracker)

    if !forceRescue {
        let session = LanguageModelSession(tools: tools, instructions: systemInstructions)
        let response = try await session.respond(to: prompt)
        if await tracker.count > 0 {
            return RunOutcome(native: true, text: response.content, toolUsed: await tracker.lastToolName)
        }
    }

    let rescueSession = LanguageModelSession(tools: tools, instructions: systemInstructions)
    let rescued = try await rescueRespond(session: rescueSession, prompt: prompt, tools: localTools)
    return RunOutcome(native: false, text: rescued.finalText, toolUsed: rescued.toolName)
}

// MARK: - Eval harness

struct EvalCase {
    let toolName: String
    let prompt: String
}

let evalCases: [EvalCase] = [
    EvalCase(toolName: "get_time", prompt: "What time is it right now on this Mac?"),
    EvalCase(toolName: "calc", prompt: "What is (17 + 5) * 3?"),
    EvalCase(toolName: "list_dir", prompt: "List the files in the current directory."),
    EvalCase(toolName: "read_file", prompt: "Read the file named eval_fixture.txt and tell me what it says."),
    EvalCase(toolName: "search", prompt: "Search the sandbox for the word \"needle\" and tell me which file it's in."),
    EvalCase(toolName: "host_info", prompt: "What is the hostname and OS version of this Mac?"),
    EvalCase(toolName: "mywant_status", prompt: "Is the MyWant server running right now?"),
    EvalCase(toolName: "mywant_wants", prompt: "List all the MyWant wants currently running."),
    EvalCase(toolName: "mywant_agents", prompt: "List all registered MyWant agent capabilities."),
    EvalCase(toolName: "mywant_deploy", prompt: "List the available MyWant recipes."),
]

func runEval(count: Int) async {
    var totalNative = 0
    var totalRescued = 0
    var totalRuns = 0

    for evalCase in evalCases {
        var native = 0
        var rescued = 0
        for _ in 0..<count {
            do {
                let outcome = try await run(prompt: evalCase.prompt)
                if outcome.toolUsed == evalCase.toolName {
                    if outcome.native { native += 1 } else { rescued += 1 }
                }
            } catch {
                printErr("[\(evalCase.toolName)] error: \(error.localizedDescription)")
            }
        }
        totalNative += native
        totalRescued += rescued
        totalRuns += count
        let hit = native + rescued
        print("\(evalCase.toolName): native \(native)/\(count), +rescued \(rescued), total \(hit)/\(count)")
    }
    let totalHit = totalNative + totalRescued
    print("---")
    print("native only:    \(totalNative)/\(totalRuns)")
    print("native+rescue:  \(totalHit)/\(totalRuns)")
}

// MARK: - Entry point

if let n = evalCount {
    await runEval(count: n)
} else {
    let prompt = promptParts.joined(separator: " ")
    guard !prompt.isEmpty else {
        printErr("usage: fmtool [--root <path>] [--eval <n>] <prompt>")
        exit(1)
    }
    do {
        let outcome = try await run(prompt: prompt, forceRescue: forceRescue)
        if let tool = outcome.toolUsed {
            printErr("[tool: \(tool), native: \(outcome.native)]")
        }
        print(outcome.text)
    } catch {
        printErr("error: \(error.localizedDescription)")
        exit(1)
    }
}
