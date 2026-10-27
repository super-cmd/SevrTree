// SevrTree 进程依赖与启动管理服务。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"sevrtree/internal/api"
	"sevrtree/internal/auth"
	"sevrtree/internal/config"
	"sevrtree/internal/logging"
	"sevrtree/internal/manager"
	"sevrtree/internal/procmon"
	"sevrtree/internal/store"
	"sevrtree/web"
)

var (
	version   = "0.1.0"
	buildTime = "dev"
)

func main() {
	addrFlag := flag.String("addr", "", "HTTP 监听地址，例如 127.0.0.1:8088")
	debugFlag := flag.Bool("debug", false, "输出调试日志")
	versionFlag := flag.Bool("version", false, "打印版本信息后退出")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("SevrTree %s (%s)\n", version, buildTime)
		return
	}

	cfg := config.Load(version)
	if *addrFlag != "" {
		cfg.Addr = *addrFlag
	}
	if err := cfg.EnsureDirs(); err != nil {
		fmt.Fprintln(os.Stderr, "初始化目录失败:", err)
		os.Exit(1)
	}

	logger, closeLog, err := logging.New(filepath.Join(cfg.LogDir, "sevrtree.log"), *debugFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化日志失败:", err)
		os.Exit(1)
	}
	defer func() { _ = closeLog() }()
	slog.SetDefault(logger)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Error("打开数据库失败", "error", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()

	authSvc, initialPassword, err := auth.NewService(st, cfg.SessionTTL, logger)
	if err != nil {
		logger.Error("初始化认证失败", "error", err)
		os.Exit(1)
	}

	mgr := manager.New(st, cfg.TaskLogDir, logger)
	collector := procmon.New(mgr.ManagedPIDs, logger)
	server := api.New(cfg, st, authSvc, mgr, collector, logger, web.Dist)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	printBanner(cfg, initialPassword)

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	logger.Info("服务已启动", "addr", "http://"+cfg.Addr, "version", version, "build", buildTime,
		"dataDir", cfg.DataDir, "logDir", cfg.LogDir)

	if os.Getenv("SEVRTREE_AUTOBOOT") != "0" {
		if err := mgr.StartAllAsync(true); err != nil {
			logger.Warn("开机任务编排未启动", "error", err)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	select {
	case err := <-errCh:
		logger.Error("HTTP 服务异常退出", "error", err)
	case sig := <-sigCh:
		logger.Info("收到退出信号", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Warn("关闭 HTTP 服务超时", "error", err)
	}
	mgr.Shutdown(10 * time.Second)
	logger.Info("已退出")
}

func printBanner(cfg *config.Config, initialPassword string) {
	fmt.Println("========================================================")
	fmt.Println("  SevrTree 进程依赖与启动管理")
	fmt.Printf("  版本: %s (%s)\n", version, buildTime)
	fmt.Printf("  访问: http://%s\n", cfg.Addr)
	fmt.Printf("  数据: %s\n", cfg.DataDir)
	fmt.Println("========================================================")
	if initialPassword == "" {
		return
	}

	passwordFile := filepath.Join(cfg.DataDir, "initial-admin-password.txt")
	_ = os.WriteFile(passwordFile, []byte(initialPassword+"\n"), 0o600)

	fmt.Println("  首次启动，已生成管理员账号：admin")
	fmt.Printf("  初始口令: %s\n", initialPassword)
	fmt.Printf("  口令已保存到: %s\n", passwordFile)
	fmt.Println("  登录后请立即在「设置」中修改口令")
	fmt.Println("========================================================")
}
