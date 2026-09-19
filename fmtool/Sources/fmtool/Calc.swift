import Foundation

/// Hand-rolled four-function parser. Deliberately avoids NSExpression, which
/// evaluates FUNCTION()-style calls and can reach arbitrary selectors.
enum CalcError: LocalizedError {
    case syntax(String)
    case divideByZero

    var errorDescription: String? {
        switch self {
        case .syntax(let detail): return "syntax error: \(detail)"
        case .divideByZero: return "division by zero"
        }
    }
}

struct CalcParser {
    private let chars: [Character]
    private var pos = 0

    private init(_ expression: String) {
        self.chars = Array(expression)
    }

    static func evaluate(_ expression: String) throws -> Double {
        var parser = CalcParser(expression)
        let value = try parser.parseExpression()
        parser.skipWhitespace()
        guard parser.pos == parser.chars.count else {
            throw CalcError.syntax("unexpected trailing input at position \(parser.pos)")
        }
        return value
    }

    private mutating func skipWhitespace() {
        while pos < chars.count, chars[pos] == " " || chars[pos] == "\t" {
            pos += 1
        }
    }

    private mutating func peek() -> Character? {
        skipWhitespace()
        return pos < chars.count ? chars[pos] : nil
    }

    private mutating func parseExpression() throws -> Double {
        var value = try parseTerm()
        while let c = peek(), c == "+" || c == "-" {
            pos += 1
            let rhs = try parseTerm()
            value = c == "+" ? value + rhs : value - rhs
        }
        return value
    }

    private mutating func parseTerm() throws -> Double {
        var value = try parseFactor()
        while let c = peek(), c == "*" || c == "/" {
            pos += 1
            let rhs = try parseFactor()
            if c == "*" {
                value *= rhs
            } else {
                guard rhs != 0 else { throw CalcError.divideByZero }
                value /= rhs
            }
        }
        return value
    }

    private mutating func parseFactor() throws -> Double {
        guard let c = peek() else { throw CalcError.syntax("unexpected end of input") }
        if c == "+" {
            pos += 1
            return try parseFactor()
        }
        if c == "-" {
            pos += 1
            return -(try parseFactor())
        }
        if c == "(" {
            pos += 1
            let value = try parseExpression()
            guard peek() == ")" else { throw CalcError.syntax("expected ')'") }
            pos += 1
            return value
        }
        return try parseNumber()
    }

    private mutating func parseNumber() throws -> Double {
        skipWhitespace()
        let start = pos
        while pos < chars.count, chars[pos].isNumber || chars[pos] == "." {
            pos += 1
        }
        guard pos > start, let value = Double(String(chars[start..<pos])) else {
            throw CalcError.syntax("expected a number at position \(pos)")
        }
        return value
    }
}
