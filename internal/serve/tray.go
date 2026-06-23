//go:build windows

package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/getlantern/systray"
)

var apiBase string

func RunTray(serverURL string, onExit func()) {
	apiBase = serverURL
	systray.Run(func() { setupTray() }, onExit)
}

func setupTray() {
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
	pipeMenu.AddSubMenuItem("─", "").Disable()

	stepMenu := systray.AddMenuItem("⚙ 单步骤", "")
	stepMenu.AddSubMenuItem("─", "").Disable()
	systray.AddSeparator()

	urlItem := systray.AddMenuItem(apiBase, "")
	urlItem.Disable()
	systray.AddSeparator()

	quitItem := systray.AddMenuItem("⏹ 退出", "")

	go loadProjectMenus(watchMenu, pipeMenu, stepMenu, watchAll, pipeAll)

	go func() {
		for {
			select {
			case <-openItem.ClickedCh:
				openBrowser(apiBase)
			case <-watchAll.ClickedCh:
				if watchAll.Checked() { apiPost("/api/watch/stop"); watchAll.Uncheck() } else { apiGet("/api/watch/start?project=all"); watchAll.Check() }
			case <-notifItem.ClickedCh:
				if notifItem.Checked() { notifItem.Uncheck() } else { notifItem.Check() }
			case <-pipeAll.ClickedCh:
				go func() { logMsg(apiGet("/api/pipeline/run-all")) }()
			case <-quitItem.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	select {}
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
		pipeItem := pipeMenu.AddSubMenuItem("  "+name, "")
		go func(n string, item *systray.MenuItem) {
			for range item.ClickedCh { apiGet("/api/pipeline/run?project=" + n) }
		}(name, pipeItem)
		stepProj := stepMenu.AddSubMenuItem("  "+name, "")
		for _, step := range []string{"check", "build", "test", "push", "deploy"} {
			stepItem := stepProj.AddSubMenuItem("    "+step, "")
			go func(n, s string, item *systray.MenuItem) {
				for range item.ClickedCh { apiGet("/api/" + s + "?project=" + n) }
			}(name, step, stepItem)
		}
	}
	_ = watchAll
	_ = pipeAll
}

func fetchProjects() []map[string]any {
	resp, err := http.Get(apiBase + "/api/projects")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result map[string][]map[string]any
	if json.NewDecoder(resp.Body).Decode(&result) != nil {
		return nil
	}
	return result["projects"]
}

func apiGet(path string) string {
	resp, err := http.Get(apiBase + path)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}

func apiPost(path string) string {
	resp, err := http.Post(apiBase+path, "application/json", nil)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}

func logMsg(msg string) {
	if msg != "" {
		log.Printf("[tray] %s\n", msg[:min(len(msg), 200)])
	}
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
