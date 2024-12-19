package matcher

import (
	"path/filepath"
	"testing"
)

func BenchmarkGit(b *testing.B) {
	b.Run("git_open", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			fpath, err := filepath.Abs(".")
			if err != nil {
				panic(err)
			}
			_, err = detectDotGit(fpath)
			if err != nil {
				panic(err)
			}
		}
	})
}
