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

	logging "github.com/op/go-logging"
	"golang.org/x/sys/unix"

	"github.com/mathpn/listme/blame"
	"github.com/mathpn/listme/matcher"
	"github.com/mathpn/listme/pretty"
)

var log = logging.MustGetLogger("listme")

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

func NewSearchParams(
	path string, tags []string, workers int, style pretty.Style, ageLimit int,
	fullPath bool, noSummary bool, noAuthor bool, glob string,
) *searchParams {
	matcher := matcher.NewMatcher(path, glob)
	regex := getRegex(tags)

	r, err := regexp.Compile(regex)
	if err != nil {
		log.Fatalf("Bad regex: %s", err)
	}

	return &searchParams{
		rootPath: filepath.ToSlash(path),
		regex:    r,
		matcher:  matcher,
		workers:  workers,
		style:    style,
		ageLimit: ageLimit,
		fullPath: fullPath,
		summary:  !noSummary,
		author:   !noAuthor,
	}
}

func getRegex(tags []string) string {
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

func (l *matchLine) Render(width int, gb *blame.GitBlame, maxLineNumber int, ageLimit int, style pretty.Style) {
	maxDigits := len(fmt.Sprint(maxLineNumber))
	// NOTE git author space + 20% reserved for line number
	maxTextWidth := width - maxDigits - int(0.2*float64(width)) - (blame.MaxAuthorLength + 6)

	prettyTag := pretty.Emojify(l.tag) + " "
	lenTag := len(l.tag) + 3
	maxLen := len(l.text) + lenTag
	for i := 0; i < maxLen; i += maxTextWidth {
		end := i + maxTextWidth
		if end > maxLen {
			end = maxLen
		}
		if i == 0 {
			// Print line number + tag + text + author info
			chunk := pretty.Colorize(pretty.Bold(prettyTag), l.tag, style) + pretty.Colorize(l.text[i:end-lenTag], l.tag, style)
			lineNumber := pretty.PadLineNumber(l.n, maxDigits)
			pad := strings.Repeat(" ", maxTextWidth-(end-i))
			chunk = chunk + pad
			var blameStr string
			if gb != nil {
				blame, err := gb.BlameLine(l.n)
				if err == nil {
					blameStr = " " + pretty.PrettifyBlame(blame, ageLimit, style)
				}
			}
			fmt.Println(lineNumber + chunk + blameStr)
		} else {
			// Print only the rest of the text
			chunk := pretty.Colorize(l.text[i-lenTag:end-lenTag], l.tag, style)
			lineNumber := strings.Repeat(" ", len(fmt.Sprint(maxLineNumber))+10)
			fmt.Println(lineNumber + chunk)
		}
	}
}

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
	fmt.Println(pretty.PrettifySummary(counter, style))
}

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
		fmt.Println(pretty.StylizeFilename(path, len(r.lines), params.style))
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
			if params.author {
				gb, _ = blame.BlameFile(job.path)
			}
			searchResults <- &searchResult{rootPath: params.rootPath, path: job.path, lines: lines, blame: gb}
		}
		wg.Done()
	}
}

func printResult(searchResults chan *searchResult, wgResult *sync.WaitGroup, params *searchParams) {
	width := getLimitedWidth()
	for result := range searchResults {
		result.Render(width, params)
		wgResult.Done()
	}
}

func getWinsize() (*unix.Winsize, error) {

	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return nil, os.NewSyscallError("GetWinsize", err)
	}

	return ws, nil
}

func getWidth() int {
	ws, err := getWinsize()

	if err != nil {
		return 50
	}

	return int(ws.Col)
}

func getLimitedWidth() int {
	width := getWidth()
	if width > 150 {
		width = 150
	}
	return width
}
