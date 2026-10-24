package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	_ "golang.org/x/crypto/x509roots/fallback"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/niaotuobang/niaotuobang-wiki/internal/runtimeconfig"
	"github.com/niaotuobang/niaotuobang-wiki/internal/webui"
	"github.com/niaotuobang/niaotuobang-wiki/internal/wiki"
)

func env(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
func main() {
	if e := run(); e != nil {
		log.Fatal(e)
	}
}
func run() error {
	command := "serve"
	args := os.Args[1:]
	if len(args) > 0 && args[0][0] != '-' {
		command = args[0]
		args = args[1:]
	}
	path, e := runtimeconfig.EnvFileArgument(args)
	if e != nil {
		return e
	}
	if path != "" {
		if e = runtimeconfig.LoadEnvFile(path); e != nil {
			return e
		}
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.String("env-file", path, "explicit private environment file")
	userID := flags.String("user-id", "", "existing user ID for explicit local recovery or promotion")
	dir := flags.String("data-dir", env("DATA_DIR", "./data"), "persistent data directory")
	listen := flags.String("listen", env("LISTEN_ADDR", "127.0.0.1:8080"), "listen address")
	demo := flags.Bool("demo", false, "local fictional demo; loopback only")
	output := flags.String("output", "./backups", "backup output directory")
	input := flags.String("input", "", "restore or import input file")
	apply := flags.Bool("apply", false, "apply a validated Matterwiki import; otherwise preview")
	primaryLegacyID := flags.String("primary-admin-id", "", "explicit legacy account ID for initial administrator migration")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if command == "restore" {
		if *input == "" {
			return fmt.Errorf("--input is required")
		}
		return wiki.RestoreBackup(*input, *dir)
	}
	if command == "healthcheck" {
		_, port, e := net.SplitHostPort(*listen)
		if e != nil {
			return e
		}
		client := http.Client{Timeout: 3 * time.Second}
		response, e := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/readyz")
		if e != nil {
			return e
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("wiki is not ready")
		}
		return nil
	}
	cfg := wiki.Config{DataDir: *dir, Demo: *demo, BaseURL: os.Getenv("BASE_URL"), UI: webui.Files()}
	if *demo {
		host, _, e := net.SplitHostPort(*listen)
		if e != nil || host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
			return fmt.Errorf("demo mode requires a loopback listen address")
		}
	}
	if env("STORAGE_DRIVER", "local") == "oss" {
		cfg.OSS = &wiki.OSSConfig{Endpoint: os.Getenv("OSS_ENDPOINT"), Region: os.Getenv("OSS_REGION"), Bucket: os.Getenv("OSS_BUCKET"), AccessKeyID: os.Getenv("OSS_ACCESS_KEY_ID"), AccessKeySecret: os.Getenv("OSS_ACCESS_KEY_SECRET"), SecurityToken: os.Getenv("OSS_SECURITY_TOKEN")}
	}
	if (command == "serve" || command == "import-matterwiki-accounts") && !cfg.Demo {
		var e error
		cfg.Auth, e = loadAuthConfig()
		if e != nil {
			return e
		}
	}
	app, e := wiki.Open(cfg)
	if e != nil {
		return e
	}
	defer app.Close()
	switch command {
	case "import-matterwiki-accounts":
		if *input == "" || *primaryLegacyID == "" {
			return fmt.Errorf("--input and --primary-admin-id are required")
		}
		report, err := app.ImportMatterwikiAccounts(*input, *primaryLegacyID, *apply)
		if err == nil {
			b, _ := json.Marshal(report)
			fmt.Println(string(b))
		}
		return err
	case "recover-system-admin":
		p, e := app.PrepareEmailRecovery(*userID, *output)
		if e == nil {
			fmt.Println(p)
		}
		return e
	case "promote-system-admin":
		return app.PromoteSystemAdmin(*userID)
	case "import-matterwiki":
		if *input == "" {
			return fmt.Errorf("--input is required")
		}
		report, err := app.ImportMatterwiki(*input, *apply)
		reportDir := filepath.Join(*dir, "imports")
		if e := os.MkdirAll(reportDir, 0700); e != nil {
			return e
		}
		reportJSON, _ := json.MarshalIndent(report, "", "  ")
		if e := os.WriteFile(filepath.Join(reportDir, "preview-report.json"), reportJSON, 0600); e != nil {
			return e
		}
		if err != nil {
			return err
		}
		fmt.Printf("articles=%d versions=%d media=%d skipped=%d applied=%t warnings=%d\n", report.Articles, report.Versions, report.Media, report.Skipped, report.Applied, len(report.Warnings))
		return nil
	case "export":
		return app.ExportFolder(*output)
	case "backup":
		p, e := app.Backup(*output)
		if e == nil {
			fmt.Println(p)
		}
		return e
	case "serve":
		if e = app.RequireSystemAdmin(); e != nil {
			return e
		}
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
	server := &http.Server{Addr: *listen, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.Maintain(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Printf("Wiki running at http://%s", *listen)
	if _, e := os.Stat(filepath.Join(*dir, "setup-token")); e == nil {
		log.Printf("first setup credential is in %s", filepath.Join(*dir, "setup-token"))
	}
	e = server.ListenAndServe()
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
