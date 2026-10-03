//go:build !windows

package filemanager

import (
	"context"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"os"
)

type fileRoot = os.Root

func (*Manager) platformCanonical(value string) (string, error) { return value, nil }

func openFileRoot(root string, _ map[string]string) (*fileRoot, error) { return os.OpenRoot(root) }
func platformNameValid(string) bool                                    { return true }
func platformVirtualRoot(string) bool                                  { return false }
func platformPathKey(value string) string                              { return value }
func (*Manager) listPlatformRoot(context.Context, string, ListOptions) (contract.FileDirectory, error, bool) {
	return contract.FileDirectory{}, nil, false
}
