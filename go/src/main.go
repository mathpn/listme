package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"regexp"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	// regexp "github.com/wasilibs/go-re2" // TODO decide between both libraries
)

var tags = []string{"BUG", "FIXME", "XXX", "TODO", "HACK", "OPTIMIZE", "NOTE"}

func findRoot(worktree *git.Worktree) (string, error) {
	// Get the absolute path of the current worktree.
	absPath, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		return "", err
	}

	// Check if the current directory is the root of the Git repository.
	if _, err := os.Stat(filepath.Join(absPath, ".git")); err == nil {
		return absPath, nil
	}

	// If it's not the root, move up one directory and check again.
	parentDir := filepath.Dir(absPath)
	if parentDir == absPath {
		return "", fmt.Errorf("git repository root not found")
	}

	return findRoot(worktree)
}

func BlameFile(path string) (*git.BlameResult, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: false})
	if err != nil {
		return nil, err
	}

	// Get the current worktree.
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	// Get the root of the Git repository.
	// This will traverse parent directories until it finds the root.
	rootPath, err := findRoot(worktree)
	if err != nil {
		return nil, err
	}

	// Retrieve the branch's HEAD, to then get the HEAD commit.
	ref, err := repo.Head()
	if err != nil {
		return nil, err
	}

	c, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, err
	}

	// Convert the relative path to an absolute path
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Info("git blame %s", path)

	gitPath := strings.ReplaceAll(absolutePath, rootPath, "")
	gitPath = strings.Trim(gitPath, "/ \t\n")

	// Blame the given file/path.
	br, err := git.Blame(c, gitPath)
	return br, err
}

func parseArgs(flagArgs []string) (string, []string) {
	if len(flagArgs) < 1 {
		log.Fatal("usage: listme <path>")
	}
	tags_regex := fmt.Sprintf(
		`(?m)(?:^|\s*(?:(?:#+|//+|<!--|--|/*|"""|''')+\s*)+)\s*(?:^|\b)(%s)[\s:;-]+(.+?)(?:$|-->|#}}|\*/|--}}|}}|#+|#}|"""|''')*$`,
		strings.Join(tags, "|"),
	)
	// fmt.Println(tags_regex)
	paths := []string{flagArgs[0]}
	return tags_regex, paths
}

func main() {
	var workers = flag.Int("w", 128, "[debug] set number of search workers")
	flag.Parse()

	query, paths := parseArgs(flag.Args())
	debug := &SearchDebug{
		Workers: *workers,
	}

	r, err := regexp.Compile(query)
	if err != nil {
		log.Fatalf("Bad regex: %s", err)
	}

	Search(paths, r, debug)
}

type SearchDebug struct {
	Workers int
}

type SearchOptions struct {
	Kind  int
	Lines bool
	Regex *regexp.Regexp
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

func Search(paths []string, regex *regexp.Regexp, debug *SearchDebug) {
	searchJobs := make(chan *searchJob)

	var wg sync.WaitGroup
	for w := 0; w < debug.Workers; w++ {
		go searchWorker(searchJobs, &wg)
	}
	for _, path := range paths {
		filepath.WalkDir(
			path,
			func(path string, d fs.DirEntry, err error) error { return walk(path, d, err, regex, searchJobs, &wg) },
		)
	}
	wg.Wait()
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

func searchWorker(jobs chan *searchJob, wg *sync.WaitGroup) {
	for job := range jobs {
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
				fmt.Printf("skipping non-text file: %s | %s\n", job.path, mimeType)
				break
			}

			match := job.regex.FindSubmatch(scanner.Bytes())
			if len(match) >= 3 {
				line := matchLine{n: line, tag: string(match[1]), text: string(match[2])}
				lines = append(lines, &line)
				// fmt.Printf("%v\n", line)
			}
			line++
		}
		for _, line := range lines {
			fmt.Printf("%s [Line %d] %s: %s\n", job.path, line.n, line.tag, line.text)
		}
		wg.Done()
	}
}
