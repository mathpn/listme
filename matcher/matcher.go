package matcher

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/op/go-logging"
	gitignore "github.com/sabhiram/go-gitignore"
)

var log = logging.MustGetLogger("listme")

const (
	// gitDirName is a special folder where all the git stuff is.
	gitDirName = ".git"
	separator  = string(os.PathSeparator)
)

type MatchType int

const (
	GitIgnore MatchType = iota
	GlobIgnore
	Match
)

// Matcher provides a method that returns the match type.
//   - Match: file should be scanned
//   - GitIgnore: ignored due to .gitignore
//   - GlobIgnore: ignored due to glob pattern
type Matcher interface {
	Match(path string) MatchType
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
// If a glob pattern is not needed, pass '*'.
func NewMatcher(path string, glob string) Matcher {
	path = filepath.Clean(path)
	repoRoot, err := detectDotGit(path)
	if err != nil {
		log.Debugf("no git repository found in %s: %s", path, err)
		return &matcher{root: path, gi: make(map[string]*gitignore.GitIgnore, 0), glob: glob}
	}
	matchers, err := walkGitignore(repoRoot, path)
	if err != nil {
		log.Errorf("error while parsing .gitignore files: %s", err)
	}
	return &matcher{root: repoRoot, gi: matchers, glob: glob}

}

func walkGitignore(repoRoot string, refPath string) (map[string]*gitignore.GitIgnore, error) {
	matchers := make(map[string]*gitignore.GitIgnore)

	parseGitignore := func(path string) {
		matcher, err := gitignore.CompileIgnoreFile(path)
		if err != nil {
			log.Warningf("failed to parse .gitignore %s: %v\n", path, err)
			return
		}

		dir := filepath.Dir(path)
		matchers[dir] = matcher
	}

	walker := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Errorf("file walk error: %s", err)
			return nil
		}

		isDir := d.IsDir()

		if !isDir {
			return nil
		}

		if MatchGit(path) {
			log.Debugf(".gitignore search: skipping .git folder %s", path)
			return filepath.SkipDir
		}

		// Check if the path is in the hierarchy of refPath
		isSub, _ := isSubfolder(path, refPath)
		isSubRev, _ := isSubfolder(refPath, path)
		if !isSub && !isSubRev {
			log.Debugf(".gitignore search: skipping %s since it's outside of %s hierachy", path, refPath)
			return filepath.SkipDir
		}

		// If an entire folder is ignored by a .gitignore, stop walking
		if gitignoreMatch(matchers, path, repoRoot) {
			log.Debugf(".gitignore search: skipping %s due to .gitignore patterns", path)
			return filepath.SkipDir
		}

		// Check if it's a directory and not a .git directory
		if !strings.HasSuffix(path, gitDirName) {
			// Check if a .gitignore file exists in the directory
			gitignorePath := filepath.Join(path, ".gitignore")
			if _, err := os.Stat(gitignorePath); err == nil {
				log.Debugf("parsing new .gitignore file: %s", gitignorePath)
				parseGitignore(gitignorePath)
			}
		}

		return nil
	}

	err := filepath.WalkDir(repoRoot, walker)
	if err != nil {
		err = fmt.Errorf("error walking directory: %s", err)
		return matchers, err
	}
	return matchers, nil
}

func (m *matcher) Match(path string) MatchType {
	if gitignoreMatch(m.gi, path, m.root) {
		return GitIgnore
	}
	base := filepath.Base(path)
	matched, err := filepath.Match(m.glob, base)
	if err != nil {
		log.Infof("glob match error with path %s: %s", path, err)
		return Match
	}
	if !matched {
		return GlobIgnore
	}
	return Match
}

func gitignoreMatch(matchers map[string]*gitignore.GitIgnore, path string, root string) bool {
	if len(matchers) == 0 {
		return false
	}

	dir := filepath.Dir(path)
	for {
		matcher, ok := matchers[dir]
		if ok {
			checkPath, err := filepath.Rel(dir, path)
			if err == nil {
				if matcher.MatchesPath(checkPath) {
					return true
				}
			} else {
				log.Errorf("error while getting relative path from %s using %s as root: %s", path, root, err)
			}
		}

		// Stop if we have reached the root of the repository
		if dir == root {
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
	return strings.HasSuffix(path, separator+gitDirName) || strings.Contains(path, separator+gitDirName+separator)
}

func detectDotGit(startDir string) (string, error) {
	startDir, err := replaceTildeWithHomeDir(startDir)
	if err != nil {
		return "", err
	}

	for {
		log.Debugf("searching for git repo in %s", startDir)
		if isSystemRoot(startDir) {
			return "", fmt.Errorf("reached the system root directory")
		}

		if hasGitDirectory(startDir) {
			log.Debugf("found git repo root in %s", startDir)
			return startDir, nil
		}

		parentDir := filepath.Dir(startDir)

		if parentDir == startDir {
			return "", fmt.Errorf("reached the system root directory without finding a .git directory")
		}

		startDir = parentDir
	}
}

func isSubfolder(subfolder, parentFolder string) (bool, error) {
	relPath, err := filepath.Rel(parentFolder, subfolder)
	if err != nil {
		log.Errorf("subfolder check failed: %s", err)
		return false, err
	}

	// Check if the relative path starts with ".." which indicates subfolder
	return !strings.HasPrefix(relPath, ".."), nil
}

// Check if a directory is the system root
func isSystemRoot(dir string) bool {
	return dir == "/" || strings.HasSuffix(dir, `:\`)
}

// Check if a directory contains a .git directory
func hasGitDirectory(dir string) bool {
	gitDir := filepath.Join(dir, gitDirName)
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
