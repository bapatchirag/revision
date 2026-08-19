package shelf

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Save writes an entry into the store at dir and returns it as stored, with the
// identifier and creation time it was given if it arrived without them.
//
// The entry is assembled in a temporary directory and renamed into place, so a
// write that fails part way through leaves no entry rather than half of one.
// That matters more here than for most stores: the caller reverts the working
// copy once this returns, and until it does the shelved work exists nowhere else.
func Save(dir string, e Entry, patch string, untracked []Payload) (Entry, error) {
	if e.ID == "" {
		e.ID = NewID(time.Now())
	}
	if err := validID(e.ID); err != nil {
		return Entry{}, err
	}
	if e.Created.IsZero() {
		e.Created = time.Now()
	}
	e.Version = formatVersion

	payloads := make([]Payload, 0, len(untracked))
	rels := make([]string, 0, len(untracked))
	for _, p := range untracked {
		rel, err := safeRel(p.Rel)
		if err != nil {
			return Entry{}, err
		}
		payloads = append(payloads, Payload{Rel: rel, Src: p.Src})
		rels = append(rels, rel)
	}
	if len(rels) > 0 {
		e.Untracked = rels
	}

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return Entry{}, fmt.Errorf("shelf: create store %s: %w", dir, err)
	}
	target := filepath.Join(dir, e.ID)
	if _, err := os.Stat(target); err == nil {
		return Entry{}, fmt.Errorf("shelf: entry %s already exists", e.ID)
	}

	tmp, err := os.MkdirTemp(dir, tmpPrefix+"*")
	if err != nil {
		return Entry{}, fmt.Errorf("shelf: create temp entry: %w", err)
	}
	// Best-effort cleanup if we return before the rename; afterwards the temp
	// directory no longer exists and RemoveAll is a no-op.
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := os.WriteFile(filepath.Join(tmp, patchFile), []byte(patch), filePerm); err != nil {
		return Entry{}, fmt.Errorf("shelf: write patch: %w", err)
	}
	for _, p := range payloads {
		if err := copyFile(p.Src, filepath.Join(tmp, untrackedDir, filepath.FromSlash(p.Rel))); err != nil {
			return Entry{}, err
		}
	}
	if err := writeMeta(tmp, e); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(tmp, target); err != nil {
		return Entry{}, fmt.Errorf("shelf: store entry %s: %w", e.ID, err)
	}
	return e, nil
}

// Scan lists the entries in the store at dir, newest first. A store that does
// not exist yet holds no entries, which is not an error — nothing has been
// shelved there. An entry whose metadata cannot be read is passed over rather
// than failing the scan, so one unreadable directory does not hide the rest.
func Scan(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("shelf: read store %s: %w", dir, err)
	}
	out := make([]Entry, 0, len(des))
	for _, de := range des {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		e, err := readMeta(filepath.Join(dir, de.Name()))
		if err != nil || e.Version > formatVersion {
			continue
		}
		// The directory name is the entry's identity, so a hand-edited meta.json
		// cannot point Drop or ReadPatch at a different entry.
		e.ID = de.Name()
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Created.Equal(out[j].Created) {
			return out[i].ID > out[j].ID
		}
		return out[i].Created.After(out[j].Created)
	})
	return out, nil
}

// ReadPatch returns the patch stored for an entry.
func ReadPatch(dir, id string) (string, error) {
	if err := validID(id); err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, id, patchFile))
	if err != nil {
		return "", fmt.Errorf("shelf: read patch for %s: %w", id, err)
	}
	return string(data), nil
}

// UntrackedDir returns the directory an entry's unversioned payloads were copied
// into, rooted so that a path from Entry.Untracked joins onto it directly.
func UntrackedDir(dir, id string) (string, error) {
	if err := validID(id); err != nil {
		return "", err
	}
	return filepath.Join(dir, id, untrackedDir), nil
}

// PatchPath returns the file an entry's patch is stored in, for handing to a
// tool that reads a patch from disk rather than from memory.
func PatchPath(dir, id string) (string, error) {
	if err := validID(id); err != nil {
		return "", err
	}
	return filepath.Join(dir, id, patchFile), nil
}

// Restore copies an entry's unversioned payloads back into a working copy at
// root, returning what it put back and what it would not.
//
// A payload whose path is already taken is reported rather than written: the
// file there now is somebody's current work, and no shelf is worth overwriting
// it unasked. The entry keeps its copy either way, so a blocked payload can
// still be recovered by hand.
func Restore(dir, id, root string) (restored, blocked []string, err error) {
	e, err := readMeta(filepath.Join(dir, id))
	if err != nil {
		return nil, nil, err
	}
	payloads, err := UntrackedDir(dir, id)
	if err != nil {
		return nil, nil, err
	}
	for _, rel := range e.Untracked {
		clean, err := safeRel(rel)
		if err != nil {
			return restored, blocked, err
		}
		dst := filepath.Join(root, filepath.FromSlash(clean))
		if _, err := os.Lstat(dst); err == nil {
			blocked = append(blocked, clean)
			continue
		}
		if err := copyFile(filepath.Join(payloads, filepath.FromSlash(clean)), dst); err != nil {
			return restored, blocked, err
		}
		restored = append(restored, clean)
	}
	return restored, blocked, nil
}

// Drop removes an entry and everything it holds.
func Drop(dir, id string) error {
	if err := validID(id); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(dir, id)); err != nil {
		return fmt.Errorf("shelf: drop entry %s: %w", id, err)
	}
	return nil
}

// Rename relabels an entry, leaving everything it captured untouched.
func Rename(dir, id, name string) error {
	if err := validID(id); err != nil {
		return err
	}
	entryDir := filepath.Join(dir, id)
	e, err := readMeta(entryDir)
	if err != nil {
		return err
	}
	e.ID = id
	e.Name = strings.TrimSpace(name)
	return writeMeta(entryDir, e)
}

// readMeta loads one entry's metadata.
func readMeta(entryDir string) (Entry, error) {
	data, err := os.ReadFile(filepath.Join(entryDir, metaFile))
	if err != nil {
		return Entry{}, fmt.Errorf("shelf: read metadata in %s: %w", entryDir, err)
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return Entry{}, fmt.Errorf("shelf: parse metadata in %s: %w", entryDir, err)
	}
	return e, nil
}

// writeMeta stores one entry's metadata. The write goes to a temporary file in
// the same directory and is renamed into place, so a relabel interrupted part
// way through cannot truncate the metadata of an entry that already exists.
func writeMeta(entryDir string, e Entry) error {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("shelf: encode metadata for %s: %w", e.ID, err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(entryDir, metaFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("shelf: create temp metadata: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("shelf: write temp metadata: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("shelf: close temp metadata: %w", err)
	}
	if err := os.Chmod(tmpName, filePerm); err != nil {
		return fmt.Errorf("shelf: chmod temp metadata: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(entryDir, metaFile)); err != nil {
		return fmt.Errorf("shelf: replace metadata for %s: %w", e.ID, err)
	}
	return nil
}

// copyFile duplicates a payload into the entry, keeping the source's permission
// bits so a shelved executable is still executable when it is put back. Only
// regular files are taken: a symlink would otherwise be followed out of the
// working copy and stored as whatever it happened to point at.
func copyFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("shelf: stat payload %s: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("shelf: payload %s is not a regular file", src)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("shelf: open payload %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return fmt.Errorf("shelf: create payload dir: %w", err)
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("shelf: create payload %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("shelf: copy payload %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("shelf: close payload %s: %w", dst, err)
	}
	return nil
}
