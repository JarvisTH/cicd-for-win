package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"ci-cd/internal/util"
)

// ========== local 子命令：本地目录浏览 ==========

var CmdLocal = &cobra.Command{
	Use:   "local <command>",
	Short: "本地目录浏览（用于选择项目路径）",
}

var CmdLocalLs = &cobra.Command{
	Use:   "ls",
	Short: "列出本地目录内容（不指定 --path 时列出盘符）",
	RunE: func(cmd *cobra.Command, args []string) error {
		dirPath, _ := cmd.Flags().GetString("path")
		jsonOut, _ := cmd.Flags().GetBool("json")

		// 无路径：列盘符（Windows）或根目录
		if dirPath == "" {
			if runtime.GOOS == "windows" {
				drives := util.ListDrives()
				if jsonOut {
					data, _ := json.Marshal(drives)
					fmt.Printf("%s\n", string(data))
					return nil
				}
				if len(drives) == 0 {
					fmt.Println("📭 未找到可用盘符")
					return nil
				}
				fmt.Println("💻 可用盘符")
				fmt.Println(strings.Repeat("─", 30))
				for _, d := range drives {
					fmt.Printf("  💽 %s\n", d)
				}
				return nil
			}
			dirPath = "/"
		}

		// 规范化路径，补充盘符根斜杠
		clean := filepath.Clean(dirPath)
		if runtime.GOOS == "windows" && len(clean) == 2 && clean[1] == ':' {
			clean += string(filepath.Separator)
		}

		info, err := os.Stat(clean)
		if err != nil {
			return fmt.Errorf("路径不存在或无法访问: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("指定路径不是目录: %s", clean)
		}

		entries, err := os.ReadDir(clean)
		if err != nil {
			return fmt.Errorf("读取目录失败: %w", err)
		}

		type item struct {
			Name  string `json:"name"`
			IsDir bool   `json:"is_dir"`
			Size  int64  `json:"size"`
		}
		var dirs, files []item
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			fi, err := e.Info()
			var size int64
			if err == nil {
				size = fi.Size()
			}
			it := item{Name: e.Name(), IsDir: e.IsDir(), Size: size}
			if e.IsDir() {
				dirs = append(dirs, it)
			} else {
				files = append(files, it)
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name) })
		sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name) })

		if jsonOut {
			all := append(dirs, files...)
			data, _ := json.Marshal(map[string]any{"path": clean, "files": all})
			fmt.Printf("%s\n", string(data))
			return nil
		}

		fmt.Printf("📁 %s\n", clean)
		fmt.Println(strings.Repeat("─", 60))
		// 返回上级提示
		parent := util.LocalParent(clean)
		if parent != "" && parent != clean {
			fmt.Printf("  %-40s %s\n", "📁 ..", parent)
		}
		for _, d := range dirs {
			fmt.Printf("  %-40s %s\n", "📁 "+d.Name, "<DIR>")
		}
		for _, f := range files {
			fmt.Printf("  %-40s %d\n", "📄 "+f.Name, f.Size)
		}
		return nil
	},
}

// ========== rules 子命令：代码检查规则文件 ==========

var CmdRules = &cobra.Command{
	Use:   "rules <command>",
	Short: "管理代码检查规则文件",
}

var CmdRulesList = &cobra.Command{
	Use:   "list",
	Short: "列出可用的代码检查规则文件",
	RunE: func(cmd *cobra.Command, args []string) error {
		rulesDir := filepath.Join(ciDir(), "rules")
		entries, err := os.ReadDir(rulesDir)
		if err != nil {
			return fmt.Errorf("读取规则目录失败: %w", err)
		}
		if len(entries) == 0 {
			fmt.Println("📭 无规则文件")
			return nil
		}
		fmt.Println("🔍 代码检查规则文件")
		fmt.Println(strings.Repeat("─", 50))
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, _ := e.Info()
			size := ""
			if info != nil {
				size = fmt.Sprintf("%d B", info.Size())
			}
			fmt.Printf("  %-25s %s\n", e.Name(), size)
		}
		fmt.Println()
		fmt.Println("查看内容: ci rules view <文件名>")
		return nil
	},
}

var CmdRulesView = &cobra.Command{
	Use:   "view <file>",
	Short: "查看规则文件内容",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fileName := args[0]
		// 安全检查：禁止路径穿越
		if strings.Contains(fileName, "..") || strings.ContainsAny(fileName, `\/`) {
			return fmt.Errorf("非法文件名，只允许 rules/ 目录下的文件名")
		}
		filePath := filepath.Join(ciDir(), "rules", fileName)
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("读取规则文件失败: %w", err)
		}
		fmt.Print(string(data))
		if len(data) > 0 && data[len(data)-1] != '\n' {
			fmt.Println()
		}
		return nil
	},
}

func init() {
	CmdLocal.AddCommand(CmdLocalLs)
	CmdLocalLs.Flags().String("path", "", "要列出的目录路径（不指定则列盘符）")
	CmdLocalLs.Flags().Bool("json", false, "JSON 格式输出")

	CmdRules.AddCommand(CmdRulesList)
	CmdRules.AddCommand(CmdRulesView)
}
