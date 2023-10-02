package matcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/op/go-logging"
	gitignore "github.com/sabhiram/go-gitignore"
)

var log = logging.MustGetLogger("listme")

// GitDirName is a special folder where all the git stuff is.
const GitDirName = ".git"

// Matcher provides a method that returns true if path should be scanned.
type Matcher interface {
	Match(path string) bool
}

type matcher struct {
	root string
	gi   map[string]*gitignore.GitIgnore
	glob string
}

// NewMatcher returns a Matcher. If a git repository is found on the provided path or on a
// parent directory, all .gitignore files are respected. The provided glob provides an additional
// filter.
//
// If a glob pattern is not needed, pass "*.*".
func NewMatcher(path string, glob string) Matcher {
	path = filepath.Clean(path)
	repoRoot, err := detectDotGit(path)
	if err != nil {
		log.Debugf("no git repository found in %s: %s", path, err)
		return &matcher{root: path, gi: make(map[string]*gitignore.GitIgnore, 0), glob: glob}
	}
	matchers, err := walkGitignore(repoRoot)
	if err != nil {
		log.Errorf("error while parsing .gitignore files: %s", err)
	}
	return &matcher{root: repoRoot, gi: matchers, glob: glob}

}

func walkGitignore(path string) (map[string]*gitignore.GitIgnore, error) {
	matchers := make(map[string]*gitignore.GitIgnore)

	var mu sync.Mutex
	parseGitignore := func(path string, wg *sync.WaitGroup) {
		defer wg.Done()
		matcher, err := gitignore.CompileIgnoreFile(path)
		if err != nil {
			log.Errorf("failed to parse .gitignore %s: %v\n", path, err)
			return
		}

		dir := filepath.Dir(path)

		mu.Lock()
		matchers[dir] = matcher
		mu.Unlock()
	}

	var wg sync.WaitGroup
	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Errorf("error accessing %s: %v\n", path, err)
			return nil
		}

		// Check if it's a directory and not a .git directory
		if info.IsDir() && !strings.HasSuffix(path, GitDirName) {
			// Check if a .gitignore file exists in the directory
			gitignorePath := filepath.Join(path, ".gitignore")
			if _, err := os.Stat(gitignorePath); err == nil {
				wg.Add(1)
				go parseGitignore(gitignorePath, &wg)
			}
		}
		return nil
	}

	err := filepath.Walk(path, walker)
	if err != nil {
		err = fmt.Errorf("error walking directory: %s", err)
		return matchers, err
	}
	wg.Wait()
	return matchers, nil
}

func (m *matcher) Match(path string) bool {
	if m.matchGitignore(path) {
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

func (m *matcher) matchGitignore(path string) bool {
	if len(m.gi) == 0 {
		return false
	}

	dir := filepath.Dir(path)
	for {
		matcher, ok := m.gi[dir]
		if ok {
			checkPath, err := filepath.Rel(dir, path)
			if err == nil {
				if matcher.MatchesPath(checkPath) {
					return true
				}
			} else {
				log.Errorf("error while getting relative path from %s using %s as root: %s", path, m.root, err)
			}
		}

		// Stop if we have reached the root of the repository
		if dir == m.root {
			return false
		}

		// Move up one directory in the hierarchy
		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			return false
		}
		dir = parentDir
	}
}

// MatchGit returns true if the path is a .git folder or is inside a .git folder.
func MatchGit(path string) bool {
	return strings.Contains(path, "/"+GitDirName+"/")
}

func detectDotGit(startDir string) (string, error) {
	startDir = filepath.ToSlash(startDir) // XXX windows
	startDir, err := replaceTildeWithHomeDir(startDir)
	if err != nil {
		return "", err
	}

	for {
		if isSystemRoot(startDir) {
			return "", fmt.Errorf("reached the system root directory")
		}

		if hasGitDirectory(startDir) {
			return startDir, nil
		}

		parentDir := filepath.Dir(startDir)

		if parentDir == startDir {
			return "", fmt.Errorf("reached the system root directory without finding a .git directory")
		}

		startDir = parentDir
	}
}

// Check if a directory is the system root
func isSystemRoot(dir string) bool {
	return dir == "/" || strings.HasSuffix(dir, `:\`)
}

// Check if a directory contains a .git directory
func hasGitDirectory(dir string) bool {
	gitDir := filepath.Join(dir, GitDirName)
	_, err := os.Stat(gitDir)
	return err == nil
}

func replaceTildeWithHomeDir(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = strings.Replace(path, "~", homeDir, 1)
	}

	return path, nil
}
