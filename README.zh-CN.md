<div align="center">

# avdroot

**纯 Go 实现：给 Android Studio 模拟器打 Magisk 补丁以获取 root 权限。**

[![CI](https://github.com/ejfkdev/avdroot/actions/workflows/ci.yml/badge.svg)](https://github.com/ejfkdev/avdroot/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ejfkdev/avdroot?include_prereleases&sort=semver)](https://github.com/ejfkdev/avdroot/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/ejfkdev/avdroot/blob/main/LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/ejfkdev/avdroot)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-informational)](#平台支持)
[![Built with ZCode](https://img.shields.io/badge/Built%20with%20ZCode-000000.svg?style=flat&logo=data:image/svg%2bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxMTgiIGhlaWdodD0iMTAwIiB2aWV3Qm94PSIwIDAgMjU2IDIxOCI+PHBhdGggZmlsbD0iI2ZmZmZmZiIgZD0iTTEzNC40IDAuMTMwMTUyTDExNi40OCAyNS42MDIyQzExMy42NjUgMjkuNTY5OSAxMDkuMDU0IDMyLjAwMTkgMTA0LjA2NCAzMi4wMDE5SDYuMzk5OVYwQzYuMzk5OSAwLjEzMDE0OSAxMzQuNCAwLjEzMDE1MiAxMzQuNCAwLjEzMDE1MloiLz48cGF0aCBmaWxsPSIjZmZmZmZmIiBkPSJNMjU2IDAuMTMwMTI3TDEwMi40MDEgMjE3LjczMkgwTDE1My41OTkgMC4xMzAxMjdIMjU2WiIvPjxwYXRoIGZpbGw9IiNmZmZmZmYiIGQ9Ik0xMjEuNjAxIDIxNy43MzJMMTM5LjY1IDE5Mi4xMzRDMTQyLjQ2NSAxODguMTY2IDE0Ny4wNzYgMTg1LjczNCAxNTIuMDY3IDE4NS43MzRIMjQ5LjYwNFYyMTcuNzM2SDEyMS42MDFWMjE3LjczMloiLz48L3N2Zz4=)](https://zcode.z.ai/)

[English](README.md) · [简体中文](README.zh-CN.md)

</div>

---

`avdroot` 用一条命令把模拟器从冷启动状态带到可用的 root shell。全部工作在主机侧完成：
解压 ramdisk、改写让 Magisk 的 init 替换 `/init`、再重新打包。构建镜像不需要 shell
脚本、busybox、`magiskboot`，也不需要在模拟器里执行任何代码。

```
$ avdroot root
==> Target
    Pixel_10_Pro_XL  (the only AVD)
==> Preparing Magisk's environment
    Magisk's files are not unpacked yet; writing them for 31.0 now
  ok 14 files written for Magisk 31.0, before the restart that will use them
==> Patching
    preinit device: vdd1 (read from the running emulator)
==> Restarting the emulator
    booting cold, because a snapshot would restore the pre-patch ramdisk
==> Installing the Magisk app
  ok installed com.topjohnwu.magisk (Magisk 31.0)
==> Granting su to the adb shell
  ok uid 2000 allowed
==> Verifying root
  ok Magisk 31.0:MAGISK:R is running
  ok su grants root

root: uid=0(root) gid=0(root) groups=0(root) context=u:r:magisk:s0
```

## 目录

- [为什么再写一个](#为什么再写一个)
- [安装](#安装)
- [快速开始](#快速开始)
- [命令](#命令)
- [工作原理](#工作原理)
- [选择 Magisk 版本](#选择-magisk-版本)
- [工具替你处理的坑](#工具替你处理的坑)
- [用 Chrome 抓 HTTPS](#用-chrome-抓-https)
- [平台支持](#平台支持)
- [输出语言](#输出语言)
- [开发](#开发)
- [致谢与许可](#致谢与许可)

## 为什么再写一个

[rootAVD](https://github.com/newbit1/rootAVD) 开创了这套做法，至今仍是该技术的参考实现，
但它没能跟上近年的 Android 和 Magisk 版本。拿它跑一个 Android 17 的 AVD，问题会直接
暴露出来，而这些问题在这里都修掉了：

| rootAVD 的行为 | avdroot |
|---|---|
| 需要 `bash`、`busybox`、`unzip`、`xxd`、`strings`、`dd`、`cpio`，以及 GNU 版的 `stat`/`sed`（在 macOS 上行为不同） | 单个静态 Go 二进制。cpio、LZ4、gzip、xz 全部在进程内实现 |
| 把脚本和二进制推进模拟器，在里面再跑一遍自己 | 完全在主机侧构建镜像 |
| 每次运行都备份 `ramdisk.img`，于是打过一次补丁后「原始备份」本身就是补丁版，`restore` 再也回不到原厂 | 只备份一次，永不覆盖；发现已有备份本身是补丁版会明确警告 |
| 按 Magisk 旧的 `magisk32`/`magisk64` 布局打补丁 | 实现 Magisk 26+（`magiskinit`、`magisk`、`init-ld`、`.backup/.rmlist`） |
| 需要手动粘贴 ramdisk 路径 | 自动识别 SDK、AVD 和镜像，可用 `--sdk`/`--avd-home` 覆盖 |
| 无条件重启然后听天由命 | 强制冷启动，并验证 `su` 真的返回 `uid=0` |

## 安装

**预编译二进制** —— 从 [Releases](https://github.com/ejfkdev/avdroot/releases) 取对应平台的压缩包：

```sh
tar -xzf avdroot_linux_amd64.tar.gz
chmod +x avdroot_linux_amd64
sudo mv avdroot_linux_amd64 /usr/local/bin/avdroot
```

Windows 和 macOS 的包里是单个可执行文件；macOS 上可能需要清除隔离属性：

```sh
xattr -d com.apple.quarantine avdroot_darwin_arm64
```

每个 release 同时发布 `SHA256SUMS.txt`：

```sh
sha256sum -c SHA256SUMS.txt --ignore-missing
```


**用 Go 安装**（需要 1.24 或更新）：

```sh
go install github.com/ejfkdev/avdroot@latest
```

版本下限是 1.24，因为更早的工具链不会生成 `LC_UUID` 加载命令，而 macOS 26 的
`dyld` 会直接拒绝启动这样的二进制。Go 1.21 和 1.22 还会把 `minos` 标记为
26.0，因此两条都不满足。

**从源码构建：**

```sh
git clone https://github.com/ejfkdev/avdroot
cd avdroot
make build          # 本机二进制
make all            # 交叉编译出所有发布目标到 dist/
```

你还需要带 `adb` 和模拟器的 Android SDK，以及 `google_apis` 或 AOSP 的系统镜像。
Play Store 镜像不能用这种方式 root（它不允许 adb root），所以 `patch` 和 `root`
会直接拒绝，而不是跑几分钟后才失败。

## 快速开始

```sh
avdroot list      # 看看有什么，处于什么状态
avdroot root      # 打补丁、重启、装 App、授予 su、验证
```

就这些。只有一个 AVD 时 `root` 不需要指定名称，会询问一次确认，而当 stdin 不是终端时
自动跳过询问：

```sh
avdroot root Pixel_10_Pro_XL --yes     # 指定 AVD，全自动
```

想分步执行也可以：

```sh
avdroot patch                  # 只打补丁；模拟器不必在运行
avdroot patch --dry-run        # 只显示会改什么，不写入
# 自己重启模拟器，然后
avdroot install && avdroot verify
```

撤销：

```sh
avdroot restore                # 从 ramdisk.img.backup 还原
```

## 命令

| 命令 | 说明 |
|---|---|
| `avdroot root [目标]` | 打补丁、重启、安装、授予 su、验证 —— 全自动 |
| `avdroot patch [目标]` | 只给 ramdisk 打补丁；写入前会确认 |
| `avdroot restore [目标]` | 从 `ramdisk.img.backup` 还原 |
| `avdroot list` | 列出 AVD、系统镜像及其补丁状态 |
| `avdroot status [目标]` | 查看某个目标的详情，含 preinit 分区 |
| `avdroot verify [目标]` | 检查 Magisk 是否运行、`su` 是否返回 `uid=0` |
| `avdroot install` | 把 Magisk App 安装到运行中的模拟器 |
| `avdroot magisk info` | 会使用哪个安装包，以及它支持什么 |
| `avdroot magisk fetch` | 下载发行版（`--list`、`--version`、`--prerelease`） |
| `avdroot trust-chrome` | 让 Chrome 接受中间人 CA（`--cert`、`--clear`） |
| `avdroot doctor` | 说明每个路径是怎么找到的 |

「目标」可以是 AVD 名称、系统镜像路径，或 `ramdisk.img` 的路径。
`avdroot <命令> --help` 会给出该命令的示例。

### 全局参数

对所有命令都有效：

| 参数 | 作用 |
|---|---|
| `--sdk PATH` | 手动指定 Android SDK 根目录（自动识别不对时） |
| `--avd-home PATH` | AVD 定义所在目录 |
| `--serial SERIAL` | 连接了多个设备时指定用哪个 |
| `--magisk PATH` | 使用指定的 Magisk 安装包 |
| `--abi ABI` | 覆盖检测到的 ABI |
| `--compress FORMAT` | 覆盖 ramdisk 压缩格式（`gzip`、`lz4_legacy`、`lz4`、`xz`） |
| `--preinit DEVICE` | 覆盖 preinit 块设备 |
| `--lang` | 覆盖系统语言（`zh` 或 `en`） |
| `--no-adb` | 不连接运行中的模拟器 |
| `--no-download` | 不自动下载 Magisk 安装包 |

### 常用命令参数

| 参数 | 命令 | 作用 |
|---|---|---|
| `-y`、`--yes` | `root`、`patch` | 跳过确认 |
| `--dry-run` | `patch` | 只打印计划，不写入 |
| `--force` | `root`、`patch` | 对已打过补丁的 ramdisk 也重新打 |
| `--no-install` | `root` | 不装 Magisk App；能不能 root 与它无关 |
| `--no-restart` | `root`、`trust-chrome` | 不动模拟器或浏览器 |
| `--timeout 10m` | `root` | 等待启动的超时时间 |
| `--keep-verity`、`--keep-forceencrypt` | `patch` | 默认都开启；关掉会去掉 fstab 里的对应选项 |
| `--recovery-mode` | `patch` | 按 recovery 模式打补丁（API 28 自动启用） |
| `--cert PATH` | `trust-chrome` | 指定要信任的 CA；省略则自动识别 |
| `--clear` | `trust-chrome` | 撤销浏览器信任 |
| `--list` | `restore`、`magisk fetch` | 只列出，不执行 |

## 工作原理

现代系统镜像的 `ramdisk.img` 是 **cpio `newc` 归档**，用 gzip、lz4 或 xz 压缩。
Android 11+ 的镜像常常在一个压缩流里串联两个归档 —— 通用 ramdisk 和 vendor ramdisk。
打补丁就是：

1. **解压**（`internal/imgfmt`）。LZ4 legacy 流按块解码，包括串联流产生的重复 magic。
2. **解析并合并** cpio 归档（`internal/cpio`）。同名条目后者覆盖前者，与内核顺序解包
   的语义一致。
3. **应用 Magisk 补丁**（`internal/magisk`），复刻 `boot_patch.sh`：先检测并回滚已有
   补丁；用 `magiskinit` 替换 `/init`（权限 `0750`）；加入
   `overlay.d/sbin/{magisk,stub,init-ld}.xz`；按需去掉 fstab 里的 verity 和加密选项；
   把与原厂归档的差异记录到 `.backup`，其中 `.backup/.rmlist` 列出 restore 需要删除
   的条目。
4. **按原格式重新压缩**，并以原子方式写入镜像。

序列化结果与 Magisk v31 的 `magiskboot` 字节兼容：inode 从 300000 开始编号，`nlink`
固定为 1，`mtime` 为 0，条目按名称字节序输出。`internal/magisk/parity_test.go` 会断言
本工具为真实 AVD 产出的条目集合、权限、`.rmlist` 内容和配置，与 Magisk 官方安装器
打出的 ramdisk 完全一致。

## 选择 Magisk 版本

Magisk 发布的是**一个通用包**，不是每个 Android 版本一个。一个发行版里就包含全部 ABI
的原生库，只要 Android 版本不低于它声明的 `minSdkVersion` 就能装：

```
$ avdroot magisk info
  ok Magisk 31.0 (31000, arm64-v8a)
    package      com.topjohnwu.magisk
    supports     API 23+ (built against 37)
    abis         arm64-v8a, armeabi-v7a, x86, x86_64
  ok compatible with Pixel_10_Pro_XL
```

所以真正按设备判断的只有两件事：

- **ABI** —— 从包里取哪个 `lib/<abi>/` 目录。自动识别，可用 `--abi` 覆盖。
- **API 范围** —— 直接解析包里的二进制 `AndroidManifest.xml` 得到，不需要 `aapt2`。
  低于 `minSdkVersion` 时直接报错停止，因为 App 根本装不上；若只是发行版比系统明显
  偏旧，则给出警告，最终以 root 验证结果为准。

发行版分两个通道，最新的工作往往只在预发布里 —— `v31.0` 就是预发布，`v30.7` 才是最
新稳定版：

```
$ avdroot magisk fetch --list
TAG    CHANNEL  PUBLISHED   SIZE
v30.7  stable   2026-02-23  11.1 MiB

$ avdroot magisk fetch --list --prerelease
v31.0  pre-release  2026-09-04  10.7 MiB
```

`fetch` 通过 GitHub API 解析最新版；指定 tag 时直接用固定 URL，所以 API 被限流也能用。
下载失败时会说明原因并给出发行页：

```
warn download failed: HTTP 404: no release with that tag

Fetch it manually
    1. open https://github.com/topjohnwu/Magisk/releases
    2. download the Magisk-v<version>.apk asset you need
    3. pass it straight to a command, no need to move it into place:
         avdroot root --magisk /path/to/Magisk-v31.0.apk
       or put it at ~/Library/Caches/avdroot/Magisk.zip
```

### 安装包从哪里来，下载到哪里去

缺少安装包时会**自动下载**，所以在全新机器上第一次运行就能直接用。本地已有安装包时
不会联网；`--no-download` 可关闭下载，供离线或脚本场景使用。

下载落在操作系统的缓存目录——安装包是可复现的，放在缓存里清理缓存就能回收，不会动到
你的配置：

| 平台 | 位置 |
|---|---|
| macOS | `~/Library/Caches/avdroot/Magisk.zip` |
| Linux | `$XDG_CACHE_HOME/avdroot/Magisk.zip`，通常是 `~/.cache/avdroot/` |
| Windows | `%LocalAppData%\avdroot\Magisk.zip` |

`avdroot doctor` 会打印确切路径。

查找顺序，命中即停：

1. `--magisk PATH`
2. `$AVDROOT_MAGISK`
3. 上面的缓存目录
4. `$XDG_CONFIG_HOME/avdroot`、`~/.config/avdroot` 或平台配置目录
5. `avdroot` 二进制同级目录，然后是它的上一级
6. 当前工作目录
7. `~/Downloads`

缓存排在配置目录之前是有意的：刚 fetch 下来的版本应该被用上，而不是被一个放在旁边
的旧文件顶掉。

下载失败会自动重试三次（带退避），并遵循 `HTTPS_PROXY`。若仍失败，命令会解释原因并
给出手动方案，所以网络受限也不会走投无路。

## 工具替你处理的坑

这些是让模拟器 root 变得麻烦的行为。每一条都由工具自动处理，也都是实测发现的，
不是猜的。

**guest 重启不会重新加载 ramdisk。** `adb reboot` 只是重启 guest，用的还是模拟器进程
启动时加载的 initrd，所以刚打好的 `ramdisk.img` 会被忽略。只有**新的模拟器进程**才会
重新读取。因此 `avdroot root` 会杀掉进程再重启，`verify` 在运行中的 Magisk 与 ramdisk
不一致时会报告版本不匹配。

**安装包没有做签名校验。** 会验证它确实是一个可用的 Magisk 发行版（能解析、包名是
`com.topjohnwu.magisk`、含目标 ABI 所需的二进制），并且通过 HTTPS 从官方发行页下载。
但**不校验 APK 自身的签名**，所以请从官方页面下载，或用 `--magisk` 指定你信任的文件。

**快照会把打补丁之前的 ramdisk 恢复回来。** 如果 AVD 开了 fast boot，模拟器可能恢复
已保存的虚拟机状态，而其中包含当时加载的 initrd —— 于是重启可能又回到**旧** ramdisk，
看起来像补丁完全没生效。`avdroot root` 重启时一律加 `-no-snapshot`，`root` 和 `doctor`
都会报告快照与 fast boot 的存在。工具从不删除任何快照。

**备份永远不能被覆盖。** 每次运行都备份，正是许多用户无法还原的原因：第一次打补丁后
他们的「原始副本」已经是补丁版。`avdroot` 只备份一次，并在发现备份本身是补丁版时警告。

**`PREINITDEVICE` 必须在打补丁时写死。** Magisk 是运行时推导的。`avdroot` 移植了
Magisk 的 `find_preinit_device`，读取运行中设备的 `/proc/self/mountinfo` 计算，对
`/data` 在 device-mapper 上、`/metadata` 在 virtio 盘上的 AVD 会得到 `vdd1`。模拟器
没运行时请传 `--preinit`。

**打过补丁的 ramdisk 不等于装好了 Magisk。** 它只装上了 Magisk 的 init 和守护进程，
而 Magisk 的其余文件在管理器 App 里、应当位于 `/data/adb/magisk`。`magiskd` 在**启动时**
判断这个目录是否完整：不完整时它根本不会把 `su` 放到 `PATH` 上，而 `PATH` 里那个 `su`
是模拟器自带的 —— 它把 `-c` 当成 uid 解析，报 `su: invalid uid/gid '-c'`。而且全程看不出
异常：Magisk 照样报版本号、照样接受 su 策略写入，症状是工具宣称 Magisk 一切正常、
却根本用不了它。

Magisk 自己的检查是
[`scripts/app_functions.sh`](https://github.com/topjohnwu/Magisk/blob/master/scripts/app_functions.sh)
里的 `env_check` shell 函数，而它的修复函数 `fix_env` 就是往那个目录拷文件。两者都不是
`magisk` 的子命令，也都不碰 boot 镜像。`avdroot` 复刻了这两者，因此 App、它的设置对话框
以及第二次重启全都不需要：文件赶在**本来就要做的那次重启之前**写好，于是第一次用打过补丁的
ramdisk 启动时它们就已经在位了。

**`su` 通常需要手点授权。** Magisk 在某个 uid 第一次请求 root 时会询问 App，而脚本
没法回答这个问题。`avdroot root` 改用 `magisk --sqlite` 直接写入策略行，取值来自
Magisk 自己的 `SuPolicy` 枚举（`Allow = 2`，`until = 0` 表示永不过期）。守护进程每次
请求都读这一行，所以立即生效、重启后仍在。

**`adb install` 是从主机推流的**，并且要求文件名以 `.apk` 结尾，所以传设备路径永远
不行。工具会把安装包落到一个临时 `.apk` 再安装。

## 用 Chrome 抓 HTTPS

把 CA 装进系统证书库对几乎所有 App 都够了，但**对 Chrome 不够**。**Chrome 在
Android 上会对「锚定到系统库的证书」强制校验 Certificate Transparency** —— 它判断
"是不是公共根 CA"的标准就是"在不在系统库"。本地生成的中间人 CA 没有 CT 日志记录，
于是 Chrome 报 `NET::ERR_CERT_AUTHORITY_INVALID`，而其他 App 一切正常。

这个现象之所以难排查，正是因为除 Chrome 外全都正常：

| 客户端 | 同一张证书的结果 |
|---|---|
| Android 平台信任库（普通 App） | 接受 |
| Chrome | `NET::ERR_CERT_AUTHORITY_INVALID` |

`avdroot trust-chrome` 通过给 Chromium 加 `--ignore-certificate-errors-spki-list`
启动参数，显式信任这一把公钥：

```
$ avdroot trust-chrome
==> 准备让 Chrome 信任 CA
  ok 证书：CN=Reqable CA (Jul 20, 2026, 16EB1AA6), ...
    读取自 /data/data/com.reqable.android/files/certificate/reqable-root.crt
    SPKI 指纹：nHhlCoiiwRRZzTU+d2MDZrdTs7bvRBrIWpFSTTyqWDU=
==> 写入 Chromium 启动参数
  ok 已写入 8 个参数文件
==> 重启 Chrome 以读取参数
  ok Chrome 已停止；重新打开即可生效
```

Reqable 的导出格式里，除 PKCS#12 外都能直接用：PEM 和 DER 都接受，与扩展名无关，
所以 `.pem`、`.crt`、`.0` 都可以。`.p12` 是容器而不是证书，解码需要密码和它自己的加密
算法；请改导出 PEM。

证书会从设备上「系统镜像之外」的 CA 里自动识别：通过设置装的、Magisk 模块装的、以及
**工具自己数据目录里的**——对于用 root 在运行时注入的工具，那是唯一的持久副本。也可以用
`--cert` 显式指定（本地路径或设备路径）。`--clear` 撤销。

三点需要知道：

- **指纹与密钥绑定**：中间人工具重新生成 CA 后需要重跑。
- **这会为该密钥关闭 Certificate Transparency 校验**，确实削弱了一层保护。仅适用于
  模拟器和测试设备。
- **必须重启 Chrome**，因为参数只在进程启动时读取。命令会自动帮你重启。

## 平台支持

| 平台 | 路径识别 | 打补丁 | 重启模拟器 |
|---|---|---|---|
| macOS | 支持 | 已实测 | 已实测 |
| Linux | 支持 | 共用代码 | 共用代码 |
| Windows | 支持 | 共用代码 | 已实现；不自动识别启动参数 |

打补丁部分是纯 Go、没有平台相关代码，所以在哪台主机上打出的 ramdisk 都逐字节一致。
这里只在 macOS 上做过端到端实测（arm64 Android 17 AVD）；Linux 和 Windows 每次改动
都会编译并跑单元测试，包括 CI 里的交叉编译。

Windows 上重启模拟器只带 `-avd NAME`，因为那里读取进程命令行不够可靠。想保留
`-no-window` 之类可以传 `--emulator-args`。传统控制台不支持 ANSI，会自动关闭颜色。

## 输出语言

输出跟随系统语言，找不到就回退到英文。Unix 上从 `LANG`/`LC_ALL`/`LC_MESSAGES` 识别
中文；Windows 上这些变量通常没有设置，改用 `GetUserDefaultUILanguage`。

```sh
avdroot list --lang zh          # 强制中文
avdroot list --lang en          # 强制英文
AVDROOT_LANG=zh avdroot list    # 或用环境变量
```

帮助文本同样会翻译：

```
$ LANG=zh_CN.UTF-8 avdroot patch --help
用法：
  avdroot patch [avd|image|ramdisk] [flags]

示例：
  avdroot patch --dry-run           # 只打印计划，不写任何东西
  avdroot patch Pixel_10_Pro_XL -y  # 指定 AVD，不询问

参数：
      --dry-run             只报告会改什么，不写入任何内容
```

内部包抛出的技术性诊断信息保持英文；命令层、它产生的错误以及所有帮助文本都已翻译。
新增一门语言只需加一个文件 —— 参考 `internal/i18n/zh.go`，一致性测试会指出漏掉的
条目。

## 开发

```sh
make build        # 本机二进制
make all          # 交叉编译所有发布目标
make test
make vet
```

### 代码结构

```
main.go                 入口
cmd/                    CLI：每个命令一个文件，另有安装包解析
internal/cpio/          newc 归档格式，以及 Magisk 的 patch/backup/restore
internal/imgfmt/        gzip、lz4、lz4_legacy、xz、lzma、bzip2
internal/magisk/        安装包解包与 boot_patch.sh 的等价实现
internal/avd/           SDK、AVD、系统镜像的识别
internal/ramdisk/       读写、原子写入、备份与还原
internal/adb/           adb 封装、su 策略、preinit 分区探测、Magisk 运行环境
internal/emulator/      模拟器启动、重启与等待开机
internal/axml/          二进制 AndroidManifest.xml 解析
internal/certutil/      证书解析与 SPKI 指纹
internal/i18n/          中英文消息目录
internal/ui/            终端输出
```

正确性最关键的是 `internal/cpio` 和 `internal/imgfmt`：它们逐字节实现格式，一致性测试
会把结果与 Magisk 官方打出的 ramdisk 对照。

### 测试

需要真实数据的测试在资源缺失时会自动跳过，所以在没装 Android SDK 的机器上
`go test ./...` 也能通过——99 个执行、14 个跳过。要完整跑起来，把
`AVDROOT_TEST_ASSETS` 指向含所需文件的目录（清单见
[testdata/README.md](testdata/README.md)）：

```sh
export AVDROOT_TEST_ASSETS=~/avdroot-assets
go test ./...     # 110 个执行，仍有 1 个跳过
```

仍然跳过的那个是证书导出格式的对比测试，需要代理导出同一张 CA 的三种格式，
清单同样见 [testdata/README.md](testdata/README.md)。

这些资源让测试套件变成针对真实数据的 cpio、压缩和 Magisk 层完整回归，其中包含与
Magisk 官方打补丁结果的对照。它们永远不入库。

### 参与贡献

打补丁部分刻意保持**纯 Go、无平台相关代码**，因此在任何主机上打出的 ramdisk 都逐字节
一致。请保持这一点：平台差异属于路径识别、进程处理和输出，不要进入镜像处理路径。

任何用户可见的改动都需要在 `internal/i18n/en.go` 和 `internal/i18n/zh.go` 各加一条；
一致性测试会在只改一边时失败。

## 致谢与许可

技术、文件格式和补丁语义来自 John Wu 的 [Magisk](https://github.com/topjohnwu/Magisk)，
以及 NewBit XDA 的 [rootAVD](https://github.com/newbit1/rootAVD) —— 后者证明了模拟器的
ramdisk 是可以这样打补丁的。

本项目是独立重写，与两者不共享任何代码；实现的确实是它们定义的格式，所以上面这些
致谢是必须写明的。

以 [MIT 许可](https://github.com/ejfkdev/avdroot/blob/main/LICENSE) 发布。
项目不分发 Magisk 本体，请从
[官方发行页](https://github.com/topjohnwu/Magisk/releases) 下载。
