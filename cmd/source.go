package cmd

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamnikolie/dziga/internal/config"
)

// Source is a resolved input clip: a local file, possibly fetched first.
type Source struct {
	Arg        string // what the user passed
	Path       string // absolute local path
	Stem       string // filename without extension
	FromURL    bool
	Downloaded bool
}

// resolveSource accepts a local path or a URL. URLs are fetched with yt-dlp into
// ~/.dziga/cache/<sha1(url)>.<ext> and reused on later calls — an agent asking
// five questions about one reel should download it once.
func resolveSource(ctx context.Context, arg string) (*Source, error) {
	if isURL(arg) {
		path, cached, err := fetchURL(ctx, arg)
		if err != nil {
			return nil, err
		}
		return &Source{Arg: arg, Path: path, Stem: stemOf(path), FromURL: true, Downloaded: !cached}, nil
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("source: %s is a directory", abs)
	}
	return &Source{Arg: arg, Path: abs, Stem: stemOf(abs)}, nil
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func stemOf(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func hash8(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:8]
}

func fetchURL(ctx context.Context, url string) (string, bool, error) {
	cacheDir, err := config.SubDir("cache")
	if err != nil {
		return "", false, err
	}
	key := hash8(url)

	if hits, _ := filepath.Glob(filepath.Join(cacheDir, key+".*")); len(hits) > 0 {
		sort.Strings(hits)
		return hits[0], true, nil
	}

	bin, err := exec.LookPath("yt-dlp")
	if err != nil {
		return "", false, fmt.Errorf("source: %q is a URL but yt-dlp is not on PATH (pip install yt-dlp), or pass a local file", url)
	}
	statusf("fetch %s → %s", url, cacheDir)
	out := filepath.Join(cacheDir, key+".%(ext)s")
	cmd := exec.CommandContext(ctx, bin,
		"--no-playlist",
		"--no-progress",
		"-f", "bv*[ext=mp4]+ba[ext=m4a]/b[ext=mp4]/b",
		"--merge-output-format", "mp4",
		"-o", out, url)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", false, fmt.Errorf("source: yt-dlp: %w: %s", err, strings.TrimSpace(string(b)))
	}
	hits, _ := filepath.Glob(filepath.Join(cacheDir, key+".*"))
	if len(hits) == 0 {
		return "", false, fmt.Errorf("source: yt-dlp wrote nothing for %s", url)
	}
	sort.Strings(hits)
	return hits[0], false, nil
}

// workDir returns the directory artifacts for this source go into. The default
// is a per-video dir under ~/.dziga/work/ — an agent tool called from a repo
// must not scatter jpegs across it. --out overrides.
func workDir(src *Source) (string, error) {
	if outDir != "" {
		abs, err := filepath.Abs(outDir)
		if err != nil {
			return "", fmt.Errorf("workDir: %w", err)
		}
		if err := os.MkdirAll(abs, 0755); err != nil {
			return "", fmt.Errorf("workDir: %w", err)
		}
		return abs, nil
	}
	base, err := config.SubDir("work")
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, sanitize(src.Stem)+"-"+hash8(src.Path))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("workDir: %w", err)
	}
	return dir, nil
}

// sanitize keeps a filename stem safe and short.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('-')
		default:
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteRune('-')
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "clip"
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}
