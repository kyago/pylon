package corpus

import (
	"io/fs"
	"os"
)

func openFile(path string) (fs.File, error) {
	return os.Open(path)
}
