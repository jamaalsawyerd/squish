package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jamaalsawyerd/squish/internal/codec"
)

// Suffix marks squish's output files. Directory scans skip anything with it, so
// running squish twice does not compress its own output.
const Suffix = "-squished"

// nameClaims stops two workers choosing the same output name, which could
// happen with photo.png and photo.webp both becoming photo-squished.webp.
type nameClaims struct {
	mu    sync.Mutex
	taken map[string]bool
}

func (n *nameClaims) claim(path string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.taken == nil {
		n.taken = map[string]bool{}
	}
	if n.taken[path] {
		return false
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	n.taken[path] = true
	return true
}

// outputPath picks a name that is not in use. It never picks an existing file.
func outputPath(input string, f codec.Format, o Options, names *nameClaims) string {
	dir, base := filepath.Split(input)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	suffix := Suffix
	if o.OutDir != "" {
		dir, suffix = o.OutDir, ""
	}
	if o.InPlace {
		suffix = ""
	}
	for i := 1; ; i++ {
		name := stem + suffix + f.Ext()
		if i > 1 {
			name = fmt.Sprintf("%s%s-%d%s", stem, suffix, i, f.Ext())
		}
		p := filepath.Join(dir, name)
		if names.claim(p) {
			return p
		}
	}
}

// writeOutput writes via a temp file and rename, so a crash can never leave a
// half written image, and in-place mode can never damage an original.
func writeOutput(input string, st fs.FileInfo, out Output, o Options, names *nameClaims) (string, error) {
	replace := o.InPlace && codec.Sniff(out.Data) == formatOfFile(input)
	var dst string
	if replace {
		dst = input
	} else {
		dst = outputPath(input, out.Format, o, names)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".squish-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // Fails harmlessly once renamed.
	if _, err := tmp.Write(out.Data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), st.Mode().Perm()); err != nil {
		return "", err
	}
	// Keeping the date means photos stay in order in galleries and file lists.
	if err := os.Chtimes(tmp.Name(), st.ModTime(), st.ModTime()); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return "", err
	}
	return dst, nil
}

func formatOfFile(path string) codec.Format {
	f, err := os.Open(path)
	if err != nil {
		return codec.Unknown
	}
	defer f.Close()
	head := make([]byte, 16)
	n, _ := f.Read(head)
	return codec.Sniff(head[:n])
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".avif": true, ".gif": true,
}

// IsImageName reports whether a file name has a supported image extension.
func IsImageName(name string) bool {
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

func isOwnOutput(name string) bool {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if strings.HasSuffix(stem, Suffix) {
		return true
	}
	i := strings.LastIndex(stem, Suffix+"-")
	if i < 0 {
		return false
	}
	for _, c := range stem[i+len(Suffix)+1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Expand turns files and directories into a list of images. Files named
// directly are always included, since the user asked for them. Scanned
// directories contribute only images, skipping hidden files and earlier output.
func Expand(paths []string, recursive bool) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			add(p)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				if path != p && (!recursive || strings.HasPrefix(name, ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasPrefix(name, ".") && IsImageName(name) && !isOwnOutput(name) {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
