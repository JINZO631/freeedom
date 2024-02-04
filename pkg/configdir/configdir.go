package configdir

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetConfigDir 設定ファイル用のディレクトリを取得する (Windows: %APPDATA%/freeedom, Linux/macOS: ~/.config/freeedom)
func GetConfigDir() (string, error) {
	var configDir string
	if runtime.GOOS == "windows" {
		// Windowsの場合
		configDir = filepath.Join(os.Getenv("APPDATA"), "freeedom")
	} else {
		// LinuxとmacOSの場合
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, ".config", "freeedom")
	}

	return configDir, nil
}
