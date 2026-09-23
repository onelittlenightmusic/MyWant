import Foundation
import FoundationModels

// MARK: - Answering a question about a picture
//
// A question about a photo — "what is the score?" of a golf score card — is
// answered here, from two things at once:
//
//   - the photo itself, which the model can look at, and
//   - the words already read out of it by the text recogniser (OCR.swift).
//
// Each is bad at what the other is good at. Looking at the photo, the model
// understands what it is of and where things are on it, and copies the words
// in it badly: it read the date, the pars and most holes of that score card
// wrong. The recogniser copies the words exactly and has no idea which of them
// matters: handed only the text, the model took the course's par for the
// player's total, and the sign over the room for the course's name.
//
// So the model looks at the photo, and the answer it gives has to be one of the
// words the recogniser read — the choice is constrained to them. It decides
// WHICH; it cannot write WHAT. Measured on that score card, the score, the
// course, the date and the par all came back right, where either input alone
// got some of them wrong.
//
// Asked on a session of its own, not the conversation's: the conversation has
// 8k tokens for everything, and a photo would crowd out what came before. The
// conversation's own turn is over by the time this runs, so the two never wait
// on each other.

private let pictureInstructions = """
    You are given a photo and the text recognized in it, line by line; cells \
    on a line are separated by |. Look at the photo to understand what it shows \
    and where things are, and pick the one recognized cell that answers the \
    question.
    """

/// The recognised cell that answers `question`, or nil when there is nothing
/// to choose from or the model could not answer.
func answerFromPicture(question: String, picture: Broker.SeenPicture) async -> String? {
    guard #available(macOS 27.0, *) else { return nil }

    var seen = Set<String>()
    let cells = picture.lines
        .flatMap { $0.components(separatedBy: " | ") }
        .map { $0.trimmingCharacters(in: .whitespaces) }
        .filter { !$0.isEmpty && seen.insert($0).inserted }
    guard !cells.isEmpty else { return nil }

    do {
        let schema = try GenerationSchema(root: DynamicGenerationSchema(name: "Cell", anyOf: cells), dependencies: [])
        let session = LanguageModelSession(instructions: pictureInstructions)
        let text = picture.lines.joined(separator: "\n")
        let url = URL(fileURLWithPath: picture.image)
        let prompt = Prompt {
            "Text recognized in the image:\n\(text)"
            Attachment(imageURL: url)
            "Question: \(question)"
        }
        let response = try await session.respond(to: prompt, schema: schema)
        let picked = try response.content.value(String.self)
        printErr("[picture] \(question) → \(picked)")
        return picked
    } catch {
        printErr("[picture] could not answer from the picture: \(error.localizedDescription)")
        return nil
    }
}
