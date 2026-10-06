package filemanager

import "os"

type fileRoot = os.Root

func openFileRoot(root string) (*fileRoot, error) { return os.OpenRoot(root) }
