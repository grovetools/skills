package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// syncManifestFileName is the dot-file written into each synced skill
// directory recording a hash of the source contents at sync time. It lets
// subsequent syncs skip the RemoveAll+CopyDir when the source is unchanged.
const syncManifestFileName = ".grove-sync-manifest"

// computeSkillManifest returns a hash identifying the current content state
// of a resolved skill's source. Disk sources hash relpath+size+mtime of every
// file (no content reads); builtin sources hash relpath+content of the
// embedded files (cheap — they are already in memory, and mtimes do not exist
// in the embedded FS).
func computeSkillManifest(r ResolvedSkill) (string, error) {
	h := sha256.New()

	if r.SourceType == SourceTypeBuiltin {
		root := filepath.Join("data/skills", r.RelPath)
		var paths []string
		err := fs.WalkDir(embeddedSkillsFS, root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return "", err
		}
		sort.Strings(paths)
		for _, p := range paths {
			content, err := fs.ReadFile(embeddedSkillsFS, p)
			if err != nil {
				return "", err
			}
			rel, _ := filepath.Rel(root, p)
			fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(content))
			h.Write(content)
			h.Write([]byte{'\n'})
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}

	type fileStamp struct {
		rel   string
		size  int64
		mtime int64
	}
	var stamps []fileStamp
	err := filepath.WalkDir(r.PhysicalPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(r.PhysicalPath, path)
		stamps = append(stamps, fileStamp{filepath.ToSlash(rel), info.Size(), info.ModTime().UnixNano()})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i].rel < stamps[j].rel })
	for _, s := range stamps {
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", s.rel, s.size, s.mtime)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// computeSkillManifests computes manifests for a resolved skill set. Skills
// whose manifest cannot be computed map to "" — an empty manifest never
// matches a stored one, so those skills are always re-synced.
func computeSkillManifests(resolved map[string]ResolvedSkill) map[string]string {
	manifests := make(map[string]string, len(resolved))
	for name, r := range resolved {
		m, err := computeSkillManifest(r)
		if err != nil {
			m = ""
		}
		manifests[name] = m
	}
	return manifests
}

// readStoredManifest returns the manifest recorded in a synced skill
// directory, or "" if none exists.
func readStoredManifest(destPath string) string {
	data, err := os.ReadFile(filepath.Join(destPath, syncManifestFileName)) //nolint:gosec // G304: derived skill dir
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// writeStoredManifest records the source manifest in a synced skill
// directory. Best-effort: a failed write only means the next sync re-copies.
func writeStoredManifest(destPath, manifest string) {
	if manifest == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(destPath, syncManifestFileName), []byte(manifest+"\n"), 0o644) //nolint:gosec // G306: marker file
}
