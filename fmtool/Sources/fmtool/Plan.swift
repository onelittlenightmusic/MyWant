import Foundation
import FoundationModels

/// Let the model work out the steps, instead of being told them.
///
/// The rescue path below this one picks ONE tool, fills its arguments, runs it
/// and answers — which is the whole of a simple question and half of most
/// others. "Where is transit search?" needs two: the board's names are not the
/// words the question used, so something has to be read before anything can be
/// pointed at. That procedure was written into the tool's own description for a
/// while, and writing procedures there does not scale: every question with two
/// steps in it would need its own paragraph, kept in step with a CLI that keeps
/// growing.
///
/// So the model is asked for the steps first — which tools, in what order, and
/// what each one is for — and then those steps are carried out one at a time,
/// each seeing what the ones before it found. The knowledge of how to answer
/// stays where it belongs: with the thing doing the answering.
struct PlanStep {
    let toolName: String
    let purpose: String
}

struct PlanResult {
    let steps: [PlanStep]
    let toolUsed: String?
    let finalText: String
}

/// How many steps a plan may have.
///
/// Three is enough for look-then-act-then-check, and small enough that a
/// question cannot spend a whole 8k context on planning to answer itself.
private let maxPlanSteps = 3

func planRespond(session: LanguageModelSession, prompt: String, tools: [any LocalTool]) async throws -> PlanResult {
    let toolNames = tools.map(\.name)
    let toolMenu = tools.map { "- \($0.name): \($0.description)" }.joined(separator: "\n")

    // The plan itself: up to three named tools, each with the reason it is in
    // the list. The reason is not decoration — it is what the argument-filling
    // step below is told the step is for.
    let stepSchema = DynamicGenerationSchema(
        name: "PlanStep",
        properties: [
            .init(
                name: "tool",
                description: "Which tool this step uses.",
                schema: DynamicGenerationSchema(name: "PlanStepTool", anyOf: toolNames)
            ),
            .init(
                name: "purpose",
                description: "What this step is for, in a few words.",
                schema: .init(type: String.self)
            ),
        ]
    )
    let planSchema = try GenerationSchema(
        root: DynamicGenerationSchema(
            name: "Plan",
            properties: [
                .init(
                    name: "steps",
                    description: "The tool calls that answer the request, in order. One is often enough; use more when a later step needs what an earlier one finds.",
                    schema: DynamicGenerationSchema(arrayOf: stepSchema, minimumElements: 1, maximumElements: maxPlanSteps)
                )
            ]
        ),
        dependencies: []
    )

    let planResponse = try await session.respond(
        to: """
        Available tools:
        \(toolMenu)

        Plan how to answer this request: which tools to call, in which order, and what each call is for.
        Request: \(prompt)
        """,
        schema: planSchema
    )

    var steps: [PlanStep] = []
    let stepsContent = try planResponse.content.value(forProperty: "steps") as [GeneratedContent]
    for step in stepsContent {
        let toolName = (try? step.value(String.self, forProperty: "tool")) ?? ""
        let purpose = (try? step.value(String.self, forProperty: "purpose")) ?? ""
        if !toolName.isEmpty {
            steps.append(PlanStep(toolName: toolName, purpose: purpose))
        }
    }
    guard !steps.isEmpty else {
        let answer = try await session.respond(to: prompt)
        return PlanResult(steps: [], toolUsed: nil, finalText: answer.content)
    }

    // Carry the plan out. Each step is filled in knowing what the steps before
    // it found, which is the whole reason for having more than one.
    var transcript: [String] = []
    var lastTool: String?
    for (index, step) in steps.enumerated() {
        guard let tool = tools.first(where: { $0.name == step.toolName }) else { continue }
        let sofar = transcript.isEmpty ? "" : """

            What the earlier steps found:
            \(transcript.joined(separator: "\n"))
            """
        let argsSchema = try GenerationSchema(root: tool.argsSchema, dependencies: [])
        let argsResponse = try await session.respond(
            to: """
            Step \(index + 1) of \(steps.count): call "\(tool.name)" to \(step.purpose.isEmpty ? "help answer the request" : step.purpose).
            Request: \(prompt)\(sofar)

            Provide the arguments for this call. Use the exact names the earlier steps reported, not the words of the request.
            """,
            schema: argsSchema
        )
        let output = try await tool.call(arguments: argsResponse.content)
        lastTool = tool.name
        transcript.append("\(tool.name): \(output)")
    }

    let final = try await session.respond(
        to: """
        What the steps found:
        \(transcript.joined(separator: "\n"))

        Using that, answer the original request: \(prompt)
        """
    )
    return PlanResult(steps: steps, toolUsed: lastTool, finalText: final.content)
}
