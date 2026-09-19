import Foundation
import FoundationModels

/// Guided-Generation fallback for when the native Tool protocol doesn't fire.
/// Three constrained generations, none of which the model can miss:
///   1. pick a tool name from an enum (or "no_tool")
///   2. fill that tool's own argument schema
///   3. (outside generation) run the tool locally and ask again with the result
struct RescueResult {
    let toolName: String?
    let toolOutput: String?
    let finalText: String
}

enum RescueError: LocalizedError {
    case noMatchingTool(String)

    var errorDescription: String? {
        switch self {
        case .noMatchingTool(let name):
            return "model chose an unknown tool: \(name)"
        }
    }
}

func rescueRespond(session: LanguageModelSession, prompt: String, tools: [any LocalTool]) async throws -> RescueResult {
    let toolNames = tools.map(\.name) + ["no_tool"]
    let toolMenu = tools.map { "- \($0.name): \($0.description)" }.joined(separator: "\n")
    let choiceSchema = try GenerationSchema(
        root: DynamicGenerationSchema(name: "ToolChoice", anyOf: toolNames),
        dependencies: []
    )
    let choiceResponse = try await session.respond(
        to: """
        Available tools:
        \(toolMenu)

        Pick exactly one tool by name to help answer this request, or "no_tool" if none apply.
        Request: \(prompt)
        """,
        schema: choiceSchema
    )
    let chosenName = try choiceResponse.content.value(String.self)
        .trimmingCharacters(in: .whitespacesAndNewlines)

    guard chosenName != "no_tool" else {
        let answer = try await session.respond(to: prompt)
        return RescueResult(toolName: nil, toolOutput: nil, finalText: answer.content)
    }

    guard let tool = tools.first(where: { $0.name == chosenName }) else {
        throw RescueError.noMatchingTool(chosenName)
    }

    let argsSchema = try GenerationSchema(root: tool.argsSchema, dependencies: [])
    let argsResponse = try await session.respond(
        to: "Provide arguments to call the tool \"\(tool.name)\" (\(tool.description)) to help answer: \(prompt)",
        schema: argsSchema
    )
    let output = try await tool.call(arguments: argsResponse.content)

    let final = try await session.respond(
        to: """
        The tool "\(tool.name)" returned:
        \(output)

        Using that result, answer the original request: \(prompt)
        """
    )
    return RescueResult(toolName: tool.name, toolOutput: output, finalText: final.content)
}
