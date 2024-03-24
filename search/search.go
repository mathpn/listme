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
	"time"
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
var zeroTime = time.Unix(0, 0)

// limiting the width improves readability of git author info
const maxWidth = 120
const defaultWidth = 75
const noComment = "\x1b[3m[no comment]\x1b[23m" // italic

type searchParams struct {
	matcher       matcher.Matcher
	regex         *regexp.Regexp
	rootPath      string
	author        string
	style         pretty.Style
	workers       int
	oldCommitTime time.Time
	commitAgeTime time.Time
	maxFs         int64
	fullPath      bool
	summary       bool
	showAuthor    bool
}

// NewSearchParams creates a searchParams struct with all the information required
// to inspect a file or directory.
func NewSearchParams(
	path string,
	tags []string,
	workers int,
	style pretty.Style,
	oldCommitLimit, commitAgeFilter int,
	maxFileSize int64,
	fullPath, noSummary, noAuthor bool,
	glob, author string,
) (*searchParams, error) {
	absPath, err := filepath.Abs(filepath.ToSlash(path))
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %s: %s", path, err)
	}

	matcher := matcher.NewMatcher(absPath, glob)
	regex := getTagRegex(tags)

	r, err := regexp.Compile(regex)
	if err != nil {
		return nil, fmt.Errorf("failed to compile regex: %s", err)
	}

	currentTime := time.Now()
	maxAge := time.Duration(oldCommitLimit) * 24 * time.Hour
	oldCommitTime := currentTime.Add(-maxAge)

	commitAgeTime := zeroTime
	if commitAgeFilter != -1 {
		maxAge = time.Duration(commitAgeFilter) * 24 * time.Hour
		commitAgeTime = currentTime.Add(-maxAge)
	}

	return &searchParams{
		rootPath:      absPath,
		regex:         r,
		matcher:       matcher,
		workers:       workers,
		style:         style,
		oldCommitTime: oldCommitTime,
		maxFs:         maxFileSize,
		fullPath:      fullPath,
		summary:       !noSummary,
		showAuthor:    !noAuthor,
		author:        author,
		commitAgeTime: commitAgeTime,
	}, nil
}

func getTagRegex(tags []string) string {
	tagsRegex := fmt.Sprintf(
		`(?m)(?:^|\s*(?:(?:#+|//+|<!--|--|/*|"""|''')+\s*)+)\s*(?:^|\b)(%s)(?:[\s:;-]|$)(.*?)(?:$|-->|#}}|\*/|--}}|}}|#+|#}|"""|''')*$`,
		strings.Join(tags, "|"),
	)
	return tagsRegex
}

type searchJob struct {
	regex *regexp.Regexp
	path  string
}

type matchLine struct {
	blame *blame.LineBlame
	tag   string
	text  string
	n     int
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
func (l *matchLine) Render(
	width int,
	maxLineNumber int,
	oldCommitTime time.Time,
	showAuthor bool,
	style pretty.Style,
) {
	maxDigits := len(fmt.Sprint(maxLineNumber))
	lnSize := maxDigits + 9
	maxTextWidth := width - lnSize - (blame.MaxAuthorLength + 7)

	lenTag := len(l.tag) + 3
	if maxTextWidth < lenTag {
		log.Fatal("terminal is too narrow")
	}

	text := strings.TrimSpace(l.text)
	if text == "" {
		text = noComment
	}

	line := pretty.Bold(pretty.Emojify(l.tag)) + " " + text
	wrapLine := wordWrap(line, maxTextWidth)
	for i, chunk := range strings.Split(wrapLine, "\n") {
		if i == 0 {
			// Print lineNumber + tag + text + author info
			cl := utf8.RuneCountInString(removeANSIEscapeCodes(chunk))
			chunk = pretty.Colorize(chunk, l.tag, style)
			lineNumber := pretty.PrettyLineNumber(l.n, maxDigits)
			pad := strings.Repeat(" ", maxTextWidth-cl)
			chunk = chunk + pad
			var blameStr string
			if showAuthor && l.blame != nil {
				blameStr = " " + pretty.PrettyBlame(l.blame, oldCommitTime, style)
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
			line.Render(width, maxLineNumber, params.oldCommitTime, params.showAuthor, params.style)
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

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Errorf("file walk error: %s", err)
			return err
		}

		if matcher.MatchGit(path) {
			log.Infof("skipping .git directory: %s", path)
			return filepath.SkipDir
		}

		isDir := d.IsDir()
		switch params.matcher.Match(path) {
		case matcher.GitIgnore:
			log.Infof("skipping %s due to .gitignore", path)
			if isDir {
				return filepath.SkipDir
			}
			return nil
		case matcher.GlobIgnore:
			log.Infof("skipping %s due to glob pattern", path)
			return nil
		}

		if isDir {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			log.Errorf("error getting file info for %s: %s", path, err)
			return nil
		}
		if info.Size() > params.maxFs<<20 {
			log.Warningf("skipping file larger than %dMB: %s", params.maxFs, path)
			return nil
		}
		wg.Add(1)
		searchJobs <- &searchJob{regex: params.regex, path: path}
		return nil
	}

	filepath.WalkDir(params.rootPath, walk)
	wg.Wait()
	wgResult.Wait()
}

func searchWorker(
	params *searchParams,
	jobs chan *searchJob,
	searchResults chan *searchResult,
	wg, wgResult *sync.WaitGroup,
) {
	for job := range jobs {
		lines := scanFile(params, job)
		if len(lines) > 0 {
			wgResult.Add(1)
			searchResults <- &searchResult{rootPath: params.rootPath, path: job.path, lines: lines}
		}
		wg.Done()
	}
}

func scanFile(
	params *searchParams,
	job *searchJob,
) []*matchLine {
	log.Debugf("scanning file %s", job.path)

	var lines []*matchLine
	f, err := os.Open(filepath.FromSlash(job.path))
	if err != nil {
		log.Fatalf("couldn't open path %s: %s", job.path, err)
		return lines
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	var gb *blame.GitBlame
	var triedBlame bool
	var lineBlame *blame.LineBlame

	requiresBlame := params.author != "" || (params.showAuthor && params.style != pretty.PlainStyle)

	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		text := scanner.Bytes()

		mimeType := http.DetectContentType(text)
		if !strings.HasPrefix(strings.SplitN(mimeType, ";", 1)[0], "text") {
			log.Infof("skipping non-text file of type %s: %s", mimeType, job.path)
			break
		}

		match := job.regex.FindSubmatch(text)
		if len(match) < 3 {
			continue
		}

		if requiresBlame && !triedBlame {
			gb, _ = blame.BlameFile(job.path)
			triedBlame = true
		}

		if requiresBlame && gb != nil {
			lineBlame, _ = gb.BlameLine(lineNumber)
		}

		line := &matchLine{blame: lineBlame, n: lineNumber, tag: string(match[1]), text: string(match[2])}
		if validLine(job.path, line, params) {
			lines = append(lines, line)
		}
	}

	if err = scanner.Err(); err != nil {
		switch err {
		case bufio.ErrTooLong:
			log.Errorf(
				"file %s has lines exceeding the maximum size of %dKB, results may be incomplete",
				job.path,
				bufio.MaxScanTokenSize>>10,
			)
		default:
			log.Errorf("error while searching for tags in file %s - %s", job.path, err)
		}
	}
	return lines
}

func validLine(path string, line *matchLine, params *searchParams) bool {
	if params.author != "" && (line.blame == nil || line.blame.Author != params.author) {
		log.Debugf("skipping %s line %d due to author filter", path, line.n)
		return false
	}
	if !params.commitAgeTime.Equal(zeroTime) {
		if line.blame == nil {
			log.Debugf("skipping %s line %d due to commit age: no git blame", path, line)
			return false
		}

		if line.blame.Time.Before(params.commitAgeTime) {
			log.Debugf("skipping %s line %d due to commit age", path, line)
			return false
		}
	}
	return true
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
		log.Warningf("couldn't read terminal size, using width %d: %s", defaultWidth, err)
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
