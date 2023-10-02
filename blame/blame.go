package blame

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("listme")

const MaxAuthorLength = 20

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
		err := fmt.Errorf("line out of range")
		log.Debug(err)
		return nil, err
	}
	return b.blames[line], nil
}

func parseGitBlame(out io.Reader) []*LineBlame {
	var blames []*LineBlame
	lr := bufio.NewReader(out)
	s := bufio.NewScanner(lr)

	var currentBlame *LineBlame
	for s.Scan() {
		buf := s.Text()
		if strings.HasPrefix(buf, "author ") {
			if currentBlame != nil {
				blames = append(blames, currentBlame)
			}
			currentBlame = &LineBlame{
				Author: truncateName(strings.TrimPrefix(buf, "author "), MaxAuthorLength),
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
	return blames
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

	blames := parseGitBlame(stdout)
	if err := cmd.Wait(); err != nil {
		err = fmt.Errorf("git blame failed: %v\n%s", err, stderr.String())
		log.Debug(err)
		return nil, err
	}

	return &GitBlame{blames: blames}, nil
}
