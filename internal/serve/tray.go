//go:build windows

package serve

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var trayIconBytes []byte

var apiBase string

func RunTray(serverURL string, onExit func()) {
	apiBase = serverURL
	systray.Run(func() { setupTray() }, onExit)
}

func setupTray() {
	systray.SetIcon(trayIconBytes)
	systray.SetTitle("CI/CD")
	systray.SetTooltip("本地 CI/CD 工具")

	openItem := systray.AddMenuItem("🌐 打开浏览器", "")
	systray.AddSeparator()

	watchMenu := systray.AddMenuItem("👀 监听", "")
	watchAll := watchMenu.AddSubMenuItemCheckbox("全部项目", "", false)

	notifItem := systray.AddMenuItemCheckbox("🔔 系统通知", "", false)
	systray.AddSeparator()

	pipeMenu := systray.AddMenuItem("▶ 流水线", "")
	pipeAll := pipeMenu.AddSubMenuItem("跑全部项目", "")
	pipeAll.SetTooltip("执行所有已启用项目的完整流水线")
	pipeMenu.AddSubMenuItem("─", "").Disable()

	stepMenu := systray.AddMenuItem("⚙ 单步骤", "")
	stepMenu.AddSubMenuItem("─", "").Disable()
	systray.AddSeparator()

	urlItem := systray.AddMenuItem(apiBase, "")
	urlItem.Disable()
	systray.AddSeparator()

	quitItem := systray.AddMenuItem("⏹ 退出", "")

	// 延迟 2 秒加载项目列表，等待 HTTP 服务器就绪
	go func() {
		time.Sleep(2 * time.Second)
		loadProjectMenus(watchMenu, pipeMenu, stepMenu, watchAll, pipeAll)
	}()

	go func() {
		for {
			select {
			case <-openItem.ClickedCh:
				go openBrowser(apiBase)
			case <-watchAll.ClickedCh:
				go func() {
					if watchAll.Checked() {
						apiGet("/api/watch/stop")
						watchAll.Uncheck()
					} else {
						apiGet("/api/watch/start?project=all")
						watchAll.Check()
					}
				}()
			case <-notifItem.ClickedCh:
				if notifItem.Checked() {
					notifItem.Uncheck()
				} else {
					notifItem.Check()
				}
			case <-pipeAll.ClickedCh:
				pipeAll.SetTitle("⏳ 跑全部项目（运行中）")
				go func() {
					logMsg(apiGet("/api/pipeline/run-all"))
					pipeAll.SetTitle("跑全部项目")
				}()
			case <-quitItem.ClickedCh:
				systray.Quit()
				os.Exit(0)
				return
			}
		}
	}()

	// setupTray 执行后正常返回，不能 select{} 阻塞，
	// 否则 systray 内部无法完成菜单注册，托盘菜单不会出现。
}

func loadProjectMenus(watchMenu, pipeMenu, stepMenu *systray.MenuItem, watchAll, pipeAll *systray.MenuItem) {
	projects := fetchProjects()
	if len(projects) == 0 {
		return
	}
	for _, p := range projects {
		name := p["name"].(string)
		watchItem := watchMenu.AddSubMenuItemCheckbox("  "+name, "", false)
		go func(n string, item *systray.MenuItem) {
			for range item.ClickedCh {
				if item.Checked() { apiGet("/api/watch/stop?project=" + n); item.Uncheck() } else { apiGet("/api/watch/start?project=" + n); item.Check() }
			}
		}(name, watchItem)
		pipeItem := pipeMenu.AddSubMenuItem("  ▶ "+name, "")
		pipeItem.SetTooltip("执行 " + name + " 的完整流水线")
		go func(n string, item *systray.MenuItem) {
			for range item.ClickedCh {
				trayLog(n, "pipeline", "info", "开始执行流水线")
				item.SetTitle("  ⏳ " + n + "（运行中）")
				go func(it *systray.MenuItem, projName string) {
					resp := apiGet("/api/pipeline/run?project=" + projName)
					title := "  ▶ " + projName
					status := "pass"
					errMsg := ""
					if strings.Contains(resp, `"status":"ok"`) {
						title = "  ✅ " + projName
					} else {
						title = "  ❌ " + projName
						status = "fail"
						if resp != "" {
							errMsg = resp[:min(len(resp), 200)]
						}
					}
					// 记录审计日志
					trayLog(projName, "pipeline", status, errMsg)
					it.SetTitle(title)
					time.Sleep(3 * time.Second)
					it.SetTitle("  ▶ " + projName)
				}(item, n)
			}
		}(name, pipeItem)
		stepProj := stepMenu.AddSubMenuItem("  "+name, "")
		for _, step := range []string{"check", "build", "test", "push", "deploy"} {
			stepItem := stepProj.AddSubMenuItem("    "+step, "")
		go func(n, s string, item *systray.MenuItem) {
			for range item.ClickedCh {
				go func(name, step string) {
					apiGet("/api/" + step + "?project=" + name)
				}(n, s)
			}
		}(name, step, stepItem)
		}
	}
	_ = watchAll
	_ = pipeAll
}

func fetchProjects() []map[string]any {
	// 直接读取 projects.json（同进程有文件权限，无需经过 HTTP API）
	ciDir := findCiDir()
	if ciDir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(ciDir, "projects.json"))
	if err != nil {
		return nil
	}
	var result map[string][]map[string]any
	if json.Unmarshal(data, &result) != nil {
		return nil
	}
	return result["projects"]
}

func apiGet(path string) string {
	fullURL := apiBase + path
	log.Printf("[tray] apiGet: %s\n", fullURL)
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		log.Printf("[tray] apiGet NewRequest error: %v\n", err)
		return ""
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	client := &http.Client{Timeout: 10 * time.Minute} // 长超时，CI 操作可能很久
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[tray] apiGet Do error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	log.Printf("[tray] apiGet response: status=%d len=%d\n", resp.StatusCode, len(data))
	return string(data)
}

// apiPostJSON 发送 POST 请求，携带 JSON body 和 CSRF 头。
func apiPostJSON(path string, body []byte) string {
	fullURL := apiBase + path
	log.Printf("[tray] apiPostJSON: %s\n", fullURL)
	var reqBody io.Reader
	if body != nil {
		reqBody = strings.NewReader(string(body))
	}
	req, err := http.NewRequest("POST", fullURL, reqBody)
	if err != nil {
		log.Printf("[tray] apiPostJSON NewRequest error: %v\n", err)
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[tray] apiPostJSON Do error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	log.Printf("[tray] apiPostJSON response: status=%d len=%d\n", resp.StatusCode, len(data))
	return string(data)
}

func logMsg(msg string) {
	if msg != "" {
		log.Printf("[tray] %s\n", msg[:min(len(msg), 200)])
	}
}

// trayLog 记录审计日志（通过 HTTP API）
func trayLog(project, action, status, detail string) {
	msg := fmt.Sprintf("[tray] [%s] %s %s", project, action, status)
	if detail != "" {
		msg += ": " + detail[:min(len(detail), 100)]
	}
	body, _ := json.Marshal(map[string]string{
		"message": msg,
		"level":   status,
	})
	apiPostJSON("/api/log/append", body)
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

func InitTray(port string) {
	url := fmt.Sprintf("http://localhost:%s", port)
	log.Printf("🖥️ 系统托盘已启动，右键图标可操作\n")
	RunTray(url, func() { log.Println("🛑 托盘退出") })
}
