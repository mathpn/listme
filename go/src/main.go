package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"golang.org/x/sys/unix"
)

var tags = []string{"BUG", "FIXME", "XXX", "TODO", "HACK", "OPTIMIZE", "NOTE"}

const maxAuthorLength = 24

func BlameFile(path string) (*GitBlame, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	err = os.Chdir(filepath.Dir(absolutePath))
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("git", "blame", absolutePath, "--line-porcelain")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	blameChannel := make(chan []*LineBlame, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		parseGitBlame(stdout, blameChannel)
	}()

	wg.Wait()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git blame failed: %v\n%s", err, stderr.String())
	}

	blames := <-blameChannel
	return &GitBlame{blames: blames}, nil
}

type LineBlame struct {
	Author    string
	Timestamp int64
}

type GitBlame struct {
	blames []*LineBlame
}

func (b *GitBlame) BlameLine(line int) (*LineBlame, error) {
	line = line - 1
	if line < 0 || line >= len(b.blames) {
		return nil, fmt.Errorf("line out of range")
	}
	return b.blames[line], nil
}

func truncateName(name string, maxLength int) string {
	totalLen := len(name)
	words := strings.Fields(name) // Split the name into words

	truncated := []string{}
	for i := len(words) - 1; i >= 0; i-- {
		if totalLen > maxLength {
			if i == 0 {
				remLen := maxLength - 2*len(truncated)
				truncated = append(truncated, words[i][0:remLen]) // Truncate if first name
			} else {
				truncated = append(truncated, string(words[i][0])) // First letter
			}
			totalLen -= len(words[i]) - 2
		} else {
			truncated = append(truncated, words[i])
		}
	}

	for i, j := 0, len(truncated)-1; i < j; i, j = i+1, j-1 {
		truncated[i], truncated[j] = truncated[j], truncated[i]
	}

	return strings.Join(truncated, " ")
}

func parseGitBlame(out io.Reader, blameChannel chan []*LineBlame) {
	var blames []*LineBlame
	lr := bufio.NewReader(out) // XXX NewReaderSize?
	s := bufio.NewScanner(lr)

	var currentBlame *LineBlame
	for s.Scan() {
		buf := s.Text()
		if strings.HasPrefix(buf, "author ") {
			if currentBlame != nil {
				blames = append(blames, currentBlame)
			}
			currentBlame = &LineBlame{
				Author: truncateName(strings.TrimPrefix(buf, "author "), maxAuthorLength),
			}
		} else if strings.HasPrefix(buf, "author-time ") {
			if currentBlame != nil {
				tsStr := strings.TrimPrefix(buf, "author-time ")
				ts, err := strconv.ParseInt(tsStr, 10, 64)
				if err == nil {
					currentBlame.Timestamp = ts
				}
			}
		}
	}

	// Append the last entry
	if currentBlame != nil {
		blames = append(blames, currentBlame)
	}

	blameChannel <- blames
}

func parseArgs(flagArgs []string) (string, string) {
	if len(flagArgs) < 1 {
		log.Fatal("usage: listme <path>")
	}
	tags_regex := fmt.Sprintf(
		`(?m)(?:^|\s*(?:(?:#+|//+|<!--|--|/*|"""|''')+\s*)+)\s*(?:^|\b)(%s)[\s:;-]+(.+?)(?:$|-->|#}}|\*/|--}}|}}|#+|#}|"""|''')*$`,
		strings.Join(tags, "|"),
	)
	return tags_regex, flagArgs[0]
}

func LoadGitignore(path string) (gitignore.Matcher, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	rootDir := wt.Filesystem

	patterns, err := gitignore.ReadPatterns(rootDir, []string{})
	if err != nil {
		return nil, err
	}
	matcher := gitignore.NewMatcher(patterns)
	return matcher, nil
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

func main() {
	fi, _ := os.Stdout.Stat()
	var style Style
	style = FullStyle
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		style = PlainStyle
	}
	var workers = flag.Int("w", 128, "set number of search workers")
	flag.Parse()

	query, path := parseArgs(flag.Args())
	opt := &Options{
		Workers: *workers,
		Style:   style,
	}

	r, err := regexp.Compile(query)
	if err != nil {
		log.Fatalf("Bad regex: %s", err)
	}

	matcher := NewMatcher(path, "*.*")
	Search(path, r, matcher, opt)
}

type Matcher interface {
	Match(path string) bool
}

type matcher struct {
	gitignore gitignore.Matcher
	glob      string
}

func (m *matcher) Match(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("%s does not exist.\n", path)
		} else {
			fmt.Printf("Error checking %s: %v\n", path, err)
		}
		return false
	}
	pathList := strings.Split(path, string(filepath.Separator))
	if m.gitignore != nil && m.gitignore.Match(pathList, info.IsDir()) {
		return false
	}
	base := filepath.Base(path)
	matched, err := filepath.Match(m.glob, base)
	if err != nil {
		return true
	}
	if matched {
		return true
	}
	return false
}

func NewMatcher(path string, globPattern string) Matcher {
	gitMatcher, _ := LoadGitignore(path)
	return &matcher{gitignore: gitMatcher, glob: globPattern}
}
type Options struct {
	Workers int
	Style   Style
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

// TODO messy, refactor
func (l *matchLine) Render(width int, gb *GitBlame, maxLineNumber int, ageLimit int, style Style) {
	maxDigits := len(fmt.Sprint(maxLineNumber))
	maxTextWidth := width - maxDigits - int(0.2*float64(width)) - (maxAuthorLength + 8)

	prettyTag := emojify(l.tag) + " "
	lenTag := len(l.tag) + 3
	maxLen := len(l.text) + lenTag
	for i := 0; i < maxLen; i += maxTextWidth {
		end := i + maxTextWidth
		if end > maxLen {
			end = maxLen
		}
		var chunk string
		if i == 0 {
			if style == FullStyle {
				chunk = colorize(BoldStyle.Render(prettyTag), l.tag) + colorize(l.text[i:end-lenTag], l.tag)
			} else {
				chunk = BoldStyle.Render(prettyTag) + l.text[i:end-lenTag]
			}
			lineNumber := padLineNumber(l.n, maxDigits)
			pad := strings.Repeat(" ", maxTextWidth-(end-i))
			chunk = chunk + pad
			var blame *LineBlame
			var err error
			if gb != nil {
				blame, err = gb.BlameLine(l.n)
			}
			if gb != nil && err == nil {
				blameStr := prettiyfyBlame(blame, ageLimit, style)
				fmt.Println(lineNumber + chunk + " " + blameStr)
			} else {
				fmt.Println(lineNumber + chunk)
			}
		} else {
			chunk = colorize(l.text[i-lenTag:end-lenTag], l.tag)
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
	blame    *GitBlame
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

var BorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).MarginLeft(2)

func shortenFilepath(path string, rootPath string) string {
	return strings.Trim(strings.Replace(path, rootPath, "", 1), string(filepath.Separator))
}

func (r *searchResult) printBox() {
	counter := make(map[string]int, len(tags))
	for i := 0; i < len(r.lines); i++ {
		counter[r.lines[i].tag]++
	}
	if len(counter) < 2 {
		return
	}

	tags := make([]string, 0, len(counter))
	for tag := range counter {
		tags = append(tags, tag)
	}

	sort.Strings(tags)
	boxStr := " "
	for _, tag := range tags {
		boxStr += colorize(fmt.Sprintf(" %s %d ", emojify(tag), counter[tag]), tag)
	}
	fmt.Println(BorderStyle.Render(boxStr + " "))
}

func (r *searchResult) Render(width int, style Style) {
	switch style {
	case PlainStyle:
		for _, line := range r.lines {
			line.PlainRender(r.path)
		}
	default:
		path := shortenFilepath(r.path, r.rootPath)
		fmt.Println(stylizeFilename(path, len(r.lines), style))
		r.printBox()
		maxLineNumber := r.maxLineNumber()
		for _, line := range r.lines {
			line.Render(width, r.blame, maxLineNumber, AGELIMIT, style)
		}
		fmt.Println()
	}
}

type Style int

const (
	FullStyle = iota
	BWStyle
	PlainStyle
)

func Search(path string, regex *regexp.Regexp, matcher gitignore.Matcher, opt *Options) {
	searchJobs := make(chan *searchJob)
	searchResults := make(chan *searchResult)

	var wg sync.WaitGroup
	var wgResult sync.WaitGroup
	for w := 0; w < opt.Workers; w++ {
		go searchWorker(path, searchJobs, searchResults, matcher, &wg, &wgResult)
	}

	go PrintResult(searchResults, &wgResult, opt)

	filepath.WalkDir(
		path,
		func(path string, d fs.DirEntry, err error) error { return walk(path, d, err, regex, searchJobs, &wg) },
	)
	wg.Wait()
	wgResult.Wait()
}

// TODO organize code
var BaseStyle = lipgloss.NewStyle()
var BoldStyle = BaseStyle.Copy().Bold(true)
var FilenameColorStyle = BoldStyle.Copy().Foreground(lipgloss.Color("#0087d7"))

func stylizeFilename(file string, nComments int, style Style) string {
	styler := BaseStyle
	if style == BWStyle {
		styler = BoldStyle
	} else if style == FullStyle {
		styler = FilenameColorStyle
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

func emojify(tag string) string {
	// TODO extra symbols support config
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

var TodoStyle = BaseStyle.Copy().Foreground(lipgloss.Color("#5fafaf"))
var XxxStyle = BaseStyle.Copy().Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#d7af00"))
var FixmeStyle = BaseStyle.Copy().Foreground(lipgloss.Color("#ff0000"))
var OptimizeStyle = BaseStyle.Copy().Foreground(lipgloss.Color("#d75f00"))
var BugStyle = BaseStyle.Copy().Foreground(lipgloss.Color("#eeeeee")).Background(lipgloss.Color("#870000"))
var NoteStyle = BaseStyle.Copy().Foreground(lipgloss.Color("#87af87"))
var HackStyle = BaseStyle.Copy().Foreground(lipgloss.Color("#d7d700"))

func colorize(text string, tag string) string {
	switch tag {
	case "TODO":
		return TodoStyle.Render(text)
	case "XXX":
		return XxxStyle.Render(text)
	case "FIXME":
		return FixmeStyle.Render(text)
	case "OPTIMIZE":
		return OptimizeStyle.Render(text)
	case "BUG":
		return BugStyle.Render(text)
	case "NOTE":
		return NoteStyle.Render(text)
	case "HACK":
		return HackStyle.Render(text)
	}
	return text
}

func padLineNumber(number int, maxDigits int) string {
	strNumber := fmt.Sprint(number)
	pad := strings.Repeat(" ", maxDigits-len(strNumber))
	return fmt.Sprintf("  [Line %s%d] ", pad, number)
}

var OldCommitStyle = BoldStyle.Copy().Foreground(lipgloss.Color("#dadada")).Background(lipgloss.Color("#d70000"))

func prettiyfyBlame(blame *LineBlame, ageLimit int, style Style) string {
	if style == PlainStyle {
		return ""
	}

	blameStr := fmt.Sprintf("[%s]", blame.Author)
	if blame.Timestamp == 0 {
		return blameStr
	}
	date := time.Unix(blame.Timestamp, 0)
	currentDate := time.Now()

	diff := currentDate.Sub(date)
	maxAge := time.Duration(ageLimit) * 24 * time.Hour
	if diff > maxAge {
		blameStr := fmt.Sprintf("[OLD %s]", blame.Author)
		return OldCommitStyle.Render(blameStr)
	}
	return blameStr
}

const STYLE = FullStyle // TODO parameter
const AGELIMIT = 30     // TODO parameter

func PrintResult(searchResults chan *searchResult, wgResult *sync.WaitGroup, opt *Options) {
	width := getLimitedWidth()
	for result := range searchResults {
		result.Render(width, opt.Style)
		wgResult.Done()
	}
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
	rootPath string, jobs chan *searchJob, searchResults chan *searchResult,
	matcher gitignore.Matcher, wg *sync.WaitGroup, wgResult *sync.WaitGroup,
) {
	for job := range jobs {
		info, err := os.Stat(job.path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("%s does not exist.\n", job.path)
			} else {
				fmt.Printf("Error checking %s: %v\n", job.path, err)
			}
			return
		}

		pathList := strings.Split(job.path, string(filepath.Separator))
		if matcher != nil && matcher.Match(pathList, info.IsDir()) {
			// fmt.Printf("skipping %s due to .gitignore\n", job.path)
			wg.Done()
			continue
		}
		f, err := os.Open(job.path)
		if err != nil {
			log.Fatalf("couldn't open path %s: %s\n", job.path, err)
		}

		scanner := bufio.NewScanner(f)

		line := 1
		lines := make([]*matchLine, 0)
		for scanner.Scan() {
			text := scanner.Bytes()

			if mimeType := http.DetectContentType(text); strings.Split(mimeType, ";")[0] != "text/plain" {
				// fmt.Printf("skipping non-text file: %s | %s\n", job.path, mimeType)
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
			gb, _ := BlameFile(job.path)
			searchResults <- &searchResult{rootPath: rootPath, path: job.path, lines: lines, blame: gb}
		}
		wg.Done()
	}
}
