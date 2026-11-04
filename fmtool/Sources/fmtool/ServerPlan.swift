import Foundation
import FoundationModels

/// A question asked of this Mac's model, answered as a plan — the server's
/// plans and acts (engine/server/plans.go, acts.go), as the iPhone and Mac
/// apps answer (mywant-ios Agent.planned):
///
///   1. /api/v1/plans/brief   what on the board may answer it, the types that
///                            could make a want for it, what may be asked of a
///                            want (its capabilities), the actions, and what
///                            was said just before (「乗り換えを再検索して」
///                            is about the route shown) — the model chooses
///   2. /api/v1/plans/params  for a want to make: its parameters, filled from
///                            what was asked
///   3. POST /api/v1/plans    the decision kept as a Plan
///   4. POST /api/v1/acts     an Act naming it: the server carries it out,
///                            writing its progress into it, and
///      GET /api/v1/acts/{n}  read back when done: what to answer from — the
///                            model words the answer
///
/// Three generations at most, each on a session of its own, none able to call
/// anything: given the board's tools to call as it pleased, a model this small
/// went round the same calls and mistook names (「look smartgolf」, two wants
/// called that). The board's tools stay for a model that works them itself
/// (Claude, through /api/v1/tools).
func serverPlanned(question: String, turnID: String?, target: ServerTarget) async throws -> String {
    let locale = ServerTools.modelLocale
    var briefReq: [String: Any] = ["question": question]
    if let locale { briefReq["locale"] = locale }
    // The turn being answered: what was said before it is in the brief.
    if let turnID { briefReq["turn"] = turnID }
    let brief = try await ServerTools.post("api/v1/plans/brief", briefReq, target: target, timeout: 30)
    let prompt = brief["prompt"] as? String ?? question
    let refs = brief["refs"] as? [String] ?? ["-"]
    let types = brief["types"] as? [String] ?? ["-"]
    let caps = brief["capabilities"] as? [String] ?? []
    // The actions to choose from, as the server offers them (want-spec's
    // PlanActions); a server from before it said them, the same three.
    let actions = brief["actions"] as? [String] ?? ["existing", "new", "none"]

    var properties: [DynamicGenerationSchema.Property] = [
        .init(name: "kind", description: "existing: show what is there; new: make a want; none: not about the board",
              schema: DynamicGenerationSchema(name: "kind", anyOf: actions)),
        .init(name: "ref", description: "For existing: the name chosen; otherwise -",
              schema: DynamicGenerationSchema(name: "ref", anyOf: refs)),
    ]
    if caps.count > 1 {
        properties.append(.init(name: "capability", description: "For existing: what to ask of it, as listed; - to just show it",
                                schema: DynamicGenerationSchema(name: "capability", anyOf: caps)))
    }
    properties.append(.init(name: "type", description: "For new: the type to make; otherwise -",
                            schema: DynamicGenerationSchema(name: "type", anyOf: types)))
    let decision = try GenerationSchema(root: DynamicGenerationSchema(name: "decision", properties: properties), dependencies: [])
    let decided = try await LanguageModelSession().respond(to: prompt, schema: decision).content
    let kind = (try? decided.value(String.self, forProperty: "kind")) ?? "none"
    let ref = (try? decided.value(String.self, forProperty: "ref")) ?? "-"
    let type = (try? decided.value(String.self, forProperty: "type")) ?? "-"
    let capability = (try? decided.value(String.self, forProperty: "capability")) ?? "-"
    printErr("[plan] \(kind) ref=\(ref) capability=\(capability) type=\(type)")

    var params: [String: String] = [:]
    if kind == "new", type != "-" {
        var req: [String: Any] = ["question": question, "type": type]
        if let locale { req["locale"] = locale }
        if let turnID { req["turn"] = turnID }
        let tb = try await ServerTools.post("api/v1/plans/params", req, target: target, timeout: 30)
        if let specs = tb["params"] as? [[String: Any]], !specs.isEmpty {
            let schema = try GenerationSchema(root: DynamicGenerationSchema(
                name: "params",
                properties: specs.map {
                    .init(name: $0["name"] as? String ?? "", description: $0["description"] as? String ?? "",
                          schema: DynamicGenerationSchema(type: String.self), isOptional: !($0["required"] as? Bool ?? false))
                }), dependencies: [])
            let filled = try await LanguageModelSession().respond(to: tb["prompt"] as? String ?? question, schema: schema).content
            for spec in specs {
                guard let name = spec["name"] as? String else { continue }
                if let v = try? filled.value(String.self, forProperty: name), !v.isEmpty { params[name] = v }
            }
        }
    }

    let plan = try await ServerTools.post("api/v1/plans", ["spec": [
        "question": question, "action": kind, "target": ref, "capability": capability, "type": type, "params": params,
    ]], target: target)
    guard let planName = (plan["metadata"] as? [String: Any])?["name"] as? String else {
        throw NSError(domain: "fmtool", code: 1, userInfo: [NSLocalizedDescriptionKey: "the server kept no plan"])
    }
    var spec: [String: Any] = ["plan": planName]
    if let turnID { spec["turn"] = turnID }
    if let locale { spec["locale"] = locale }
    let created = try await ServerTools.post("api/v1/acts", ["spec": spec], target: target)
    guard let actName = (created["metadata"] as? [String: Any])?["name"] as? String else {
        throw NSError(domain: "fmtool", code: 1, userInfo: [NSLocalizedDescriptionKey: "the server started no act"])
    }
    // Held by the server until the act is done (?wait), asked again past that.
    let deadline = Date().addingTimeInterval(120)
    while true {
        let act = try await ServerTools.get("api/v1/acts/\(actName)", query: [URLQueryItem(name: "wait", value: "60")],
                                            target: target, timeout: 75)
        let status = act["status"] as? [String: Any] ?? [:]
        switch status["phase"] as? String {
        case "Succeeded":
            return try await LanguageModelSession().respond(to: status["prompt"] as? String ?? question).content
                .trimmingCharacters(in: .whitespacesAndNewlines)
        case "Failed":
            throw NSError(domain: "fmtool", code: 1, userInfo: [NSLocalizedDescriptionKey: status["message"] as? String ?? "act failed"])
        default:
            if Date() > deadline { throw URLError(.timedOut) }
        }
    }
}
