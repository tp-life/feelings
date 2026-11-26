package configs

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed strategies/*.yaml styles.yaml prompts/*.tmpl
var EmbeddedFS embed.FS

// ReadFile 优先读取磁盘 overrideDir 下的文件（支持用户在本地/Web Prompt Studio 中实时编辑），
// 若磁盘文件不存在则回退到二进制内嵌的默认配置。
func ReadFile(overrideDir, relPath string) ([]byte, error) {
	if overrideDir != "" {
		diskPath := filepath.Join(overrideDir, filepath.FromSlash(relPath))
		if data, err := os.ReadFile(diskPath); err == nil {
			return data, nil
		}
	}
	return fs.ReadFile(EmbeddedFS, relPath)
}

// WriteOverride 将用户修改的配置/提示词模板保存到磁盘目录。
func WriteOverride(overrideDir, relPath string, data []byte) error {
	if overrideDir == "" {
		overrideDir = "configs"
	}
	diskPath := filepath.Join(overrideDir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(diskPath, data, 0o644)
}

// ListDir 列出内嵌与磁盘覆盖目录合并后的相对路径（去重）。
func ListDir(overrideDir, dir string) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string

	if entries, err := fs.ReadDir(EmbeddedFS, dir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				p := dir + "/" + e.Name()
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
	}
	if overrideDir != "" {
		if entries, err := os.ReadDir(filepath.Join(overrideDir, filepath.FromSlash(dir))); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					p := dir + "/" + e.Name()
					if _, ok := seen[p]; !ok {
						seen[p] = struct{}{}
						out = append(out, p)
					}
				}
			}
		}
	}
	return out, nil
}
