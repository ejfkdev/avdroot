package i18n

// chinese is the Simplified Chinese catalogue. Its key set must match english;
// a test enforces that, and any key missing here falls back to English.
var chinese = map[string]string{
	// Application
	"app.name":    "avdroot",
	"app.tagline": "给 Android Studio 模拟器打 Magisk 补丁以获取 root 权限",
	"app.long": `avdroot 给 Android Studio 模拟器的 ramdisk 打补丁，让 Magisk 在开机时装载，
从而让模拟器获得 root 权限。

全部工作在主机侧完成：本程序自己解压 ramdisk、就地改写、再重新打包。构建镜像的
过程中不需要 shell、busybox，也不需要在模拟器里运行任何 Magisk 二进制。

打好的镜像会覆盖 ramdisk.img，原文件保存为 ramdisk.img.backup，
因此 "avdroot restore" 随时可以还原。`,
	"app.example": `  # 查看有哪些 AVD、是否已打过补丁
  avdroot list

  # 从冷启动状态一条命令完成 root（无需人工干预）
  avdroot root

  # 指定 AVD，并跳过确认
  avdroot root Pixel_10_Pro_XL --yes

  # 只打补丁；模拟器不必在运行
  avdroot patch
  avdroot patch --dry-run          # 只预览会改什么

  # 撤销
  avdroot restore

  # 结果不对劲？看看每个路径是怎么找到的
  avdroot doctor`,
	"help.repo":         "项目地址：https://github.com/ejfkdev/avdroot\n",
	"help.usage":        "用法：",
	"help.examples":     "示例：\n",
	"help.commands":     "可用命令：",
	"help.flags":        "参数：\n",
	"help.global_flags": "全局参数：\n",
	"help.more":         "其他命令：",
	"help.help_hint":    "用 \"{{.CommandPath}} [command] --help\" 查看某个子命令的详细说明。",
	"help.version":      "版本 ",

	"app.lang_hint": "输出语言由 $AVDROOT_LANG 或系统语言决定；用 --lang en 切回英文",

	// Shared field labels
	"field.target":      "目标",
	"field.emulator":    "模拟器",
	"field.ramdisk":     "Ramdisk",
	"field.name":        "名称",
	"field.abi":         "ABI",
	"field.android":     "安卓版本",
	"field.status":      "状态",
	"field.compression": "压缩格式",
	"field.size":        "文件大小",
	"field.entries":     "cpio 条目",
	"field.backup":      "备份",
	"field.serial":      "序列号",
	"field.preinit":     "preinit 分区",
	"field.ramdisk_is":  "ramdisk：{0}",
	"field.problems":    "问题",
	"field.next":        "下一步",
	"field.snapshots":   "快照",
	"field.fastboot":    "快速启动",

	// list
	"list.short":      "列出 AVD、系统镜像及其补丁状态",
	"list.example":    "  avdroot list\n  avdroot list --lang en",
	"list.sdk":        "Android SDK：{0}",
	"list.avd_dir":    "AVD 目录：{0}",
	"list.avds":       "AVD",
	"list.images":     "系统镜像",
	"list.no_avds":    "没有定义任何 AVD",
	"list.no_ramdisk": "无 ramdisk",
	"list.unreadable": "无法读取",
	"list.next_hint":  "下一步：\"avdroot patch\" 只打补丁，\"avdroot root\" 打补丁并验证。",

	// status
	"status.short":   "查看某个目标的详情",
	"status.example": "  avdroot status\n  avdroot status Pixel_10_Pro_XL",

	// doctor
	"doctor.short": "说明 SDK、AVD 目录、adb 和模拟器是怎么找到的",
	"doctor.long": `doctor 会打印 avdroot 解析出的环境，以及所有被考虑过的位置。当路径识别
不对、找不到 AVD，或者同一件事在一台机器上正常另一台上不正常时，用它排查。

每一项都可以用参数覆盖：--sdk、--avd-home、--serial、--magisk。`,
	"doctor.example":       "  avdroot doctor\n  avdroot doctor --sdk /your/sdk",
	"doctor.host":          "主机",
	"doctor.package":       "包名",
	"doctor.env":           "环境变量",
	"list.sdk_heading":     "Android SDK",
	"doctor.avd_heading":   "AVD 目录",
	"doctor.os":            "系统",
	"doctor.go_runtime":    "Go 版本",
	"doctor.home":          "主目录",
	"doctor.selected":      "选中",
	"doctor.found_via":     "来源",
	"doctor.markers":       "标志目录",
	"doctor.error":         "错误：{0}",
	"doctor.no_avds":       "无",
	"doctor.magisk":        "Magisk 安装包",
	"doctor.result":        "结果",
	"doctor.none_found":    "（未找到）",
	"doctor.unset":         "（未设置）",
	"doctor.candidates":    "候选位置：",
	"doctor.tools":         "工具",
	"doctor.not_found":     "（未找到）",
	"doctor.no_candidates": "没有生成任何候选位置",
	"doctor.state_missing": "不存在",
	"doctor.state_exists":  "存在，但不像 SDK",
	"doctor.state_ok":      "正常",
	"doctor.markers_none":  "无（这可能不是 SDK）",
	"doctor.fetch_hint":    "运行 \"avdroot magisk fetch\" 或传 --magisk",
	"doctor.no_avds_hint":  "在 Android Studio 里建一个，或检查上面的 AVD 目录",
	"doctor.fastboot_warn": "是（快照可能把打补丁前的 ramdisk 恢复回来）",

	// patch
	"patch.short": "给 ramdisk 打补丁，让 Magisk 在开机时装载",
	"patch.long": `patch 会解压 ramdisk、用 Magisk 的 init 替换 /init、把 Magisk 载荷放进
overlay.d/sbin，然后按原压缩格式写回。

首次运行会把原始镜像保存为 ramdisk.img.backup，后续复用，所以 "restore"
总能还原到原厂状态。

dm-verity 和强制加密默认保持开启，这正是模拟器需要的。

ramdisk 是就地修改的，所以写入前会先打印摘要并请求确认。用 --yes 跳过确认
（stdin 不是终端时自动跳过），或用 --dry-run 只预览不写入。`,
	"patch.example": `  avdroot patch                     # 唯一的 AVD，会先确认
  avdroot patch --dry-run           # 只打印计划，不写任何东西
  avdroot patch Pixel_10_Pro_XL -y  # 指定 AVD，不询问
  avdroot patch --preinit vdd1      # 模拟器没运行：手动指定分区`,
	"patch.checking_target":        "检查目标",
	"patch.already_patched":        "这个 ramdisk 已经打过 Magisk 补丁，将重新打一遍",
	"patch.force_hint":             "加 --force 可跳过此提示",
	"patch.loading_magisk":         "加载 Magisk",
	"patch.supports":               "支持 {0}，包含 {1}",
	"patch.reading":                "读取 ramdisk",
	"patch.compress_override":      "压缩格式被覆盖：{0} -> {1}",
	"patch.uncompressed":           "{0}，解压后 {1}",
	"patch.patching":               "打补丁",
	"patch.prev_state":             "之前的状态：{0}",
	"patch.added":                  "新增 {0}",
	"patch.backed_up":              "已备份 {0}",
	"patch.preinit_detected":       "preinit 分区：{0}（从运行中的模拟器读取）",
	"patch.preinit_inherited":      "preinit 分区：{0}（沿用上次补丁的配置）",
	"patch.preinit_override":       "preinit 分区：{0}（来自 --preinit）",
	"patch.preinit_missing":        "未记录 preinit 分区：Magisk 仍能启动，但模块存储可能退化。请在模拟器运行时重试，或传 --preinit",
	"patch.preinit_none":           "没有运行中的模拟器，无法自动探测 preinit 分区",
	"patch.preinit_query_failed":   "查询 preinit 分区失败：{0}",
	"patch.preinit_none_on_device": "模拟器没有合适的分区可用作 preinit；Magisk 仍能启动，但模块存储可能退回 /data",
	"patch.summary":                "摘要",
	"patch.will_save":              "将把原始镜像保存为 {0}",
	"patch.backup_kept":            "沿用已有备份：{0}",
	"patch.backup_kept_short":      "沿用已有备份",
	"patch.backup_is_patched":      "已有备份本身也是打过 Magisk 补丁的，从它还原不会回到原厂镜像",
	"patch.will_overwrite":         "将覆盖 {0}",
	"patch.plan":                   "{0} 个 cpio 条目 -> {1} 个，压缩格式 {2} -> {3}",
	"patch.dry_run_done":           "试运行：没有写入任何内容",
	"patch.preserving":             "保存原始 ramdisk",
	"patch.saved_backup":           "已保存 {0}",
	"patch.writing":                "写入打过补丁的 ramdisk",
	"patch.written":                "已写入 {0}（{1} 个 cpio 条目）",
	"patch.next_restart":           "必须重启模拟器（不是 adb reboot）才会生效：",
	"patch.next_root":              "  avdroot root      按需打补丁、重启、授予 su 并验证",
	"patch.next_or_start":          "或者自己启动：",
	"patch.next_then":              "然后运行：avdroot install && avdroot verify",

	// restore
	"restore.short":          "从 .backup 备份还原 ramdisk",
	"restore.example":        "  avdroot restore\n  avdroot restore --list",
	"restore.restoring":      "正在还原 {0}",
	"restore.from":           "来源 {0}",
	"restore.done":           "已还原，ramdisk 现在是 {0}",
	"restore.backup_kept":    "备份文件已保留；需要释放空间可自行删除",
	"restore.backup_patched": "备份 {0} 本身也是打过 Magisk 补丁的",
	"restore.keeps_patch":    "还原后补丁依然存在",
	"restore.none":           "没有找到任何 ramdisk 备份",
	"restore.list_flag":      "列出所有找到的 ramdisk 备份",

	// root
	"root.short": "一条命令完成 root，全程无需人工干预",
	"root.long": `root 会一次做完整个流程，不需要任何后续输入：

  1. 模拟器没运行就先启动它（preinit 分区只能从运行中的挂载表读取）
  2. 给 ramdisk 打补丁
  3. 重启模拟器，让新 ramdisk 生效
  4. 安装 Magisk App
  5. 给 adb shell 授予 su，无需在 Magisk App 里点任何东西
  6. 验证 su 返回 uid=0

只有一个 AVD 时不需要指定目标；有多个时必须指定一个。

有两个细节保证重启真的可靠：模拟器是以新进程重启，而不是 "adb reboot"，
因为 guest 重启会沿用模拟器启动时加载的 ramdisk；并且重启强制冷启动
（-no-snapshot），因为恢复快照会把打补丁之前的 initrd 带回来。
只想打补丁可用 "avdroot patch"，只检查运行状态可用 "avdroot verify"。
`,
	"root.example": `  avdroot root                  # 唯一的 AVD，会先确认
  avdroot root --yes            # 全自动
  avdroot root Pixel_10_Pro_XL  # 指定 AVD
  avdroot root --no-install     # 不装 Magisk App
  avdroot root --emulator-args "-no-window -gpu swiftshader_indirect"`,
	"root.target_note_named":      "（命令行指定）",
	"root.target_note_only_avd":   "（唯一的 AVD）",
	"root.target_note_only_image": "（唯一的系统镜像）",
	"root.already_running":        "已在运行：{0}",
	"root.not_running":            "未运行，正在启动",
	"root.started":                "模拟器已启动",
	"root.restarting":             "重启模拟器",
	"root.waiting_boot":           "等待模拟器启动完成（大约需要一两分钟）",
	"root.booted":                 "模拟器已启动完成",
	"root.snapshot_fastboot":      "该 AVD 开启了快速启动，且存在已保存状态（{0}）",
	"root.snapshot_present":       "存在已保存的模拟器状态（{0}）",
	"root.cold_boot":              "改用冷启动，因为快照会把打补丁之前的 ramdisk 恢复回来",
	"root.env_setup":              "准备 Magisk 运行环境",
	"root.env_ready":              "Magisk {0} 已经解压在 /data/adb 下",
	"root.env_missing":            "Magisk 的文件还没解压，正在写入 {0} 的文件",
	"root.env_other_version":      "Magisk 的文件属于另一个版本，正在替换为 {0}",
	"root.env_installed":          "已写入 {0} 个文件（Magisk {1}），赶在用到它们的那次重启之前",
	"root.env_unknown":            "无法判断 Magisk 的文件是否就位：{0}",
	"root.granting_su":            "给 adb shell 授予 su",
	"root.su_already":             "uid {0} 已授权",
	"root.su_allowed":             "已允许 uid {0}",
	"root.adbd_still_root":        "adbd 仍是 root，下面的检查会无条件通过",
	"root.adbd_unrooted":          "adbd 已恢复为普通 shell，这样下面的检查才是在验证 Magisk 的 su；需要时可用 \"adb root\" 切回",
	"root.verifying":              "验证 root",
	"root.magisk_not_running":     "Magisk 未在运行：{0}",
	"root.maybe_stale":            "模拟器可能没有用打过补丁的 ramdisk 重启",
	"root.magisk_running":         "Magisk {0} 正在运行",
	"root.version_mismatch":       "模拟器运行的是 Magisk {0}，但 ramdisk 里是 {1}",
	"root.reboot_not_enough":      "打补丁后模拟器还没重启过；\"adb reboot\" 不够",
	"root.shell_root":             "adb shell 已经以 root 运行（adb root）",
	"root.shell_root_hint":        "adbd 是 root 时无法检验 Magisk 的 su；执行 \"adb unroot\" 后才能正确检查",
	"root.dropping_adbd_root":     "先放弃 adbd 的 root，这样检查的是 Magisk 而不是模拟器自身",
	"root.su_not_run":             "su 未能执行：{0}",
	"root.su_hint":                "用 avdroot root 授权，或在 Magisk App 里点「允许」",
	"root.su_no_root":             "su 没有给出 root：{0}",
	"root.su_ok":                  "su 已获得 root",
	"root.result":                 "root: {0}",

	// verify / install
	"verify.short":       "检查运行中的模拟器是否已获得 root",
	"verify.example":     "  avdroot verify\n  adb shell su -c id",
	"install.short":      "把 Magisk App 安装到运行中的模拟器",
	"install.example":    "  avdroot install",
	"install.installing": "安装 Magisk App",
	"install.done":       "已安装 {0}（Magisk {1}）",
	"install.done_nover": "已安装 {0}",
	"install.no_package": "安装命令执行了，但列表里没有 Magisk 包",

	// magisk
	"magisk.short":         "管理与打补丁相关的 Magisk 安装包",
	"magisk.example":       "  avdroot magisk info\n  avdroot magisk fetch --list\n  avdroot magisk fetch --version v31.0",
	"magisk.info_short":    "显示会使用哪个 Magisk 安装包，以及它支持什么",
	"magisk.info_example":  "  avdroot magisk info",
	"magisk.installer":     "安装包：{0}",
	"magisk.file_info":     "大小：{0}，修改时间 {1}",
	"magisk.no_target_abi": "未选择目标；按 {0} 报告",
	"magisk.package":       "包名         {0}",
	"magisk.supports":      "支持         {0}",
	"magisk.abis":          "包含 ABI     {0}",
	"magisk.magiskinit":    "magiskinit   {0}",
	"magisk.magisk":        "magisk       {0}",
	"magisk.initld":        "init-ld      {0}",
	"magisk.stub":          "stub apk     {0}",
	"magisk.compatible":    "与 {0} 兼容",
	"magisk.newer_hint":    "下载更新的发行版：{0}",

	"magisk.fetch_short": "下载 Magisk 安装包",
	"magisk.fetch_long": `fetch 会把 Magisk 发行版下载到配置目录，"patch" 和 "root" 会自动找到它。

可以按 tag 指定任意发行版。请只从官方发行页下载：被篡改的安装包会被写进模拟器的
ramdisk 里。

如果下载失败，fetch 会打印发行页地址，并告诉你如何直接使用自己下载的文件：

  avdroot root --magisk /path/to/Magisk.apk`,
	"magisk.fetch_example": `  avdroot magisk fetch                    # 最新稳定版
  avdroot magisk fetch --list             # 看看有哪些版本
  avdroot magisk fetch --version v31.0    # 指定 tag
  avdroot magisk fetch --prerelease       # 包含预发布版`,
	"magisk.downloading":     "下载 Magisk {0}",
	"magisk.asset":           "资产 {0}",
	"magisk.api_failed":      "无法查询发行版列表（{0}）",
	"magisk.retrying":        "重试下载（第 {0}/{1} 次，等待 {2}）",
	"magisk.saved":           "已保存 {0}（{1}）",
	"magisk.verified":        "校验通过：{0}（{1}）",
	"magisk.download_failed": "下载失败：{0}",
	"magisk.manual_header":   "手动获取",
	"magisk.manual_1":        "1. 打开 {0}",
	"magisk.manual_2":        "2. 下载你需要的 Magisk-v<版本>.apk 资产",
	"magisk.manual_3":        "3. 直接传给命令即可，不用挪动文件：",
	"magisk.manual_cmd":      "     avdroot root --magisk /path/to/Magisk-v31.0.apk",
	"magisk.manual_or":       "   或者放到 {0}",
	"magisk.releases_failed": "无法列出发行版：{0}",
	"magisk.browse":          "可以到这里浏览：{0}",
	"magisk.fetch_hint":      "获取方式：avdroot magisk fetch --version <tag>",
	"magisk.using":           "使用 {0}，来自 {1}",

	"cmd.help_short":       "查看任意命令的帮助",
	"cmd.help_long":        "查看 avdroot 中任意命令的帮助。\n输入 {{.CommandPath}} help [命令路径] 可查看完整说明。",
	"cmd.completion_short": "为指定 shell 生成自动补全脚本",
	"cmd.completion_long":  "为 avdroot 生成指定 shell 的自动补全脚本。",

	// Flags
	"flag.sdk":             "Android SDK 根目录（自动识别：$ANDROID_HOME、PATH 上的 adb、或平台默认位置）",
	"flag.avd_home":        "AVD 定义所在目录（默认 $ANDROID_AVD_HOME 或 ~/.android/avd）",
	"flag.serial":          "连接了多个设备时指定 adb 序列号",
	"flag.magisk":          "Magisk 安装包路径",
	"flag.abi":             "覆盖检测到的 ABI（如 arm64-v8a）",
	"flag.compress":        "覆盖 ramdisk 压缩格式（gzip、lz4_legacy、lz4、xz）",
	"flag.preinit":         "覆盖 preinit 块设备（通常自动探测）",
	"flag.program_version": "显示版本号后退出",
	"flag.help":            "显示帮助",
	"flag.no_adb":          "不连接运行中的模拟器",
	"flag.lang":            "输出语言：zh 或 en（默认跟随系统语言）",
	"flag.yes":             "不询问确认",
	"flag.dry_run":         "只报告会改什么，不写入任何内容",
	"flag.force":           "即使是已打过补丁的 ramdisk 也重新打",
	"flag.keep_verity":     "在 fstab 中保留 dm-verity/AVB 选项（默认开启）",
	"flag.keep_encrypt":    "在 fstab 中保留强制加密选项（默认开启）",
	"flag.recovery":        "按 recovery 模式打补丁（API 28 自动启用）",
	"flag.no_install":      "跳过安装 Magisk App",
	"flag.emulator_args":   "用这些启动参数替代自动识别的参数",
	"flag.timeout":         "等待模拟器启动的超时时间（默认 6 分钟）",
	"flag.version":         "Magisk 版本 tag，如 v31.0（默认最新稳定版）",
	"flag.dest":            "保存位置",
	"flag.prerelease":      "解析最新版时把预发布版也算进去",
	"flag.list_releases":   "列出可用发行版后退出",
	"flag.list_backups":    "列出所有找到的 ramdisk 备份",

	// Confirmation and prompts
	"prompt.patch": "给这个 ramdisk 打补丁并重启模拟器？",
	"prompt.write": "写入打过补丁的 ramdisk？",

	// Errors surfaced by the CLI
	"err.no_sdk":               "没有找到 Android SDK。\n已尝试：\n{0}\n请设置 ANDROID_HOME，或传 --sdk /path/to/sdk",
	"err.sdk_not_dir":          "{0}：{1} 不是目录。\n已尝试：\n{2}",
	"err.avdhome_not_dir":      "{0}：{1} 不是目录。\n已尝试：\n{2}",
	"err.sdk_suspect":          "使用了 {0}，但其中 platform-tools、system-images、cmdline-tools、emulator、platforms、licenses 都不存在",
	"err.read_ramdisk":         "读取 ramdisk 失败：{0}",
	"err.unsupported_patch":    "{0} 是被不支持的程序打过补丁的；请先还原原厂镜像（avdroot restore）或重新下载系统镜像",
	"err.api_unsupported":      "{0}\n可从 https://github.com/topjohnwu/Magisk/releases 下载更新的发行版\n或传 --magisk /path/to/Magisk.apk",
	"err.cancelled_write":      "已取消，没有写入任何内容",
	"err.backup_failed":        "创建备份失败：{0}",
	"err.write_failed":         "写入 {0} 失败：{1}",
	"err.no_backup":            "{0} 没有备份，无法还原",
	"err.target_not_found":     "{0} 既不是 AVD 名称、系统镜像或 ramdisk 路径，也不存在这个文件",
	"err.unexpected_args":      "{0} 不接受参数，但收到了 {1} 个",
	"err.too_many_targets":     "最多只能指定一个目标，实际给了 {0} 个",
	"err.no_installer":         "未找到 {0}；已查找：\n  {1}\n可从 https://github.com/topjohnwu/Magisk/releases 下载后放到该位置，\n或传 --magisk /path/to/Magisk.apk，或运行 \"avdroot magisk fetch\"",
	"err.adb_required_root":    " \"avdroot root\" 需要 adb；请改用 \"avdroot patch\"",
	"err.not_emulator":         "当前连接的设备不是 Android 模拟器；请改用 \"avdroot patch\" 并自行重启设备",
	"err.no_emulator_named":    "没有运行中的模拟器，且无法确定 AVD 名称；请启动 {0}，或显式指定 AVD",
	"err.cancelled":            "已取消",
	"err.no_avd_name":          "无法确定要重启的 AVD 名称；请手动重启模拟器，然后运行 \"avdroot verify\"",
	"err.su_policy":            "写入 su 策略失败：{0}",
	"err.adb_required_verify":  "验证 root 需要 adb",
	"err.no_emulator":          "没有运行中的模拟器：{0}",
	"err.no_root":              "未获得 root",
	"err.adb_required_install": "安装 App 需要 adb",
	"err.install_failed":       "安装 Magisk App 失败：{1}: {0}",
	"err.no_release_asset":     "发行版 {0} 没有安装包资产",
	"err.download_failed":      "无法下载 Magisk",
	"err.archive_unusable":     "下载到的包不可用：{0}",
	"err.http_404":             "HTTP 404：没有这个 tag 的发行版",
	"err.http_other":           "HTTP {0}",
	"err.download_tiny":        "只下载到 {0}，太小了，不像是一个发行版",
	"err.api_status":           "GitHub API 返回 {0}",
	"err.no_release":           "没有找到可下载的发行版",

	// Table headers and value fragments
	"table.name":             "名称",
	"table.api":              "API",
	"table.abi":              "ABI",
	"table.status":           "状态",
	"table.backup":           "备份",
	"table.image":            "镜像",
	"table.path":             "路径",
	"table.ramdisk":          "RAMDISK",
	"table.tag":              "版本",
	"table.channel":          "通道",
	"table.published":        "发布时间",
	"table.size":             "大小",
	"flavor.api_and_release": "API {0}（Android {1}）",
	"flavor.api":             "API {0}",
	"flavor.release":         "Android {0}",
	"flavor.unknown":         "未知",

	"backup.present":            "有（{0}）",
	"backup.none":               "无",
	"src.tool":                  "PATH 上的 {0}",
	"src.default":               "{0} 的默认位置",
	"src.search":                "扫描 {0} 找到",
	"note.has_images":           "含 system-images",
	"note.no_images":            "无 system-images",
	"note.avd_count":            "{0} 个 AVD",
	"note.no_avds":              "没有 AVD 定义",
	"kind.sdk":                  "Android SDK",
	"kind.avd_home":             "AVD 目录",
	"doctor.state_unlike":       "存在，但不像{0}",
	"warn.playstore":            "该 AVD 使用 Play Store 镜像：无法使用 adb root，所以补丁可以打进去，但 Magisk App 无法通过 adb 管理它",
	"doctor.no_ramdisk":         "（没有带 ramdisk 的系统镜像）",
	"patch.next_root_cold":      "  avdroot root      从冷启动状态一条命令完成",
	"patch.snapshot_fastboot":   "该 AVD 开启了快速启动，且存在已保存状态（{0}）",
	"patch.snapshot_present":    "存在已保存的模拟器状态（{0}）",
	"patch.snapshot_hint":       "补丁需要冷启动才会生效，\"avdroot root\" 会自动这样做。过期的快照可以在 Android Studio 的设备管理器里删除",
	"err.playstore_unsupported": "{0} 使用的是 Play Store 镜像，不允许 adb root 也无法转为可写；不能用这种方式 root",
	"magisk.auto_fetching":      "未找到 Magisk 安装包，正在下载",
	"magisk.none_found":         "未找到 Magisk 安装包，且下载已被禁用",
	"err.no_installer_disabled": "没有可用的 Magisk 安装包，且指定了 --no-download；请放到 {0}，或用 --magisk 指定",
	"flag.no_download":          "不要自动下载 Magisk 安装包",
	"doctor.cache":              "安装包缓存",
	"magisk.cache_hint":         "下载的安装包缓存在 {0}，可以随时清除",
	// trust-chrome
	"trust.short": "让模拟器上的 Chrome 信任某个 CA 证书",
	"trust.long": `Chrome 在 Android 上会对「锚定到系统证书库的证书」强制校验 Certificate
Transparency。本地生成的中间人 CA 没有 CT 日志记录，所以 Chrome 会以
NET::ERR_CERT_AUTHORITY_INVALID 拒绝它，而 Android 的其他部分却完全接受。

本命令通过给 Chromium 的启动参数加入 --ignore-certificate-errors-spki-list，
让它显式信任这一把公钥。Chrome 和 WebView 都从设备上的文件读取该参数，这里会把
它们会读的每个路径和变体都写一遍。

证书来自 --cert；未指定时会自动从设备上「非系统镜像自带」的 CA 证书里识别。

导出格式：PEM 和 DER 都接受，与文件扩展名无关，所以 Reqable 的 .pem、.crt、.0 三种
导出都能用。.p12 是容器格式，不支持；请改导出 PEM。`,
	"trust.example": `  avdroot trust-chrome                            # 自动识别已安装的 CA
  avdroot trust-chrome --cert ~/reqable-root.crt  # 显式指定证书
  avdroot trust-chrome --cert /data/local/tmp/ca.crt   # 设备上的路径
  avdroot trust-chrome --clear                    # 撤销`,
	"trust.checking":            "准备让 Chrome 信任 CA",
	"trust.cert_summary":        "证书：{0}",
	"trust.cert_source":         "读取自 {0}",
	"trust.not_ca":              "该证书不是 CA，用它做中间人不会生效",
	"trust.spki":                "SPKI 指纹：{0}",
	"trust.writing":             "写入 Chromium 启动参数",
	"trust.written":             "已写入 {0} 个参数文件",
	"trust.write_failed":        "写入 {0} 失败：{1}",
	"trust.chmod_failed":        "设置 {0} 权限失败",
	"trust.restarting":          "重启 Chrome 以读取参数",
	"trust.restarted":           "Chrome 已停止；重新打开即可生效",
	"trust.restart_hint":        "需要重启 Chrome 才能读到参数",
	"trust.clearing":            "移除 Chromium 启动参数",
	"trust.cleared":             "参数文件已移除",
	"trust.caveat":              "Chrome 现在会接受由这一把密钥签发的证书，且不再校验 Certificate Transparency。",
	"trust.caveat_key":          "指纹与密钥绑定：如果中间人工具重新生成了 CA，需要重新执行。仅适用于模拟器和测试设备。",
	"err.adb_required_trust":    "信任证书需要模拟器在运行；请先启动再试",
	"err.root_required_trust":   "写入 Chromium 参数需要 root：{0}",
	"err.cert_unreadable":       "{0}：{1}",
	"err.no_candidate_ca":       "在设备上（系统镜像之外）没有找到 CA 证书。\n已查找：\n  {0}\n请先安装中间人 CA，或用 --cert /path/to/ca.crt 指定",
	"err.multiple_candidate_ca": "设备上装了多个 CA 证书，请用 --cert 指定其中一个：\n  {0}",
	"err.trust_write_failed":    "没有任何一个 Chromium 参数文件写入成功",
	"err.trust_clear_failed":    "移除参数文件失败：{0}",
	"flag.cert":                 "要信任的 CA 证书（PEM 或 DER，不限扩展名；本地路径或设备路径）",
	"flag.clear_trust":          "移除参数而不是写入",
	"flag.no_restart":           "不要重启 Chrome",

	// Values
	"status.stock":       "原厂",
	"status.magisk":      "已打 Magisk 补丁",
	"status.unsupported": "不支持的补丁",
	"bool.yes":           "是",
	"bool.no":            "否",
}
