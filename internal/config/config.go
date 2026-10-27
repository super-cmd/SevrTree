// Package config 负责集中加载、校验并规范化服务端运行配置。
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config 是服务端运行所需的全部基础配置。
// 敏感项（管理员口令、监听地址、数据目录）一律优先从环境变量读取。
type Config struct {
	Addr       string        // HTTP 监听地址
	DataDir    string        // 数据目录
	LogDir     string        // 日志目录
	DBPath     string        // SQLite 数据库文件
	TaskLogDir string        // 受管任务输出日志目录
	SessionTTL time.Duration // 会话有效期
	Version    string        // 构建版本
}

// Load 读取环境变量并生成配置，未设置的项使用可用的默认值。
func Load(version string) *Config {
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}
	exeDir := filepath.Dir(exe)

	dataDir := envString("SEVRTREE_DATA_DIR", filepath.Join(exeDir, "data"))
	logDir := envString("SEVRTREE_LOG_DIR", filepath.Join(dataDir, "logs"))

	return &Config{
		Addr:       envString("SEVRTREE_ADDR", "127.0.0.1:8088"),
		DataDir:    dataDir,
		LogDir:     logDir,
		DBPath:     filepath.Join(dataDir, "sevrtree.db"),
		TaskLogDir: filepath.Join(dataDir, "tasklogs"),
		SessionTTL: time.Duration(envInt("SEVRTREE_SESSION_MINUTES", 720)) * time.Minute,
		Version:    version,
	}
}

// EnsureDirs 创建运行期必需的目录。
func (c *Config) EnsureDirs() error {
	for _, dir := range []string{c.DataDir, c.LogDir, c.TaskLogDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return errors.New("创建目录失败: " + dir + ": " + err.Error())
		}
	}
	return nil
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}
