package svn

import (
	"context"
	"os"
	"path/filepath"
)

// Delete schedules a versioned path for removal (svn delete --force PATH); the
// removal is finalized on the next commit. --force lets svn delete a path that
// still has local modifications instead of refusing.
func (c *Client) Delete(ctx context.Context, path string) error {
	_, err := c.run(ctx, "delete", "--force", path)
	return err
}

// RemoveUnversioned deletes an unversioned path from disk. Such a path is not
// tracked, so there is nothing for svn to schedule; it is simply removed. The
// path is resolved against the working-copy directory.
func (c *Client) RemoveUnversioned(path string) error {
	return os.RemoveAll(filepath.Join(c.Dir, path))
}
