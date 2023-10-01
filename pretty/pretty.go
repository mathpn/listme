package pretty

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mathpn/listme/blame"

	"github.com/charmbracelet/lipgloss"
)

type Style int

const (
	FullStyle Style = iota
	BWStyle
	PlainStyle
)

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

func Bold(str string) string {
	return boldStyle.Render(str)
}

func PadLineNumber(number int, maxDigits int) string {
	strNumber := fmt.Sprint(number)
	pad := strings.Repeat(" ", maxDigits-len(strNumber))
	return fmt.Sprintf("  [Line %s%d] ", pad, number)
}


func StylizeFilename(rootPath string, file string, nComments int, style Style) string {
	file = shortenFilepath(file, rootPath)
	styler := baseStyle
	if style == BWStyle {
		styler = boldStyle
	} else if style == FullStyle {
		styler = filenameColorStyle
	}
	fname := styler.Render(fmt.Sprintf("• %s", file))
	var comments string
	if nComments > 1 {
		comments = fmt.Sprintf("(%d comments)", nComments)
	} else {
		comments = fmt.Sprintf("(%d comment)", nComments)
	}
	return fname + " " + comments
}

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
	}
	return "⚠ " + tag
}

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
	}
	return text
}

func PrettifyBlame(blame *blame.LineBlame, ageLimit int, style Style) string {
	blameStr := fmt.Sprintf("[%s]", blame.Author)
	if blame.Timestamp == 0 {
		return blameStr
	}
	date := time.Unix(blame.Timestamp, 0)
	currentDate := time.Now()

	diff := currentDate.Sub(date)
	maxAge := time.Duration(ageLimit) * 24 * time.Hour
	if diff > maxAge {
		blameStr = fmt.Sprintf("[OLD %s]", blame.Author)
		if style == FullStyle {
			blameStr = oldCommitStyle.Render(blameStr)
		}
	}
	return blameStr
}

func PrettifySummary(counter map[string]int, style Style) string {
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

func GetStyle(bw bool, plain bool) (Style, error) {
	if bw && plain {
		return -1, fmt.Errorf("only one style can be specified")
	}

	fi, _ := os.Stdout.Stat()
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return PlainStyle, nil
	} else if bw {
		return BWStyle, nil
	} else if plain {
		return PlainStyle, nil
	}
	return FullStyle, nil
}
