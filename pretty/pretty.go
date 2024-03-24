package pretty

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mathpn/listme/blame"

	"github.com/charmbracelet/lipgloss"
)

// Style used to print to stdout
type Style int

const (
	FullStyle Style = iota
	BWStyle
	PlainStyle
)
const boldCode = "\x1b[1m"
const resetBold = "\x1b[22m"

// Styles
var baseStyle = lipgloss.NewStyle()
var boldStyle = baseStyle.Copy().Bold(true)
var filenameColorStyle = boldStyle.Copy().Foreground(lipgloss.Color("#0087d7"))
var borderStyle = baseStyle.Copy().Border(lipgloss.RoundedBorder()).MarginLeft(2)
var oldCommitStyle = boldStyle.Copy().Foreground(lipgloss.Color("#dadada")).Background(lipgloss.Color("#d70000"))
var todoStyle = baseStyle.Copy().Foreground(lipgloss.Color("#5fafaf"))
var xxxStyle = baseStyle.Copy().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#d7af00"))
var fixmeStyle = baseStyle.Copy().Foreground(lipgloss.Color("#ff0000"))
var optimizeStyle = baseStyle.Copy().Foreground(lipgloss.Color("#d75f00"))
var bugStyle = baseStyle.Copy().Foreground(lipgloss.Color("#eeeeee")).Background(lipgloss.Color("#870000"))
var noteStyle = baseStyle.Copy().Foreground(lipgloss.Color("#87af87"))
var hackStyle = baseStyle.Copy().Foreground(lipgloss.Color("#d7d700"))

// Bold returns the provided string with bold style
func Bold(str string) string {
	return boldCode + str + resetBold
}

// PrettyLineNumber returns a string with the format
//
//	[Line 123]
//
// The string is padded according to the maximum number of digits (maxDigits)
// to ensure vertical alignment between line numbers of the same file.
func PrettyLineNumber(number int, maxDigits int) string {
	strNumber := fmt.Sprint(number)
	pad := strings.Repeat(" ", maxDigits-len(strNumber))
	return fmt.Sprintf("  [Line %s%d] ", pad, number)
}

// PrettyFilename returns a string with the format
//
//   - tests/generic_code.py (10 comments)
//
// The string if formatted according to the provided style (colorful or black-and-white).
func PrettyFilename(path string, nComments int, style Style) string {
	var styler lipgloss.Style
	switch style {
	case BWStyle:
		styler = boldStyle
	case FullStyle:
		styler = filenameColorStyle
	default:
		styler = baseStyle
	}
	fname := styler.Render(fmt.Sprintf("• %s", path))
	var comments string
	if nComments > 1 {
		comments = fmt.Sprintf("(%d comments)", nComments)
	} else {
		comments = fmt.Sprintf("(%d comment)", nComments)
	}
	return fname + " " + comments
}

// Emojify prepends the tag string with an emoji
func Emojify(tag string) string {
	switch tag {
	case "TODO":
		return "✓ TODO"
	case "XXX":
		return "✘ XXX"
	case "FIXME":
		return "⚠ FIXME"
	case "OPTIMIZE":
		return " OPTIMIZE"
	case "BUG":
		return "☢ BUG"
	case "NOTE":
		return "✐ NOTE"
	case "HACK":
		return "✄ HACK"
	default:
		return "⚠ " + tag
	}
}

// Colorize colorizes the provided text according to the tag and style.
// If style != FullStyle, this function does nothing.
func Colorize(text string, tag string, style Style) string {
	if style != FullStyle {
		return text
	}
	switch tag {
	case "TODO":
		return todoStyle.Render(text)
	case "XXX":
		return xxxStyle.Render(text)
	case "FIXME":
		return fixmeStyle.Render(text)
	case "OPTIMIZE":
		return optimizeStyle.Render(text)
	case "BUG":
		return bugStyle.Render(text)
	case "NOTE":
		return noteStyle.Render(text)
	case "HACK":
		return hackStyle.Render(text)
	default:
		return text
	}
}

// PrettyBlame returns a string with the format
//
//	[John Doe]
//
// If the commit is older than ageLimit (in days), the format is
//
//	[OLD John Doe]
//
// Color is added according to the style.
func PrettyBlame(blame *blame.LineBlame, ageLimit int, style Style) string {
	blameStr := fmt.Sprintf("[%s]", blame.Author)
	if blame.Time.IsZero() {
		return blameStr
	}
	// TODO remove timestamp logic from this module
	currentDate := time.Now()

	diff := currentDate.Sub(blame.Time)
	maxAge := time.Duration(ageLimit) * 24 * time.Hour
	if diff > maxAge {
		blameStr = fmt.Sprintf("[OLD %s]", blame.Author)
		if style == FullStyle {
			blameStr = oldCommitStyle.Render(blameStr)
		}
	}
	return blameStr
}

func PrettySummary(counter map[string]int, style Style) string {
	tags := make([]string, 0, len(counter))
	for tag := range counter {
		tags = append(tags, tag)
	}

	sort.Strings(tags)
	boxStr := " "
	for _, tag := range tags {
		tagStr := fmt.Sprintf(" %s %d ", Emojify(tag), counter[tag])
		if style == FullStyle {
			tagStr = Colorize(tagStr, tag, style)
		}
		boxStr += tagStr
	}
	return borderStyle.Render(boxStr + " ")
}

// GetStyle returns the style that should be used. FullStyle is the default.
// If bw, then BWStyle. If plain, then PlainStyle.
//
// If the output (stdout) is redirected, PlainStyle is always used.
func GetStyle(bw bool, plain bool) (Style, error) {
	if bw && plain {
		return -1, fmt.Errorf("only one style can be specified")
	}

	fi, err := os.Stdout.Stat()
	if err != nil {
		err = fmt.Errorf("error while read stdout info: %s", err)
		return PlainStyle, err
	}

	var style Style
	switch {
	case (fi.Mode() & os.ModeCharDevice) == 0:
		style = PlainStyle
	case bw:
		style = BWStyle
	case plain:
		style = PlainStyle
	default:
		style = FullStyle
	}
	return style, nil
}
