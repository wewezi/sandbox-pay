// Mintweft —— 一个跑在 2c2G 小机器上的支付沙箱网关。
//
// 全部逻辑在 internal/ 下：config/logger/money/signature/apiurl/jwt/
// httperr/httpclient/validate/db/httpx/services/gateways/routes。
// 这个文件只做四件事：读 .env、开库、装配、起 HTTP 服务（含优雅退出）。
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httpx"
	"sandbox-pay/internal/logger"
	"sandbox-pay/internal/routes"
	"sandbox-pay/internal/services"
)

// 构建后的前端打进二进制，部署时不用另外安装 Node 或携带静态文件。
//
//go:generate npm run build
//go:embed all:web/dist
var frontend embed.FS

func main() {
	// create-admin 是脚本不是服务：读 .env → 开库 → 建/改用户 → 退出。
	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		os.Exit(runCreateAdmin(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "seed-demo" {
		os.Exit(runSeedDemo(os.Args[2:]))
	}
	os.Exit(runServer())
}

// runServer 起 HTTP 服务，收到 SIGINT/SIGTERM 优雅退出。
func runServer() int {
	// 已存在的环境变量优先 —— 便于 systemd/docker 注入而不被仓库里的 .env 覆盖。
	config.LoadEnv(".env")

	log := logger.New("Bootstrap")

	cfg := config.EnvConfig{}
	if err := config.Validate(cfg); err != nil {
		log.Error("Invalid configuration: " + err.Error())
		return 1
	}

	packageRoot, err := os.Getwd()
	if err != nil {
		log.Error(fmt.Sprintf("无法确定工作目录: %s", err))
		return 1
	}

	store, err := db.Open(cfg.Get("DB_FILE"), packageRoot)
	if err != nil {
		log.Error(fmt.Sprintf("打开数据库失败: %s", err))
		return 1
	}
	defer store.Close()

	// 建表 + 确保分组存在（原版是 GroupService 的 onModuleInit）
	container := routes.NewContainer(cfg, store)
	if err := container.Groups.Init(); err != nil {
		log.Error(fmt.Sprintf("初始化分组失败: %s", err))
		return 1
	}

	app := routes.New(cfg, container, staticHandler())
	stop := startScheduledTasks(container)
	defer stop()

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "18080"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           app,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Log(fmt.Sprintf("Mintweft listening on http://localhost:%s", port))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error(fmt.Sprintf("HTTP 服务异常退出: %s", err))
			os.Exit(1)
		}
	}()

	// 优雅退出：先停止接新连接，再关定时器和数据库。
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Log(fmt.Sprintf("%s received, shutting down", sig))

	// 兜底：5 秒还没关干净就强退，别挂在那。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Error(fmt.Sprintf("关闭 HTTP 服务超时: %s", err))
	}
	stop()
	return 0
}

// staticHandler 优先读取本地构建产物，否则使用二进制内嵌资源。
func staticHandler() http.Handler {
	if fsys := routes.DiskFS("web/dist"); fsys != nil {
		return routes.Static(fsys)
	}
	return routes.Static(routes.SubFS(frontend, "web/dist"))
}

// startScheduledTasks 后台定时任务：每秒处理通知和沙箱任务，每分钟扫过期订单和运行维护。
//
// time.Ticker 就够了：单进程、任务只是扫表，没有 cron 表达式的需求。
func startScheduledTasks(c *routes.Container) (stop func()) {
	log := logger.New("ScheduledTasks")

	expire := time.NewTicker(time.Minute)
	notify := time.NewTicker(time.Second)
	sandbox := time.NewTicker(time.Second)
	operations := time.NewTicker(time.Minute)
	// 限流桶 5 分钟清一次（窗口最长 1 分钟，留足余量）
	limiter := time.NewTicker(5 * time.Minute)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-sandbox.C:
				if config.Bool(c.Config, "DEMO_DATA_MODE") {
					continue
				}
				err := c.RunSandboxTasks()
				message := ""
				if err != nil {
					message = "场景任务执行失败"
					log.Error(message)
				}
				_ = c.Store.RecordOperationRun("sandbox", message)
			case <-done:
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case now := <-operations.C:
				if config.Bool(c.Config, "DEMO_DATA_MODE") {
					continue
				}
				if err := c.Operations.RunSchedule(now); err != nil {
					log.Error("运行维护任务失败：" + err.Error())
				}
			case <-done:
				return
			}
		}
	}()

	go func() {
		for {
			select {
			case <-expire.C:
				if config.Bool(c.Config, "DEMO_DATA_MODE") {
					continue
				}
				closed, err := c.Store.Orders.ExpirePending(1000)
				message := ""
				if err != nil {
					message = "订单过期处理失败"
				}
				_ = c.Store.RecordOperationRun("expiration", message)
				if err != nil {
					log.Error(fmt.Sprintf("Order cleanup failed: %s", err))
				} else if closed > 0 {
					log.Log(fmt.Sprintf("Closed %d expired orders", closed))
				}
			case <-notify.C:
				if config.Bool(c.Config, "DEMO_DATA_MODE") {
					continue
				}
				err := c.Notify.HandleNotifications()
				message := ""
				if err != nil {
					message = "回调投递任务失败"
				}
				_ = c.Store.RecordOperationRun("notifications", message)
				if err != nil {
					log.Error(fmt.Sprintf("Notification dispatch failed: %s", err))
				}
			case <-limiter.C:
				httpx.DefaultLimiter.Cleanup()
			case <-done:
				return
			}
		}
	}()

	var stopped bool
	return func() {
		if stopped {
			return
		}
		stopped = true
		expire.Stop()
		notify.Stop()
		sandbox.Stop()
		operations.Stop()
		limiter.Stop()
		close(done)
	}
}

// runSeedDemo 在停止服务后清空数据库记录并重建关联演示数据。
func runSeedDemo(args []string) int {
	if len(args) != 1 || args[0] != "--replace" {
		fmt.Fprintln(os.Stderr, "Usage: mintweft seed-demo --replace\n此命令清空数据库记录并重建关联演示数据，请先停止服务。")
		return 1
	}
	config.LoadEnv(".env")
	cfg := config.EnvConfig{}
	if !config.Bool(cfg, "ENABLE_SANDBOX") {
		fmt.Fprintln(os.Stderr, "演示数据仅允许在 ENABLE_SANDBOX=true 时生成。")
		return 1
	}
	packageRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	store, err := db.Open(cfg.Get("DB_FILE"), packageRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer store.Close()
	summary, err := services.SeedDemo(store, cfg, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成演示数据失败：", err)
		return 1
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// runCreateAdmin 创建（或重置密码）一个管理员账号。
//
//	go run . create-admin <username> <password> [role]
//	ADMIN_USERNAME=.. ADMIN_PASSWORD=.. go run . create-admin
//
// role 默认 SuperAdmin。密码永不打印。
func runCreateAdmin(args []string) int {
	config.LoadEnv(".env")
	cfg := config.EnvConfig{}

	username := pickArg(args, 0, os.Getenv("ADMIN_USERNAME"))
	password := pickArg(args, 1, os.Getenv("ADMIN_PASSWORD"))
	role := pickArg(args, 2, os.Getenv("ADMIN_ROLE"))
	if role == "" {
		role = db.RoleSuperAdmin
	}

	if username == "" || password == "" {
		fmt.Fprintln(os.Stderr,
			"Usage: mintweft create-admin <username> <password> [role]\n"+
				"   or: ADMIN_USERNAME=.. ADMIN_PASSWORD=.. mintweft create-admin")
		return 1
	}
	if len(username) < 3 || len(username) > 20 {
		fmt.Fprintln(os.Stderr, "username must be 3-20 characters")
		return 1
	}
	if len(password) < 6 || len(password) > 72 {
		fmt.Fprintln(os.Stderr, "password must be 6-72 characters")
		return 1
	}
	if role != db.RoleAdmin && role != db.RoleSuperAdmin {
		role = db.RoleSuperAdmin
	}

	packageRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// Open 会顺手把表建好
	store, err := db.Open(cfg.Get("DB_FILE"), packageRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer store.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	user, err := store.Users.FindByUsername(username)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if user != nil {
		err = store.Users.UpdateCredentials(username, string(hash), role)
	} else {
		err = store.Users.Insert(username, string(hash), role)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if user != nil {
		fmt.Printf("Updated existing user %q (role=%s).\n", username, role)
	} else {
		fmt.Printf("Created user %q (role=%s).\n", username, role)
	}
	return 0
}

// pickArg 取第 i 个命令行参数，空串回落到环境变量。
func pickArg(args []string, i int, fallback string) string {
	if i < len(args) && args[i] != "" {
		return args[i]
	}
	return fallback
}
