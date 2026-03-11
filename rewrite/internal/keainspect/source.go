package keainspect

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

type SourceProperty struct {
	Name       string `json:"name"`
	NameStart  int    `json:"nameStart"`
	ValueStart int    `json:"valueStart"`
	ValueEnd   int    `json:"valueEnd"`
}

type SourceLogic struct {
	Name        string           `json:"name"`
	InputKind   string           `json:"inputKind"`
	KeaStart    int              `json:"keaStart"`
	ObjectStart int              `json:"objectStart"`
	ObjectEnd   int              `json:"objectEnd"`
	Properties  []SourceProperty `json:"properties"`
}

func FindLogics(source string) ([]SourceLogic, error) {
	var logics []SourceLogic
	for i := 0; i < len(source); i++ {
		if !matchesIdentifierAt(source, i, "kea") {
			continue
		}

		j := skipTrivia(source, i+len("kea"))
		if j < len(source) && source[j] == '<' {
			end, err := findMatching(source, j, '<', '>')
			if err != nil {
				return nil, err
			}
			j = skipTrivia(source, end+1)
		}

		if j >= len(source) || source[j] != '(' {
			continue
		}
		j = skipTrivia(source, j+1)
		var (
			inputKind  string
			objectEnd  int
			properties []SourceProperty
			err        error
		)
		switch {
		case j < len(source) && source[j] == '{':
			inputKind = "object"
			objectEnd, err = findMatching(source, j, '{', '}')
			if err != nil {
				return nil, err
			}
			properties, err = parseTopLevelProperties(source, j, objectEnd)
			if err != nil {
				return nil, err
			}
		case j < len(source) && source[j] == '[':
			inputKind = "builders"
			objectEnd, err = findMatching(source, j, '[', ']')
			if err != nil {
				return nil, err
			}
			properties, err = parseTopLevelBuilderCalls(source, j, objectEnd)
			if err != nil {
				return nil, err
			}
		default:
			continue
		}

		logics = append(logics, SourceLogic{
			Name:        guessLogicName(source, i),
			InputKind:   inputKind,
			KeaStart:    i,
			ObjectStart: j,
			ObjectEnd:   objectEnd,
			Properties:  properties,
		})
		i = objectEnd
	}
	return logics, nil
}

func parseTopLevelProperties(source string, objectStart, objectEnd int) ([]SourceProperty, error) {
	var properties []SourceProperty
	i := objectStart + 1
	for i < objectEnd {
		i = skipTrivia(source, i)
		if i >= objectEnd {
			break
		}
		if source[i] == ',' {
			i++
			continue
		}
		if source[i] == '.' {
			end, err := findPropertyEnd(source, i, objectEnd)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}
		if !isIdentifierStart(source[i]) {
			end, err := findPropertyEnd(source, i, objectEnd)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}

		nameStart := i
		for i < objectEnd && isIdentifierPart(source[i]) {
			i++
		}
		name := source[nameStart:i]
		i = skipTrivia(source, i)
		if i >= objectEnd || source[i] != ':' {
			end, err := findPropertyEnd(source, i, objectEnd)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}

		valueStart := skipTrivia(source, i+1)
		valueEnd, err := findPropertyEnd(source, valueStart, objectEnd)
		if err != nil {
			return nil, err
		}
		properties = append(properties, SourceProperty{
			Name:       name,
			NameStart:  nameStart,
			ValueStart: valueStart,
			ValueEnd:   valueEnd,
		})
		i = valueEnd
	}
	return properties, nil
}

func parseTopLevelBuilderCalls(source string, arrayStart, arrayEnd int) ([]SourceProperty, error) {
	var properties []SourceProperty
	i := arrayStart + 1
	for i < arrayEnd {
		i = skipTrivia(source, i)
		if i >= arrayEnd {
			break
		}
		if source[i] == ',' {
			i++
			continue
		}
		if !isIdentifierStart(source[i]) {
			end, err := findPropertyEnd(source, i, arrayEnd)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}

		nameStart := i
		for i < arrayEnd && isIdentifierPart(source[i]) {
			i++
		}
		name := source[nameStart:i]
		i = skipTrivia(source, i)
		if i >= arrayEnd || source[i] != '(' {
			end, err := findPropertyEnd(source, i, arrayEnd)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}

		valueStart := skipTrivia(source, i+1)
		valueEnd, err := findPropertyEnd(source, valueStart, arrayEnd)
		if err != nil {
			return nil, err
		}
		properties = append(properties, SourceProperty{
			Name:       name,
			NameStart:  nameStart,
			ValueStart: valueStart,
			ValueEnd:   valueEnd,
		})
		i = valueEnd
	}
	return properties, nil
}

func FindInspectableObjectLiteral(source string, valueStart, valueEnd int) (int, int, bool, error) {
	start := skipTrivia(source, valueStart)
	endLimit := trimExpressionEnd(source, valueEnd)
	if start >= endLimit {
		return 0, 0, false, nil
	}

	for start < endLimit && source[start] == '(' {
		end, err := findMatching(source, start, '(', ')')
		if err != nil {
			return 0, 0, false, err
		}
		if trimExpressionEnd(source, end+1) != endLimit {
			break
		}
		start = skipTrivia(source, start+1)
		endLimit = trimExpressionEnd(source, end)
	}

	if source[start] == '{' {
		end, err := findMatching(source, start, '{', '}')
		if err != nil {
			return 0, 0, false, err
		}
		if end <= endLimit {
			return start, end, true, nil
		}
	}

	arrowIndex, ok, err := findTopLevelArrow(source, start, endLimit)
	if err != nil || !ok {
		return 0, 0, false, err
	}

	bodyStart := skipTrivia(source, arrowIndex+2)
	if bodyStart >= endLimit {
		return 0, 0, false, nil
	}

	if source[bodyStart] == '(' {
		bodyStart = skipTrivia(source, bodyStart+1)
	}
	if bodyStart >= endLimit || source[bodyStart] != '{' {
		return 0, 0, false, nil
	}

	end, err := findMatching(source, bodyStart, '{', '}')
	if err != nil {
		return 0, 0, false, err
	}
	return bodyStart, end, true, nil
}

func FindInspectableArrayLiteral(source string, valueStart, valueEnd int) (int, int, bool, error) {
	start := skipTrivia(source, valueStart)
	endLimit := trimExpressionEnd(source, valueEnd)
	if start >= endLimit {
		return 0, 0, false, nil
	}

	for start < endLimit && source[start] == '(' {
		end, err := findMatching(source, start, '(', ')')
		if err != nil {
			return 0, 0, false, err
		}
		if trimExpressionEnd(source, end+1) != endLimit {
			break
		}
		start = skipTrivia(source, start+1)
		endLimit = trimExpressionEnd(source, end)
	}

	if start >= endLimit || source[start] != '[' {
		return 0, 0, false, nil
	}
	end, err := findMatching(source, start, '[', ']')
	if err != nil {
		return 0, 0, false, err
	}
	if end > endLimit {
		return 0, 0, false, nil
	}
	return start, end, true, nil
}

func FindLastTopLevelArrayElement(source string, valueStart, valueEnd int) (int, int, bool, error) {
	arrayStart, arrayEnd, ok, err := FindInspectableArrayLiteral(source, valueStart, valueEnd)
	if err != nil || !ok {
		return 0, 0, false, err
	}

	lastStart := -1
	lastEnd := -1
	i := arrayStart + 1
	for i < arrayEnd {
		i = skipTrivia(source, i)
		if i >= arrayEnd {
			break
		}
		if source[i] == ',' {
			i++
			continue
		}

		end, err := findPropertyEnd(source, i, arrayEnd)
		if err != nil {
			return 0, 0, false, err
		}
		lastStart = i
		lastEnd = trimExpressionEnd(source, end)
		i = end
	}

	if lastStart == -1 || lastEnd <= lastStart {
		return 0, 0, false, nil
	}
	return lastStart, lastEnd, true, nil
}

func FindArrowFunctionReturnProbe(source string, valueStart, valueEnd int) (int, bool, error) {
	start := skipTrivia(source, valueStart)
	endLimit := trimExpressionEnd(source, valueEnd)
	if start >= endLimit {
		return 0, false, nil
	}

	arrowIndex, ok, err := findTopLevelArrow(source, start, endLimit)
	if err != nil || !ok {
		return 0, false, err
	}

	bodyStart := skipTrivia(source, arrowIndex+2)
	if bodyStart >= endLimit {
		return 0, false, nil
	}
	if source[bodyStart] != '{' {
		return bodyStart, true, nil
	}

	blockEnd, err := findMatching(source, bodyStart, '{', '}')
	if err != nil {
		return 0, false, err
	}
	return findTopLevelReturnProbe(source, bodyStart+1, blockEnd)
}

func findTopLevelReturnProbe(source string, start, limit int) (int, bool, error) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0

	for i := start; i < limit; i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return 0, false, err
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return 0, false, err
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return 0, false, err
			}
			i = end
		case '/':
			if i+1 < limit && source[i+1] == '/' {
				i += 2
				for i < limit && source[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < limit && source[i+1] == '*' {
				i += 2
				for i+1 < limit && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= limit {
					return 0, false, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		default:
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 && matchesIdentifierAt(source, i, "return") {
				probe := skipTrivia(source, i+len("return"))
				if probe >= limit || source[probe] == ';' {
					return 0, false, nil
				}
				return probe, true, nil
			}
		}
	}

	return 0, false, nil
}

func trimExpressionEnd(source string, end int) int {
	for end > 0 {
		ch := source[end-1]
		if unicode.IsSpace(rune(ch)) || ch == ',' || ch == ';' {
			end--
			continue
		}
		break
	}
	return end
}

func findTopLevelArrow(source string, start, limit int) (int, bool, error) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	for i := start; i < limit-1; i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return 0, false, err
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return 0, false, err
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return 0, false, err
			}
			i = end
		case '/':
			if i+1 < limit && source[i+1] == '/' {
				i += 2
				for i < limit && source[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < limit && source[i+1] == '*' {
				i += 2
				for i+1 < limit && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= limit {
					return 0, false, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '=':
			if i+1 < limit && source[i+1] == '>' && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				return i, true, nil
			}
		}
	}
	return 0, false, nil
}

func findPropertyEnd(source string, start, limit int) (int, error) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	angleDepth := 0
	for i := start; i < limit; i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return 0, err
			}
			i = end
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return 0, err
			}
			i = end
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return 0, err
			}
			i = end
		case '/':
			if i+1 < limit && source[i+1] == '/' {
				i += 2
				for i < limit && source[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < limit && source[i+1] == '*' {
				i += 2
				for i+1 < limit && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= limit {
					return 0, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 && angleDepth == 0 {
				return i, nil
			}
			if braceDepth > 0 {
				braceDepth--
			}
		case '<':
			if shouldOpenAngle(source, i) {
				angleDepth++
			}
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && angleDepth == 0 {
				return i + 1, nil
			}
		}
	}
	return limit, nil
}

func findMatching(source string, start int, open, close byte) (int, error) {
	depth := 0
	for i := start; i < len(source); i++ {
		switch source[i] {
		case '\'':
			end, err := skipQuoted(source, i, '\'')
			if err != nil {
				return 0, err
			}
			i = end
			continue
		case '"':
			end, err := skipQuoted(source, i, '"')
			if err != nil {
				return 0, err
			}
			i = end
			continue
		case '`':
			end, err := skipTemplate(source, i)
			if err != nil {
				return 0, err
			}
			i = end
			continue
		case '/':
			if i+1 < len(source) && source[i+1] == '/' {
				i += 2
				for i < len(source) && source[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(source) && source[i+1] == '*' {
				i += 2
				for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
					i++
				}
				if i+1 >= len(source) {
					return 0, fmt.Errorf("unterminated block comment")
				}
				i++
				continue
			}
		}

		if source[i] == open {
			depth++
			continue
		}
		if source[i] == close {
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated %q", string(open))
}

func skipQuoted(source string, start int, quote byte) (int, error) {
	for i := start + 1; i < len(source); i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		if source[i] == quote {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unterminated quoted string")
}

func skipTemplate(source string, start int) (int, error) {
	for i := start + 1; i < len(source); i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		if source[i] == '$' && i+1 < len(source) && source[i+1] == '{' {
			end, err := findMatching(source, i+1, '{', '}')
			if err != nil {
				return 0, err
			}
			i = end
			continue
		}
		if source[i] == '`' {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unterminated template string")
}

func skipTrivia(source string, start int) int {
	for i := start; i < len(source); {
		if unicode.IsSpace(rune(source[i])) {
			i++
			start = i
			continue
		}
		if i+1 < len(source) && source[i] == '/' && source[i+1] == '/' {
			i += 2
			for i < len(source) && source[i] != '\n' {
				i++
			}
			start = i
			continue
		}
		if i+1 < len(source) && source[i] == '/' && source[i+1] == '*' {
			i += 2
			for i+1 < len(source) && !(source[i] == '*' && source[i+1] == '/') {
				i++
			}
			if i+1 < len(source) {
				i += 2
			}
			start = i
			continue
		}
		return i
	}
	return len(source)
}

func guessLogicName(source string, keaStart int) string {
	start := keaStart - 300
	if start < 0 {
		start = 0
	}
	window := source[start:keaStart]
	re := regexp.MustCompile(`(?m)(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*$`)
	matches := re.FindAllStringSubmatch(window, -1)
	if len(matches) == 0 {
		return "logic"
	}
	return matches[len(matches)-1][1]
}

func matchesIdentifierAt(source string, start int, identifier string) bool {
	end := start + len(identifier)
	if end > len(source) || source[start:end] != identifier {
		return false
	}
	if start > 0 && isIdentifierPart(source[start-1]) {
		return false
	}
	if end < len(source) && isIdentifierPart(source[end]) {
		return false
	}
	return true
}

func shouldOpenAngle(source string, index int) bool {
	prev := index - 1
	for prev >= 0 && unicode.IsSpace(rune(source[prev])) {
		prev--
	}
	if prev < 0 {
		return false
	}
	return isIdentifierPart(source[prev]) || strings.ContainsRune(")]>", rune(source[prev]))
}

func isIdentifierStart(ch byte) bool {
	return ch == '_' || ch == '$' || unicode.IsLetter(rune(ch))
}

func isIdentifierPart(ch byte) bool {
	return isIdentifierStart(ch) || (ch >= '0' && ch <= '9')
}
