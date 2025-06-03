package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/klauspost/compress/gzip"
	"github.com/schollz/progressbar/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// 特殊错误类型
var ErrSkipped = errors.New("跳过编译")

// BuildTarget 构建目标
type BuildTarget struct {
	GOOS   string
	GOARCH string
	Name   string
}

// Config 配置结构
type Config struct {
	SourceFile string
	OutputDir  string
	BinaryName string
	Platforms  []string
	Verbose    int
	Parallel   bool
	Compress   bool
	Clean      bool
	Retry      bool
	MaxRetries int
	Progress   bool
	LDFlags    string
	Tags       string
	SkipTests  bool
	SkipCGO    bool
	Force      bool
	NoPrompt   bool
	All        bool // 编译指定OS的所有架构（否则仅编译本机架构）
}

// PlatformGroups 预设平台组合
var PlatformGroups = map[string][]string{
	"default": {
		"windows/amd64", "windows/386", "windows/arm64",
		"linux/amd64", "linux/386", "linux/arm64", "linux/arm",
		"darwin/amd64", "darwin/arm64",
		"android/arm64", // 只包含最主要的Android平台
	},
	"desktop": {
		"windows/amd64", "windows/386", "windows/arm64",
		"linux/amd64", "linux/386", "linux/arm64", "linux/arm",
		"darwin/amd64", "darwin/arm64",
	},
	"server": {
		"linux/amd64", "linux/arm64",
		"freebsd/amd64", "freebsd/arm64",
	},
	"mobile": {
		"android/arm64", "android/arm",
		"ios/amd64", "ios/arm64",
	},
	"web": {
		"js/wasm",
	}, "embedded": {
		"linux/arm", "linux/arm64",
		"linux/mips", "linux/mips64",
		"linux/riscv64",
	},
	// "all" 组合将通过 getAllSupportedPlatforms() 动态获取
}

var (
	// 颜色配置
	colorTitle   = color.New(color.FgCyan, color.Bold)
	colorSuccess = color.New(color.FgGreen, color.Bold)
	colorError   = color.New(color.FgRed, color.Bold)
	colorWarning = color.New(color.FgYellow, color.Bold)
	colorInfo    = color.New(color.FgBlue)
	colorBold    = color.New(color.Bold)

	// 全局配置
	config Config
	logger *logrus.Logger
)

func init() {
	// 初始化日志
	logger = logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})

	// 检查Android环境
	checkAndroidEnvironment()
}

// checkAndroidEnvironment 检查Android环境并设置GOENV
func checkAndroidEnvironment() {
	if runtime.GOOS == "android" {
		goenvPath := "/data/adb/modules/gogogo/go.env"
		if _, err := os.Stat(goenvPath); err == nil {
			os.Setenv("GOENV", goenvPath)
			logger.Info("检测到Android环境，已设置GOENV:", goenvPath)
		}
	}
}

// checkGoEnvironment 检查Go环境
func checkGoEnvironment() error {
	colorInfo.Print("🔍 检查Go环境...")

	// 检查go命令
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("未找到go命令，请确保Go已正确安装并添加到PATH")
	}

	// 获取Go版本
	cmd := exec.Command("go", "version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("无法获取Go版本: %v", err)
	}
	colorSuccess.Printf(" ✓ %s\n", strings.TrimSpace(string(output)))
	return nil
}

// getAllSupportedPlatforms 获取Go支持的所有平台
func getAllSupportedPlatforms() ([]string, error) {
	cmd := exec.Command("go", "tool", "dist", "list")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("获取平台列表失败: %v", err)
	}

	platforms := strings.Split(strings.TrimSpace(string(output)), "\n")
	var validPlatforms []string
	for _, platform := range platforms {
		platform = strings.TrimSpace(platform)
		if platform != "" && strings.Contains(platform, "/") {
			validPlatforms = append(validPlatforms, platform)
		}
	}

	return validPlatforms, nil
}

// getArchsForOS 获取指定操作系统支持的架构列表
func getArchsForOS(targetOS string) ([]string, error) {
	allPlatforms, err := getAllSupportedPlatforms()
	if err != nil {
		return nil, err
	}

	var archs []string
	for _, platform := range allPlatforms {
		parts := strings.Split(platform, "/")
		if len(parts) == 2 && parts[0] == targetOS {
			archs = append(archs, parts[1])
		}
	}

	return archs, nil
}

// getNativeArch 获取本机架构
func getNativeArch() string {
	return runtime.GOARCH
}

// parsePlatforms 解析平台字符串
func parsePlatforms(platformStr string) []BuildTarget {
	var targets []BuildTarget
	platforms := strings.Split(platformStr, ",")
	for _, platform := range platforms {
		platform = strings.TrimSpace(platform)

		// 特殊处理 "all" 平台组合
		if platform == "all" {
			allPlatforms, err := getAllSupportedPlatforms()
			if err != nil {
				if config.Verbose >= 1 {
					colorError.Printf("⚠️  获取所有平台失败，使用静态列表: %v\n", err)
				}
				// 如果获取失败，使用静态的备用列表
				fallbackAll := []string{
					"windows/amd64", "windows/386", "windows/arm64",
					"linux/amd64", "linux/386", "linux/arm64", "linux/arm",
					"darwin/amd64", "darwin/arm64",
					"freebsd/amd64", "freebsd/arm64",
					"android/arm64", "android/arm",
					"ios/amd64", "ios/arm64",
					"js/wasm",
					"linux/mips", "linux/mips64",
					"linux/riscv64",
					"openbsd/amd64", "netbsd/amd64",
					"dragonfly/amd64", "solaris/amd64",
				}
				allPlatforms = fallbackAll
			}

			for _, p := range allPlatforms {
				parts := strings.Split(p, "/")
				if len(parts) == 2 {
					targets = append(targets, BuildTarget{
						GOOS:   parts[0],
						GOARCH: parts[1],
						Name:   p,
					})
				}
			}
		} else if group, exists := PlatformGroups[platform]; exists {
			// 检查是否是其他预设组合
			for _, p := range group {
				parts := strings.Split(p, "/")
				if len(parts) == 2 {
					targets = append(targets, BuildTarget{
						GOOS:   parts[0],
						GOARCH: parts[1],
						Name:   p,
					})
				}
			}
		} else if strings.Contains(platform, "/") {
			// 包含斜杠的为完整平台格式 (OS/ARCH)
			parts := strings.Split(platform, "/")
			if len(parts) == 2 {
				targets = append(targets, BuildTarget{
					GOOS:   parts[0],
					GOARCH: parts[1],
					Name:   platform,
				})
			}
		} else {
			// 单个操作系统名称，需要根据 -all 标志决定架构
			var archs []string
			var err error

			if config.All {
				// 获取该OS支持的所有架构
				archs, err = getArchsForOS(platform)
				if err != nil {
					if config.Verbose >= 1 {
						colorError.Printf("⚠️  获取 %s 支持的架构失败: %v\n", platform, err)
					}
					continue
				}
				if len(archs) == 0 {
					if config.Verbose >= 1 {
						colorWarning.Printf("⚠️  操作系统 %s 不支持或未找到\n", platform)
					}
					continue
				}
			} else {
				// 仅使用本机架构
				nativeArch := getNativeArch()
				// 验证该OS是否支持本机架构
				supportedArchs, err := getArchsForOS(platform)
				if err != nil {
					if config.Verbose >= 1 {
						colorError.Printf("⚠️  获取 %s 支持的架构失败: %v\n", platform, err)
					}
					continue
				}

				// 检查本机架构是否在支持列表中
				found := false
				for _, arch := range supportedArchs {
					if arch == nativeArch {
						found = true
						break
					}
				}

				if found {
					archs = []string{nativeArch}
				} else {
					if config.Verbose >= 1 {
						colorWarning.Printf("⚠️  操作系统 %s 不支持本机架构 %s，支持的架构: %s\n",
							platform, nativeArch, strings.Join(supportedArchs, ", "))
						colorInfo.Printf("💡 可以使用 --all 标志编译该OS的所有架构\n")
					}
					continue
				}
			}

			// 添加目标平台
			for _, arch := range archs {
				targets = append(targets, BuildTarget{
					GOOS:   platform,
					GOARCH: arch,
					Name:   platform + "/" + arch,
				})
			}
		}
	}
	return targets
}

// askUserConfirm 询问用户确认
func askUserConfirm(prompt string) bool {
	if config.NoPrompt {
		return true
	}

	colorWarning.Printf("%s (y/N): ", prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return response == "y" || response == "yes"
	}
	return false
}

// buildSingle 编译单个目标
func buildSingle(target BuildTarget, sourceFile, outputDir, binaryName string) error { // 跳过CGO相关平台
	if config.SkipCGO && (target.GOOS == "android" || target.GOOS == "ios") {
		if config.Verbose >= 1 {
			colorWarning.Printf("⚠️  跳过需要CGO支持的平台: %s (使用 --skip-cgo=false 强制编译)\n", target.Name)
		}
		return ErrSkipped
	}

	// 构建输出文件名
	filename := binaryName
	if target.GOOS == "windows" {
		filename += ".exe"
	}

	outputPath := filepath.Join(outputDir, target.Name, filename)

	// 确保输出目录存在
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 构建命令
	args := []string{"build"}

	if config.LDFlags != "" {
		args = append(args, "-ldflags", config.LDFlags)
	}

	if config.Tags != "" {
		args = append(args, "-tags", config.Tags)
	}

	args = append(args, "-o", outputPath, sourceFile)

	// 设置环境变量
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(),
		"GOOS="+target.GOOS,
		"GOARCH="+target.GOARCH,
	)
	// 特殊平台的CGO设置
	if target.GOOS == "js" {
		// WebAssembly需要禁用CGO
		cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	} else if target.GOOS == "ios" {
		// iOS平台特殊处理
		if runtime.GOOS != "darwin" {
			if !config.Force {
				if config.Verbose >= 1 {
					colorWarning.Printf("⚠️  跳过iOS平台: 只能在macOS上编译 (使用 --force 强制尝试)\n")
				}
				return ErrSkipped
			} else {
				colorError.Printf("⚠️  警告: 在非macOS系统上强制编译iOS，可能会失败!\n")
			}
		}

		// 检查是否安装了Xcode (仅在macOS上)
		if runtime.GOOS == "darwin" {
			if _, err := exec.LookPath("xcodebuild"); err != nil {
				return fmt.Errorf("iOS编译需要安装Xcode和Command Line Tools")
			}
		}

		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
		if config.Verbose >= 1 {
			colorWarning.Printf("⚠️  iOS平台需要Xcode和iOS SDK，建议使用gomobile工具\n")
			colorInfo.Printf("💡 安装gomobile: go install golang.org/x/mobile/cmd/gomobile@latest\n")
			colorInfo.Printf("💡 初始化gomobile: gomobile init\n")
			colorInfo.Printf("💡 构建iOS应用: gomobile build -target=ios .\n")
		}
	} else if target.GOOS == "android" {
		// Android平台处理
		if config.Verbose >= 1 {
			colorWarning.Printf("⚠️  Android平台建议使用gomobile工具进行构建\n")
			colorInfo.Printf("💡 安装gomobile: go install golang.org/x/mobile/cmd/gomobile@latest\n")
			colorInfo.Printf("💡 构建Android应用: gomobile build -target=android .\n")

			if !askUserConfirm("是否继续使用标准Go工具链编译Android平台?") {
				if config.Verbose >= 1 {
					colorInfo.Printf("⏭️  跳过Android平台编译\n")
				}
				return ErrSkipped
			}
		}

		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")

		if config.Verbose >= 1 && runtime.GOOS == "windows" {
			colorInfo.Printf("💡 Windows上可以直接编译Android/arm64平台\n")
		}

		// 为Android设置编译标志，尝试静态链接
		if config.LDFlags == "" {
			// 尝试静态链接，如果失败会降级到动态链接
			newLDFlags := "-linkmode=external -extldflags=-static"
			for i, arg := range args {
				if arg == "-o" {
					// 在-o参数前插入ldflags
					newArgs := make([]string, 0, len(args)+2)
					newArgs = append(newArgs, args[:i]...)
					newArgs = append(newArgs, "-ldflags", newLDFlags)
					newArgs = append(newArgs, args[i:]...)
					args = newArgs
					break
				}
			}
		}
	} else {
		// 其他平台通常禁用CGO以避免交叉编译问题
		cmd.Env = append(cmd.Env, "CGO_ENABLED=0")
	}

	if config.Verbose >= 2 {
		logger.Infof("执行命令: %s", strings.Join(cmd.Args, " "))
		logger.Infof("环境变量: GOOS=%s GOARCH=%s", target.GOOS, target.GOARCH)
	}

	// 执行编译
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("编译失败 [%s]: %v\n输出: %s", target.Name, err, string(output))
	}

	// 压缩文件
	if config.Compress {
		if err := compressFile(outputPath); err != nil {
			logger.Warnf("压缩文件失败 [%s]: %v", target.Name, err)
		}
	}

	return nil
}

// compressFile 压缩文件
func compressFile(filePath string) error {
	// 读取原文件
	input, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// 创建压缩文件
	compressedPath := filePath + ".gz"
	output, err := os.Create(compressedPath)
	if err != nil {
		return err
	}
	defer output.Close()

	// 使用gzip压缩
	writer := gzip.NewWriter(output)
	defer writer.Close()

	_, err = writer.Write(input)
	if err != nil {
		return err
	}

	// 删除原文件
	os.Remove(filePath)

	return nil
}

// buildWithProgress 带进度条的编译
func buildWithProgress(targets []BuildTarget, sourceFile, outputDir, binaryName string) error {
	if config.Verbose >= 1 {
		colorInfo.Printf("🚀 开始编译 %d 个目标平台\n", len(targets))
	}

	var bar *progressbar.ProgressBar
	if config.Progress && config.Verbose >= 1 {
		bar = progressbar.NewOptions(len(targets),
			progressbar.OptionSetDescription("编译进度"),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "█",
				SaucerPadding: "░",
				BarStart:      "[",
				BarEnd:        "]",
			}),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
		)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	var skipped []string
	var successful []string

	// 控制并发数
	maxWorkers := runtime.NumCPU()
	if !config.Parallel {
		maxWorkers = 1
	}

	semaphore := make(chan struct{}, maxWorkers)

	for _, target := range targets {
		wg.Add(1)
		go func(t BuildTarget) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 重试逻辑
			var err error
			for attempt := 0; attempt <= config.MaxRetries; attempt++ {
				err = buildSingle(t, sourceFile, outputDir, binaryName)
				if err == nil {
					break
				}

				if attempt < config.MaxRetries && config.Retry {
					if config.Verbose >= 2 {
						logger.Warnf("编译失败，正在重试 [%s] (第%d次): %v", t.Name, attempt+1, err)
					}
					time.Sleep(time.Second * time.Duration(attempt+1))
				}
			}

			mu.Lock()
			if err != nil {
				if errors.Is(err, ErrSkipped) {
					// 跳过的平台不计入错误
					skipped = append(skipped, t.Name)
					if config.Verbose >= 1 {
						colorWarning.Printf("⏭️ %s (跳过)\n", t.Name)
					}
				} else {
					errs = append(errs, fmt.Errorf("[%s] %v", t.Name, err))
				}
			} else {
				successful = append(successful, t.Name)
				if config.Verbose >= 1 {
					colorSuccess.Printf("✓ %s\n", t.Name)
				}
			}

			if bar != nil {
				bar.Add(1)
			}
			mu.Unlock()
		}(target)
	}
	wg.Wait()
	if len(errs) > 0 {
		colorError.Println("\n❌ 编译过程中出现错误:")
		for _, err := range errs {
			colorError.Printf("  • %v\n", err)
		}
		return fmt.Errorf("编译失败: %d个目标出现错误", len(errs))
	}

	if config.Verbose >= 1 {
		if len(successful) > 0 {
			colorSuccess.Printf("\n🎉 编译完成! 共编译 %d 个目标平台\n", len(successful))
		}
		if len(skipped) > 0 {
			colorWarning.Printf("⏭️ 跳过 %d 个目标平台: %s\n", len(skipped), strings.Join(skipped, ", "))
		}
		if len(successful) == 0 && len(skipped) > 0 {
			colorInfo.Printf("💡 所有平台都被跳过，没有实际编译任何目标\n")
		}
	}

	return nil
}

// listPlatforms 列出所有支持的平台
func listPlatforms() {
	colorTitle.Println("📋 支持的平台:")

	// 获取所有平台
	cmd := exec.Command("go", "tool", "dist", "list")
	output, err := cmd.Output()
	if err != nil {
		colorError.Printf("获取平台列表失败: %v\n", err)
		return
	}

	platforms := strings.Split(strings.TrimSpace(string(output)), "\n")

	// 按OS分组显示
	osGroups := make(map[string][]string)
	for _, platform := range platforms {
		parts := strings.Split(platform, "/")
		if len(parts) == 2 {
			osGroups[parts[0]] = append(osGroups[parts[0]], parts[1])
		}
	}

	for os, archs := range osGroups {
		colorBold.Printf("  %s: ", os)
		fmt.Printf("%s\n", strings.Join(archs, ", "))
	}
}

// listGroups 列出平台组合
func listGroups() {
	colorTitle.Println("📦 平台组合:")

	// 显示静态预设组合
	for group, platforms := range PlatformGroups {
		colorBold.Printf("  %s:\n", group)
		for _, platform := range platforms {
			fmt.Printf("    • %s\n", platform)
		}
		fmt.Println()
	}

	// 动态显示 "all" 组合
	colorBold.Printf("  all (动态获取):\n")
	allPlatforms, err := getAllSupportedPlatforms()
	if err != nil {
		colorError.Printf("    ❌ 获取失败: %v\n", err)
		fmt.Printf("    💡 将使用静态备用列表\n")
	} else {
		colorInfo.Printf("    💡 共 %d 个平台，动态从 'go tool dist list' 获取\n", len(allPlatforms))
		// 显示前几个平台作为示例
		maxShow := 10
		for i, platform := range allPlatforms {
			if i >= maxShow {
				fmt.Printf("    • ... 还有 %d 个平台\n", len(allPlatforms)-maxShow)
				break
			}
			fmt.Printf("    • %s\n", platform)
		}
	}
	fmt.Println()
}

// cleanOutputDir 清理输出目录
func cleanOutputDir(outputDir string) error {
	if _, err := os.Stat(outputDir); err == nil {
		if config.Verbose >= 1 {
			colorInfo.Printf("🧹 清理输出目录: %s\n", outputDir)
		}
		return os.RemoveAll(outputDir)
	}
	return nil
}

// showVersion 显示版本信息
func showVersion() {
	fmt.Printf(`%s%sgogogo v2.0.0 - Go跨平台编译工具%s

%s特性:%s
  ✓ 支持多平台并行编译
  ✓ 智能重试机制
  ✓ 进度条显示
  ✓ 文件压缩
  ✓ Android环境支持
  ✓ 详细的日志输出

%s环境信息:%s
  Go版本: %s
  运行平台: %s/%s
  CPU核心: %d

`,
		colorTitle.Sprint(""), colorBold.Sprint(""), color.Reset,
		colorBold.Sprint(""), color.Reset,
		colorBold.Sprint(""), color.Reset,
		runtime.Version(),
		runtime.GOOS, runtime.GOARCH,
		runtime.NumCPU(),
	)
}

// showExamples 显示使用示例
func showExamples() {
	colorTitle.Println("📚 使用示例:")
	examples := []struct {
		desc string
		cmd  string
	}{
		{"编译桌面平台", "gogogo -s main.go"},
		{"编译指定平台", "gogogo -s main.go -p windows/amd64,linux/amd64"},
		{"详细输出并压缩", "gogogo -s main.go -v 2 -c"},
		{"编译所有平台，清理输出目录", "gogogo -s main.go -p all --clean"},
		{"编译单个OS的本机架构", "gogogo -s main.go -p illumos"},
		{"编译单个OS的所有架构", "gogogo -s main.go -p illumos --all"},
		{"在Android设备上编译", "gogogo -s main.go -p android/arm64,android/arm"},
		{"强制编译iOS（在Windows上）", "gogogo -s main.go -p ios/arm64 --force"},
		{"跳过所有确认提示", "gogogo -s main.go -p mobile --no-prompt"},
		{"安静模式编译", "gogogo -s main.go -v 0"},
		{"使用自定义ldflags", "gogogo -s main.go --ldflags \"-s -w\""},
		{"跳过CGO平台", "gogogo -s main.go -p all --skip-cgo"},
	}

	for _, example := range examples {
		colorBold.Printf("  • %s:\n", example.desc)
		colorInfo.Printf("    %s\n\n", example.cmd)
	}
}

func main() {
	var rootCmd = &cobra.Command{
		Use: "gogogo", Short: "Go跨平台编译工具", Long: `gogogo v2.0.0 - 一个强大的Go跨平台编译工具

特性:
  ✓ 支持多平台并行编译
  ✓ 智能重试机制  
  ✓ 进度条显示
  ✓ 文件压缩
  ✓ Android环境支持
  ✓ 详细的日志输出
  ✓ 支持单个OS名称编译

预设平台组合:
  default    默认平台 (桌面 + Android/arm64)
  desktop    桌面平台 (Windows, Linux, macOS)
  server     服务器平台 (Linux, FreeBSD)  
  mobile     移动平台 (Android, iOS) - 需要特殊工具链
  web        Web平台 (WebAssembly)
  embedded   嵌入式平台 (ARM, MIPS, RISC-V)
  all        所有支持的平台 (动态从 'go tool dist list' 获取)

单个操作系统编译:
  • 指定OS名称 (如 'illumos', 'freebsd', 'openbsd')
  • 默认仅编译本机架构 (如在amd64上仅编译amd64)
  • 使用 --all 标志编译该OS支持的所有架构

平台编译说明:
  • 桌面平台：支持直接编译
  • Android：推荐使用gomobile工具，或在Android环境中编译
  • iOS：仅支持在macOS上编译，需要Xcode和gomobile工具
  • WebAssembly：支持直接编译
  • 其他平台：大部分支持直接跨平台编译

注意: 如果遇到CGO相关错误，请使用 --skip-cgo 参数跳过问题平台。
使用 --force 参数可以强制尝试编译iOS平台（即使不在macOS上）。
使用 --no-prompt 参数可以跳过所有用户确认提示。
使用 --all 参数编译指定OS的所有架构（否则仅编译本机架构）。`, Example: `  # 编译桌面平台
  gogogo -s main.go

  # 编译指定平台
  gogogo -s main.go -p windows/amd64,linux/amd64

  # 编译单个OS的本机架构
  gogogo -s main.go -p illumos

  # 编译单个OS的所有架构
  gogogo -s main.go -p illumos --all

  # 详细输出并压缩
  gogogo -s main.go -v 2 -c

  # 编译所有平台，清理输出目录
  gogogo -s main.go -p all --clean`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 检查必需参数
			if config.SourceFile == "" {
				return fmt.Errorf("请指定源文件 (-s)，使用 'gogogo --help' 查看帮助")
			}

			// 设置日志级别
			switch config.Verbose {
			case 0:
				logger.SetLevel(logrus.ErrorLevel)
			case 1:
				logger.SetLevel(logrus.InfoLevel)
			case 2:
				logger.SetLevel(logrus.DebugLevel)
			case 3:
				logger.SetLevel(logrus.TraceLevel)
			}

			// 检查Go环境
			if err := checkGoEnvironment(); err != nil {
				return err
			}

			// 检查源文件
			if _, err := os.Stat(config.SourceFile); err != nil {
				return fmt.Errorf("源文件不存在: %s", config.SourceFile)
			}

			// 设置默认二进制名称
			if config.BinaryName == "" {
				config.BinaryName = strings.TrimSuffix(filepath.Base(config.SourceFile), filepath.Ext(config.SourceFile))
			}

			// 清理输出目录
			if config.Clean {
				if err := cleanOutputDir(config.OutputDir); err != nil {
					return fmt.Errorf("清理输出目录失败: %v", err)
				}
			}

			// 解析目标平台
			targets := parsePlatforms(strings.Join(config.Platforms, ","))
			if len(targets) == 0 {
				return fmt.Errorf("没有找到有效的目标平台")
			}

			// 执行编译
			return buildWithProgress(targets, config.SourceFile, config.OutputDir, config.BinaryName)
		},
	}

	// 添加子命令
	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "列出所有支持的平台",
		Long:  "列出Go工具链支持的所有目标平台",
		Run: func(cmd *cobra.Command, args []string) {
			listPlatforms()
		},
	}

	var groupsCmd = &cobra.Command{
		Use:   "groups",
		Short: "列出所有平台组合",
		Long:  "列出预设的平台组合，可以直接使用这些组合名称",
		Run: func(cmd *cobra.Command, args []string) {
			listGroups()
		},
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "显示版本信息",
		Long:  "显示gogogo的版本信息和环境信息",
		Run: func(cmd *cobra.Command, args []string) {
			showVersion()
		},
	}

	var examplesCmd = &cobra.Command{
		Use:   "examples",
		Short: "显示使用示例",
		Long:  "显示详细的使用示例和常见用法",
		Run: func(cmd *cobra.Command, args []string) {
			showExamples()
		},
	}

	// 添加子命令到根命令
	rootCmd.AddCommand(listCmd, groupsCmd, versionCmd, examplesCmd)

	// 添加主要的命令行参数
	rootCmd.Flags().StringVarP(&config.SourceFile, "source", "s", "", "源Go文件路径 (必需)")
	rootCmd.Flags().StringVarP(&config.OutputDir, "output", "o", "./build", "输出目录")
	rootCmd.Flags().StringVarP(&config.BinaryName, "name", "n", "", "二进制文件名 (默认: 源文件名)")
	rootCmd.Flags().StringSliceVarP(&config.Platforms, "platforms", "p", []string{"default"}, "目标平台 (可使用预设组合或具体平台)")
	// 构建选项
	rootCmd.Flags().IntVarP(&config.Verbose, "verbose", "v", 1, "详细程度 (0=安静, 1=正常, 2=详细, 3=调试)")
	rootCmd.Flags().BoolVar(&config.Parallel, "parallel", true, "并行编译")
	rootCmd.Flags().BoolVarP(&config.Compress, "compress", "c", false, "压缩二进制文件")
	rootCmd.Flags().BoolVar(&config.Clean, "clean", false, "编译前清理输出目录")
	rootCmd.Flags().BoolVar(&config.Retry, "retry", true, "失败时重试")
	rootCmd.Flags().IntVar(&config.MaxRetries, "max-retries", 2, "最大重试次数")
	rootCmd.Flags().BoolVar(&config.Progress, "progress", true, "显示进度条")
	rootCmd.Flags().BoolVar(&config.All, "all", false, "编译指定OS的所有架构（否则仅编译本机架构）")
	// 高级选项
	rootCmd.Flags().StringVar(&config.LDFlags, "ldflags", "", "链接器标志 (如: \"-s -w\")")
	rootCmd.Flags().StringVar(&config.Tags, "tags", "", "构建标签")
	rootCmd.Flags().BoolVar(&config.SkipTests, "skip-tests", false, "跳过测试")
	rootCmd.Flags().BoolVar(&config.SkipCGO, "skip-cgo", false, "跳过需要CGO支持的平台")
	rootCmd.Flags().BoolVar(&config.Force, "force", false, "强制编译所有平台（包括在非macOS上编译iOS）")
	rootCmd.Flags().BoolVar(&config.NoPrompt, "no-prompt", false, "跳过所有用户确认提示")

	// 设置帮助模板
	rootCmd.SetHelpTemplate(`{{.Long}}

用法:
  {{.UseLine}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

别名:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

示例:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

可用命令:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

选项:
{{.LocalFlags.FlagUsages}}{{end}}{{if .HasAvailableInheritedFlags}}

全局选项:
{{.InheritedFlags.FlagUsages}}{{end}}{{if .HasHelpSubCommands}}

其他帮助主题:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

使用 "{{.CommandPath}} [command] --help" 获取更多关于命令的信息。{{end}}
`)

	// 执行命令
	if err := rootCmd.Execute(); err != nil {
		colorError.Printf("❌ 错误: %v\n", err)
		os.Exit(1)
	}
}
