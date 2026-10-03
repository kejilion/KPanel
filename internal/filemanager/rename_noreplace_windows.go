//go:build windows

package filemanager

func renameNoReplaceRoot(root *fileRoot, oldVirtual, newVirtual string) error {
	return root.rename(rootName(oldVirtual), rootName(newVirtual), false)
}
