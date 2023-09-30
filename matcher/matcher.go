package matcher

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("listme")

type Matcher interface {
	Match(path string) bool
}

type matcher struct {
	gitignore gitignore.Matcher
	glob      string
}

func (m *matcher) Match(path string) bool {
	info, err := os.Stat(filepath.FromSlash(path))
	if err != nil {
		if os.IsNotExist(err) {
			log.Warningf("%s does not exist.\n", path)
		} else {
			log.Errorf("Error checking %s: %v\n", path, err)
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

func MatchGit(path string) bool {
	return strings.Contains(path, "/.git/")
}

func NewMatcher(path string, globPattern string) Matcher {
	gitMatcher, _ := loadGitignore(path)
	return &matcher{gitignore: gitMatcher, glob: globPattern}
}

func loadGitignore(path string) (gitignore.Matcher, error) {
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
