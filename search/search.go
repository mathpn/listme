package search

import (
	"bufio"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	tsize "github.com/kopoli/go-terminal-size"
	logging "github.com/op/go-logging"

	"github.com/mathpn/listme/blame"
	"github.com/mathpn/listme/matcher"
	"github.com/mathpn/listme/pretty"
)

var log = logging.MustGetLogger("listme")
var ansiRegex = regexp.MustCompile("\x1b(\\[[0-9;]*[A-Za-z])")

// limiting the width improves readability of git author info
const maxWidth = 120
const defaultWidth = 75

type searchParams struct {
	rootPath string
	regex    *regexp.Regexp
	matcher  matcher.Matcher
	workers  int
	style    pretty.Style
	ageLimit int
	fullPath bool
	summary  bool
	author   bool
}

// NewSearchParams creates a searchParams struct with all the information required
// to inspect a file or folder.
func NewSearchParams(
	path string, tags []string, workers int, style pretty.Style, ageLimit int,
	fullPath bool, noSummary bool, noAuthor bool, glob string,
) (*searchParams, error) {
	absPath, err := filepath.Abs(filepath.ToSlash(path))
	if err != nil {
		log.Fatalf("error while building absolute path for %s: %s", path, err)
	}

	matcher := matcher.NewMatcher(absPath, glob)
	regex := getTagRegex(tags)

	r, err := regexp.Compile(regex)
	if err != nil {
		return nil, fmt.Errorf("bad regex: %s", err)
	}

	return &searchParams{
		rootPath: absPath,
		regex:    r,
		matcher:  matcher,
		workers:  workers,
		style:    style,
		ageLimit: ageLimit,
		fullPath: fullPath,
		summary:  !noSummary,
		author:   !noAuthor,
	}, nil
}

func getTagRegex(tags []string) string {
	tagsRegex := fmt.Sprintf(
		`(?m)(?:^|\s*(?:(?:#+|//+|<!--|--|/*|"""|''')+\s*)+)\s*(?:^|\b)(%s)[\s:;-]+(.+?)(?:$|-->|#}}|\*/|--}}|}}|#+|#}|"""|''')*$`,
		strings.Join(tags, "|"),
	)
	return tagsRegex
}

type searchJob struct {
	path  string
	regex *regexp.Regexp
}

type matchLine struct {
	n    int
	tag  string
	text string
}

// Wraps a long string on words with a max lineWidth.
// Adapted from https://codereview.stackexchange.com/questions/244435/word-wrap-in-go
// to count emojis as 1 character and ignore ANSI escape sequences. It's much slower though.
func wordWrap(text string, lineWidth int) string {
	wrap := make([]byte, 0, len(text)+2*len(text)/lineWidth)
	eoLine := lineWidth
	running := 0
	inWord := false
	for i, j := 0, 0; ; {
		r, size := utf8.DecodeRuneInString(text[i:])
		if size == 0 && r == utf8.RuneError {
			r = ' '
		}
		if unicode.IsSpace(r) {
			if inWord {
				wl := utf8.RuneCountInString(removeANSIEscapeCodes(text[j:i]))
				if running+wl >= eoLine {
					wrap = append(wrap, '\n')
					running = 0
				} else if len(wrap) > 0 {
					wrap = append(wrap, ' ')
					running++
				}
				running += wl
				wrap = append(wrap, text[j:i]...)
			}
			inWord = false
		} else if !inWord {
			inWord = true
			j = i
		}
		if size == 0 && r == ' ' {
			break
		}
		i += size
	}
	return string(wrap)
}

func removeANSIEscapeCodes(input string) string {
	cleaned := ansiRegex.ReplaceAllString(input, "")
	return cleaned
}

// Render the line and print it to stdout using the provided style.
// Depending on the width of the terminal, multiple lines may be printed.
func (l *matchLine) Render(width int, gb *blame.GitBlame, maxLineNumber int, ageLimit int, style pretty.Style) {
	maxDigits := len(fmt.Sprint(maxLineNumber))
	lnSize := maxDigits + 9
	maxTextWidth := width - lnSize - (blame.MaxAuthorLength + 7)

	lenTag := len(l.tag) + 3
	if maxTextWidth < lenTag {
		log.Fatal("terminal is too narrow")
	}

	line := pretty.Bold(pretty.Emojify(l.tag)) + " " + l.text
	wrapLine := wordWrap(line, maxTextWidth)
	for i, chunk := range strings.Split(wrapLine, "\n") {
		if i == 0 {
			// Print lineNumber + tag + text + author info
			cl := utf8.RuneCountInString(removeANSIEscapeCodes(chunk))
			chunk := pretty.Colorize(chunk, l.tag, style)
			lineNumber := pretty.PrettyLineNumber(l.n, maxDigits)
			pad := strings.Repeat(" ", maxTextWidth-cl)
			chunk = chunk + pad
			var blameStr string
			if gb != nil {
				blame, err := gb.BlameLine(l.n)
				if err == nil {
					blameStr = " " + pretty.PrettyBlame(blame, ageLimit, style)
				}
			}
			fmt.Println(lineNumber + chunk + blameStr)
		} else {
			// Print only the rest of the text
			chunk = pretty.Colorize(chunk, l.tag, style)
			lineNumber := strings.Repeat(" ", len(fmt.Sprint(maxLineNumber))+10)
			fmt.Println(lineNumber + chunk)
		}
	}
}

// Render the line and print it to stdout using the plain style format.
func (l *matchLine) PlainRender(path string) {
	fmt.Printf("%s:%d:%s:%s\n", path, l.n, l.tag, l.text)
}

type searchResult struct {
	rootPath string
	path     string
	lines    []*matchLine
	blame    *blame.GitBlame
}

func (r *searchResult) maxLineNumber() int {
	max := 0
	for _, line := range r.lines {
		if line.n > max {
			max = line.n
		}
	}
	return max
}

func (r *searchResult) printSummary(style pretty.Style) {
	counter := make(map[string]int, 10)
	for i := 0; i < len(r.lines); i++ {
		counter[r.lines[i].tag]++
	}
	if len(counter) < 2 {
		return
	}
	fmt.Println(pretty.PrettySummary(counter, style))
}

// Render and print the filename and all matching lines to stdout.
func (r *searchResult) Render(width int, params *searchParams) {
	path := r.path
	if !params.fullPath {
		path = shortenFilepath(path, r.rootPath)
	}
	switch params.style {
	case pretty.PlainStyle:
		for _, line := range r.lines {
			line.PlainRender(path)
		}
	default:
		fmt.Println(pretty.PrettyFilename(path, len(r.lines), params.style))
		if params.summary {
			r.printSummary(params.style)
		}
		maxLineNumber := r.maxLineNumber()
		for _, line := range r.lines {
			line.Render(width, r.blame, maxLineNumber, params.ageLimit, params.style)
		}
		fmt.Println()
	}
}

func shortenFilepath(path string, rootPath string) string {
	shortPath := strings.Trim(strings.Replace(path, rootPath, "", 1), string(os.PathSeparator))
	if shortPath == "" {
		shortPath = filepath.Base(path)
	}
	return shortPath
}

// Search a file or folder for the specified tags.
// Use the function NewSearchParams to create the required struct.
func Search(params *searchParams) {
	searchJobs := make(chan *searchJob)
	searchResults := make(chan *searchResult)

	var wg sync.WaitGroup
	var wgResult sync.WaitGroup
	for w := 0; w < params.workers; w++ {
		go searchWorker(params, searchJobs, searchResults, &wg, &wgResult)
	}

	go printResult(searchResults, &wgResult, params)

	filepath.WalkDir(
		params.rootPath,
		func(path string, d fs.DirEntry, err error) error {
			return walk(path, d, err, params.regex, searchJobs, &wg)
		},
	)
	wg.Wait()
	wgResult.Wait()
}

func walk(path string, d fs.DirEntry, err error, regex *regexp.Regexp, searchJobs chan *searchJob, wg *sync.WaitGroup) error {
	if err != nil {
		return err
	}
	if !d.IsDir() {
		wg.Add(1)
		searchJobs <- &searchJob{
			path,
			regex,
		}
	}
	return nil
}

func searchWorker(
	params *searchParams, jobs chan *searchJob, searchResults chan *searchResult,
	wg *sync.WaitGroup, wgResult *sync.WaitGroup,
) {
	for job := range jobs {
		if matcher.MatchGit(job.path) {
			log.Debugf("skipping %s since it's in a .git directory\n", job.path)
			wg.Done()
			continue
		}
		if !params.matcher.Match(job.path) {
			log.Warningf("skipping %s due to .gitignore or glob pattern\n", job.path)
			wg.Done()
			continue
		}
		f, err := os.Open(filepath.FromSlash(job.path))
		if err != nil {
			log.Fatalf("couldn't open path %s: %s\n", job.path, err)
		}

		scanner := bufio.NewScanner(f)

		line := 1
		lines := make([]*matchLine, 0)
		for scanner.Scan() {
			text := scanner.Bytes()

			if mimeType := http.DetectContentType(text); !strings.HasPrefix(strings.Split(mimeType, ";")[0], "text") {
				log.Warningf("skipping non-text file of type %s: %s\n", mimeType, job.path)
				break
			}

			match := job.regex.FindSubmatch(scanner.Bytes())
			if len(match) >= 3 {
				line := matchLine{n: line, tag: string(match[1]), text: string(match[2])}
				lines = append(lines, &line)
			}
			line++
		}
		if len(lines) > 0 {
			wgResult.Add(1)
			var gb *blame.GitBlame
			if params.author && params.style != pretty.PlainStyle {
				gb, _ = blame.BlameFile(job.path)
			}
			searchResults <- &searchResult{rootPath: params.rootPath, path: job.path, lines: lines, blame: gb}
		}
		wg.Done()
	}
}

func printResult(searchResults chan *searchResult, wgResult *sync.WaitGroup, params *searchParams) {
	var width int
	if params.style != pretty.PlainStyle {
		width = getLimitedWidth()
	}
	for result := range searchResults {
		result.Render(width, params)
		wgResult.Done()
	}
}

func getWidth() int {
	s, err := tsize.GetSize()

	if err != nil {
		log.Errorf("couldn't read terminal size, using width %d: %s", defaultWidth, err)
		return defaultWidth
	}

	return s.Width
}

func getLimitedWidth() int {
	width := getWidth()
	if width > maxWidth {
		width = maxWidth
	}
	return width
}
