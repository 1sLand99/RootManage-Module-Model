# Git Android 编译脚本 v4.0 完整版

这是一个功能强大的Android Git编译脚本，支持多种编译环境和模式，能够为Android设备生成完全兼容的Git二进制文件。

## 🚀 主要特性

### 支持的编译环境
- **Termux** - Android终端模拟器，本机编译
- **Android原生** - 在Android系统中直接编译（需要root）
- **Linux交叉编译** - 在Linux系统上交叉编译ARM64版本
- **WSL/WSL2** - Windows子系统Linux环境
- **Android NDK** - 使用Android NDK工具链编译

### 编译模式
1. **Termux本机编译** - 推荐，在Termux中直接编译
2. **静态编译** - 生成无依赖的静态链接版本
3. **NDK动态编译** - 使用Android NDK编译
4. **最小化编译** - 精简版本，禁用部分功能
5. **自动模式** - 智能检测环境并选择最佳编译方式

### 强大的功能
- 🔧 **智能依赖管理** - 自动编译OpenSSL、zlib、curl、expat等ARM64依赖库
- 🛠️ **多包管理器支持** - 支持apt、yum、pacman等包管理器
- 📊 **进度追踪** - 可视化编译进度和步骤状态
- 🔄 **断点续编** - 支持从中断的地方继续编译
- ✅ **完整测试** - 7项功能测试确保编译质量
- 📦 **安装脚本** - 自动生成Android安装脚本
- 📋 **详细日志** - 完整的编译日志和错误诊断

## 📋 环境要求

### Termux环境
```bash
pkg update
pkg install git clang make autoconf automake libtool pkg-config gettext curl openssl zlib expat
```

### Linux交叉编译环境
```bash
# Ubuntu/Debian
sudo apt-get install gcc-aarch64-linux-gnu build-essential autoconf automake libtool make pkg-config gettext git wget curl cmake python3 libssl-dev zlib1g-dev libexpat1-dev

# CentOS/RHEL
sudo yum groupinstall "Development Tools"
sudo yum install gcc-aarch64-linux-gnu autoconf automake libtool make pkg-config gettext git wget curl cmake python3 openssl-devel zlib-devel expat-devel

# Arch Linux
sudo pacman -S base-devel aarch64-linux-gnu-gcc autoconf automake libtool make pkg-config gettext git wget curl cmake python openssl zlib expat
```

### Android NDK环境
1. 下载Android NDK: https://developer.android.com/ndk/downloads
2. 解压到当前目录或设置环境变量：
   ```bash
   export ANDROID_NDK_HOME=/path/to/android-ndk-rXX
   ```

## 🚀 使用方法

### 基本使用
```bash
# 下载脚本
wget https://raw.githubusercontent.com/your-repo/compile_git_android.sh
chmod +x compile_git_android.sh

# 交互式编译（推荐）
./compile_git_android.sh

# 自动选择最佳模式
./compile_git_android.sh auto

# 显示帮助
./compile_git_android.sh help
```

### 高级功能
```bash
# 断点续编
./compile_git_android.sh resume

# 清理构建文件
./compile_git_android.sh clean
```

## 📖 编译流程详解

### 1. 环境检测和模式选择
脚本会自动检测运行环境：
- 检查是否在Termux、Android原生、WSL或Linux环境
- 检测可用的编译工具和NDK
- 根据环境智能推荐最佳编译模式

### 2. 依赖安装
- 根据不同环境安装必要的编译工具
- 支持多种包管理器（apt、yum、pacman、pkg）
- 检查并安装交叉编译工具链

### 3. ARM64依赖库编译
所有依赖库都使用NDK工具链编译，确保ARM64兼容性：
- **zlib** - 数据压缩库
- **OpenSSL** - 加密和SSL/TLS支持
- **curl** - HTTP/HTTPS传输支持
- **expat** - XML解析支持

### 4. Git源码获取
- 支持多个镜像源（Gitee、GitHub、Kernel.org等）
- 自动重试和错误恢复
- 源码完整性验证

### 5. 构建配置
- 智能库检测和配置
- 根据编译模式优化configure参数
- 详细的配置摘要显示

### 6. 编译和测试
- 并行编译提高速度
- 完整的功能测试套件
- 二进制文件架构验证

### 7. 安装包创建
- 生成tar.gz安装包
- 创建智能安装脚本
- 包含所有ARM64依赖库

## 📦 输出文件说明

编译完成后会生成以下文件：

### 安装包
- `git-android-{version}-{mode}-{date}.tar.gz` - 主安装包

### 安装脚本
- `install_git_android.sh` - 完整安装脚本（推荐）
- `quick_install.sh` - 快速安装脚本

### 信息文件
- `git-source-info.txt` - Git源码信息
- `DEPENDENCIES_INFO.txt` - ARM64依赖库信息
- `compile.log` - 完整编译日志

## 🔧 Android安装说明

### 方法1：使用完整安装脚本（推荐）
```bash
# 将安装包复制到Android设备
adb push git-android-*.tar.gz /data/local/tmp/
adb push install_git_android.sh /data/local/tmp/

# 在Android设备上安装
adb shell
cd /data/local/tmp
tar -xzf git-android-*.tar.gz
su  # 获取root权限
./install_git_android.sh
```

### 方法2：快速安装
```bash
# 解压并运行快速安装
tar -xzf git-android-*.tar.gz
su
./quick_install.sh
```

### 方法3：Termux安装
如果是Termux编译的版本，可以直接使用：
```bash
tar -xzf git-android-*.tar.gz -C $PREFIX/
```

## 🐛 故障排除

### 常见问题

#### 1. NDK编译器找不到
**错误**: `NDK编译器不存在`
**解决**: 
- 检查ANDROID_NDK_HOME环境变量
- 确保NDK版本支持API 21+
- 下载最新NDK版本

#### 2. 依赖库编译失败
**错误**: `OpenSSL编译失败`
**解决**:
- 检查网络连接（下载源码）
- 确保有足够的磁盘空间
- 使用 `./compile_git_android.sh resume` 继续编译

#### 3. configure失败
**错误**: `configure失败`
**解决**:
- 查看 `config.log` 获取详细错误
- 检查编译器和依赖库路径
- 确保所有依赖都正确安装

#### 4. 交叉编译器缺失
**错误**: `交叉编译器不存在`
**解决**:
```bash
# Ubuntu/Debian
sudo apt-get install gcc-aarch64-linux-gnu

# CentOS/RHEL  
sudo yum install gcc-aarch64-linux-gnu

# Arch Linux
sudo pacman -S aarch64-linux-gnu-gcc
```

### 调试技巧

1. **查看详细日志**:
   ```bash
   tail -f android-git-build/compile.log
   ```

2. **检查编译状态**:
   ```bash
   cat android-git-build/.build_state
   ```

3. **手动验证依赖**:
   ```bash
   file android-git-build/deps/lib/*.a
   ```

4. **测试编译的Git**:
   ```bash
   ./android-git-build/git-android/bin/git --version
   ```

## 🔧 高级配置

### 自定义编译选项
可以通过修改脚本中的configure参数来自定义编译：
- 添加或移除功能模块
- 修改安装路径
- 调整优化级别

### 支持新的环境
脚本结构化设计，易于扩展：
- 添加新的环境检测
- 实现新的编译模式
- 支持新的包管理器

## 📊 性能数据

### 编译时间（参考）
- **Termux (8核ARM64)**: ~15-20分钟
- **Linux交叉编译 (8核x64)**: ~10-15分钟
- **WSL2 (4核)**: ~20-25分钟

### 安装包大小
- **完整版**: ~8-12MB
- **最小化版**: ~4-6MB
- **依赖库**: ~3-5MB

## 🤝 贡献和支持

### 贡献指南
欢迎提交Issue和Pull Request：
- Bug报告
- 功能建议
- 代码改进
- 文档完善

### 获取支持
如遇问题，请提供：
- 运行环境信息
- 编译日志
- 错误信息
- 复现步骤

## 📄 许可证

本项目遵循GPL-2.0许可证，与Git项目保持一致。

## 🔄 更新历史

### v4.0 (当前版本)
- ✅ 完整的ARM64依赖库编译支持
- ✅ 智能环境检测和模式选择
- ✅ 断点续编功能
- ✅ 完整的测试套件
- ✅ 多包管理器支持
- ✅ 增强的错误处理和恢复

### v3.x
- 基础的交叉编译支持
- 简单的依赖管理

### v2.x  
- Termux编译支持
- 基础功能实现

### v1.x
- 初始版本
- 基本编译功能

---

**注意**: 确保在编译前阅读完整文档，特别是环境要求和故障排除部分。如有问题，请查看日志文件获取详细信息。
