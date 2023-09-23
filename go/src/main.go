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
		return "", fmt.Errorf("Git repository root not found")
	}

	return findRoot(worktree)
}

// Basic example of how to blame a repository.
func main() {
	CheckArgs("<file_to_blame>")
	path := os.Args[1]

	fmt.Println("1")
	repo, err := git.PlainOpenWithOptions(".", &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: false})
	fmt.Println("2")
	CheckIfError(err)

	// Get the current worktree.
	worktree, err := repo.Worktree()
	if err != nil {
		fmt.Printf("Error getting worktree: %v\n", err)
		return
	}

	// Get the root of the Git repository.
	// This will traverse parent directories until it finds the root.
	rootPath, err := findRoot(worktree)
	if err != nil {
		fmt.Printf("Error finding root: %v\n", err)
		return
	}

	fmt.Printf("Root of the Git repository: %s\n", rootPath)

	// Retrieve the branch's HEAD, to then get the HEAD commit.
	ref, err := repo.Head()
	CheckIfError(err)
	fmt.Println("3")

	c, err := repo.CommitObject(ref.Hash())
	CheckIfError(err)

	// Convert the relative path to an absolute path
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		fmt.Printf("Error converting relative path to absolute path: %v\n", err)
		return
	}
	Info("git blame %s", path)

	gitPath := strings.ReplaceAll(absolutePath, rootPath, "")
	gitPath = strings.Trim(gitPath, "/ \t\n")
	fmt.Printf("%s", c)
	fmt.Println(gitPath)
	// Blame the given file/path.
	br, err := git.Blame(c, gitPath)
	fmt.Println(err)
	CheckIfError(err)

	fmt.Println("6")
	fmt.Printf("%s", br.String())
}
