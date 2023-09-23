package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	. "github.com/go-git/go-git/v5/_examples"
)

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
	CheckIfError(err)

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

func main() {
	br, err := BlameFile("../../src/listme/parser.py")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s", br.String())
}
