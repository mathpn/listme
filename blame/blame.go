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
	"time"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("listme")

// Maximum length for the Git author string
const MaxAuthorLength = 20

// LineBlame contains Git blame information for a specific file line.
//   - Time: date and time of commit
//   - Author: author name
type LineBlame struct {
	Time   time.Time
	Author string
}

type GitBlame struct {
	blames []*LineBlame
}

// BlameLine returns a LineBlame for the specified line if possible.
// If the line is out of range, an error is returned.
func (b *GitBlame) BlameLine(line int) (*LineBlame, error) {
	line = line - 1
	if line < 0 || line >= len(b.blames) {
		err := fmt.Errorf("line %d out of range", line)
		log.Info(err)
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
				time := time.Unix(ts, 0)
				if err == nil {
					currentBlame.Time = time
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

// BlameFile runs git blame for the provided path using the OS interface,
// parses the output and returns a *GitBlame or error.
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
		err = fmt.Errorf("git blame failed: %v - %s", err, stderr.String())
		log.Debug(err)
		return nil, err
	}

	return &GitBlame{blames: blames}, nil
}
