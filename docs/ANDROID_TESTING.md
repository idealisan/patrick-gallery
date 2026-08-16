# 本地 Android 模拟器端到端测试方案（immich-go）

目标：在本地用 **Android 模拟器**运行 **官方 immich Android APK（v3.1.0）**，
通过真实客户端驱动 `immich-go` 服务端，做真正的端到端（E2E）测试，而不是仅靠
代码审查或 headless 契约重放。

> 经验来源：本方案是在 Debian 13（trixie）x86_64、**没有 `/dev/kvm`** 的机器上
> 一步步踩坑后总结出来的。没有 KVM 意味着模拟器只能用 **软件加速**
>（`-accel off`，QEMU TCG），启动很慢（约 7 分钟），且 `system_server` 偶发不稳定。
> 下面所有「坑」都是实打实遇到过的。

---

## 0. 前置条件

- 一台 Linux 宿主机（Debian 13 验证通过），x86_64。
- 确认 **没有 KVM**：
  ```sh
  ls -la /dev/kvm   # 不存在 -> 软件加速
  ```
- 磁盘至少 10GB 空闲（SDK + 系统镜像 + AVD + APK）。
- 网络可访问 `dl.google.com` 与 `github.com`。
- 宿主机上 `immich-go` 已构建并监听 `:8081`
  （本仓库默认 `IMMICH_PORT=8081`，二进制 `./dist/immich-go`，
  视频库通过 `LD_LIBRARY_PATH=./dist/libs` 加载）。

---

## 1. 安装 Android 工具链

```sh
export ANDROID_HOME=/opt/android-sdk
export ANDROID_SDK_ROOT=/opt/android-sdk
export PATH=$ANDROID_HOME/emulator:$ANDROID_HOME/platform-tools:$PATH

# 1.1 系统依赖（apt）
apt-get update
apt-get install -y wget unzip git openjdk-17-jdk-headless python3 \
        libpulse0 libxkbfile1 libnss3 libgtk-3-0 libsdl2-2.0-0 \
        libx11-6 libxcb1 libxext6 libxrandr2 aapt

# 1.2 下载 commandline-tools（注意版本号可能变化，以官网为准）
mkdir -p $ANDROID_HOME/cmdline-tools
cd /tmp
wget -q https://dl.google.com/android/repository/commandlinetools-linux-11076708_latest.zip
unzip -q commandlinetools-linux-11076708_latest.zip -d $ANDROID_HOME/cmdline-tools
mv $ANDROID_HOME/cmdline-tools/cmdline-tools $ANDROID_HOME/cmdline-tools/latest

# 1.3 接受许可
yes | $ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager --licenses

# 1.4 安装核心包（emulator / platform-tools / 系统镜像）
$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager \
    "platform-tools" "emulator" "system-images;android-34;default;x86_64"
```

> 关键坑：`emulator` 启动需要 `libpulse.so.0` 和 `libxkbfile.so.1`，缺失时会报
> `error while loading shared libraries`。上面 `apt` 一行已包含。若仍缺库，用
> `ldd $ANDROID_HOME/emulator/qemu/linux-x86_64/qemu-system-x86_64` 找出缺哪个再装。

---

## 2. 创建 AVD

```sh
$ANDROID_HOME/cmdline-tools/latest/bin/avdmanager create avd \
    -n immich_test -k "system-images;android-34;default;x86_64" \
    -d pixel_5 -c 512M
# 校验
$ANDROID_HOME/cmdline-tools/latest/bin/avdmanager list avd
```

---

## 3. 启动模拟器（无头 + 软件加速）

```sh
export ANDROID_HOME=/opt/android-sdk
export ANDROID_SDK_ROOT=/opt/android-sdk
export PATH=$ANDROID_HOME/emulator:$ANDROID_HOME/platform-tools:$PATH
export QT_QPA_PLATFORM=offscreen

# 用 setsid+nohup 让它在后台存活，即使 shell 退出也不被杀
setsid nohup $ANDROID_HOME/emulator/emulator \
    -avd immich_test \
    -no-window \
    -accel off \
    -gpu swiftshader_indirect \
    -no-audio \
    -memory 4096 \
    -partition-size 2048 \
    > /tmp/emulator.log 2>&1 < /dev/null &
```

要点：
- `-accel off`：没有 KVM 时必须显式关闭加速，否则模拟器拒绝启动。
- `-no-window`：无头运行（CI/服务器场景）。
- `-gpu swiftshader_indirect`：软件渲染 GL（否则 GPU 初始化失败）。
- 启动很慢，等待 `sys.boot_completed=1`：
  ```sh
  for i in $(seq 1 60); do
    bc=$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')
    [ "$bc" = "1" ] && { echo "booted"; break; }
    sleep 10
  done
  ```
- 偶发弹「System UI 没有响应」ANR：直接点 **Wait**（坐标约 `540 1120`），继续等即可。

---

## 4. 安装官方 immich APK

**APK 正确下载地址（v3.1.0）：**

```
https://github.com/immich-app/immich/releases/download/v3.1.0/app-x86_64-release.apk
```

> 关键坑：不要下载 `immich-android.apk` —— 该文件名在 v3.1.0 已不存在，会 404。
> 真实产物名之一是 `app-x86_64-release.apk`（约 70MB，对 x86_64 模拟器最合适；
> 真机可用 `app-arm64-v8a-release.apk`）。可用 GitHub API 核对：
> `curl -sL https://api.github.com/repos/immich-app/immich/releases/tags/v3.1.0 | grep '"name":'`

安装步骤（软件模拟环境下 `adb install` 流式传输易断，先用 `push` 再 `pm install`）：

```sh
# 下载（宿主机）
wget -q https://github.com/immich-app/immich/releases/download/v3.1.0/app-x86_64-release.apk \
     -O /tmp/immich.apk

# 推到设备
adb push /tmp/immich.apk /data/local/tmp/immich.apk

# 从设备内安装（system_server 不稳定时多试几次）
for i in $(seq 1 8); do
  adb shell pm install -r /data/local/tmp/immich.apk && break
  sleep 10
done

# 校验
adb shell pm list packages | grep immich   # 期望 app.alextran.immich
```

> 关键坑：刚开机后 `package` / `activity` 服务会间歇性「Broken pipe (32)」或
> 「Can't find service: package」。这是软件模拟下 `system_server` 还没完全就绪，
> **不是命令写错**。反复重试直到服务稳定即可。
> 若安装状态可疑（例如 `am start` 报 Activity does not exist 但 `pm list` 又显示已装），
> 用 `adb uninstall app.alextran.immich` 后重新干净安装一次。

---

## 5. 网络：让 APK 连上宿主机上的 immich-go ⚠️ 最重要的一坑

**模拟器内没有任何 IP 网络接口**（`ip addr` 看不到 `eth0`/`wlan0`，`ip route` 为空，
`ping 10.0.2.2` 报 `Network is unreachable`）。也就是说：

- `10.0.2.2`（模拟器回环到宿主机的经典地址）**不通**。
- 真机常用的「填 `http://<宿主机局域网IP>:8081`」在这里也**无效**。

解决：用 `adb reverse` 把「模拟器本机 `127.0.0.1:8081`」转发到「宿主机 `127.0.0.1:8081`」。
`adb reverse` 走的是 adb 传输通道，不依赖模拟器 IP 网络，因此一定可用。

```sh
# 在宿主机上（immich-go 监听 :8081 的前提下）
adb reverse tcp:8081 tcp:8081
adb reverse --list     # 期望 host-xx tcp:8081 tcp:8081
```

然后在 APK 的「Server Endpoint URL」里填：

```
http://127.0.0.1:8081
```

> 注意：`adb reverse` 在模拟器重启后会失效，重启模拟器需重新执行。

---

## 6. 用「布局信息」驱动 UI（不要靠截图猜坐标）

官方 immich 是 Flutter 应用，控件是 Skia 自绘的，**不要靠截图目测坐标去 `input tap`**，
而要用 `uiautomator dump` 拿到真实控件树，按 `content-desc` / `text` / `class` 定位，
再点击其中心点。

### 6.1 抓取布局

```sh
adb shell uiautomator dump /sdcard/ui.xml
adb pull /sdcard/ui.xml /tmp/ui.xml
```

### 6.2 解析控件中心坐标

仓库提供辅助脚本 `scripts/android_uicenter.py`（纯 Python，无第三方依赖）：

```sh
# 按 content-desc / text 匹配，输出中心点 x y 及控件标签
python3 scripts/android_uicenter.py /tmp/ui.xml Next
#   -> 540 638  [Next]

python3 scripts/android_uicenter.py /tmp/ui.xml "" android.widget.EditText
#   -> 540 581  [http://127.0.0.1:8081]
```

拿到坐标后：

```sh
adb shell input tap <cx> <cy>
```

### 6.3 设置界面控件（实测）

immich APK 首屏（Server Endpoint URL）的控件树实测如下：

| 控件 | 类型 | 定位方式 | 备注 |
|------|------|----------|------|
| 服务器地址输入框 | `android.widget.EditText` | `class=android.widget.EditText` | 内容是所填 URL |
| 下一步 | `android.widget.Button` | `content-desc="Next"` | **在 Settings 上方** |
| 设置 | `android.widget.Button` | `content-desc="Settings"` | 在 Next 下方 |

典型操作流程（每一步后用 `uiautomator dump` 复核）：

1. 点输入框中心 → `input text "http://127.0.0.1:8081"`。
2. **点 `Next` 按钮（按 content-desc 定位）**，不要按键盘的 Enter 键
   （Enter 在某些输入法下会误触发系统 WebView，跳到浏览器）。
3. 进入登录页：填邮箱、密码，点登录按钮（同样按 content-desc 定位）。
4. 登录成功后进入时间线/照片页。

> 关键坑 —— 清空输入框：`adb shell input keyevent KEYCODE_DEL` 对 Flutter 输入框
> **经常无效**（光标不会删除字符，反而会把新文本追加到旧文本后面）。
> 需要改 URL 时，**不要试图删字符**，直接：
> ```sh
> adb shell am force-stop app.alextran.immich
> adb shell pm clear app.alextran.immich   # 清空偏好（含已填的 endpoint）
> adb shell am start -n app.alextran.immich/app.alextran.immich.MainActivity
> ```
> 重新进入后输入框为空，再 `input text` 一次写对。
>
> 关键坑 —— 启动 Activity 的组件名是
> `app.alextran.immich/app.alextran.immich.MainActivity`
> （用 `aapt dump badging <apk>` 可查到 `launchable-activity`）。

---

## 7. 如何确认「真·端到端」跑通

- 宿主机侧：观察 `immich-go` 日志（启动时 `-o`/重定向到文件），应能看到来自 APK 的
  真实请求，例如 `/api/server/about`、`/api/auth/login`、`/api/users/me`、
  `/api/users/me/preferences`、`/api/sync/stream`、timeline 等。
- 设备侧：用 `uiautomator dump` 确认已进入登录后页面（出现照片/时间线相关控件，
  且不再显示「Server Endpoint URL」）。
- 进一步验证：在 APK 内执行上传一张照片、查看详情、创建 stack、设置头像、设置 PIN
  等操作，每一项都在宿主机日志里对应到真实客户端请求，即为端到端通过。

---

## 8. 一键复盘清单

```sh
export ANDROID_HOME=/opt/android-sdk ANDROID_SDK_ROOT=/opt/android-sdk
export PATH=$ANDROID_HOME/emulator:$ANDROID_HOME/platform-tools:$PATH

# 1) 起模拟器（无头/软件）
setsid nohup $ANDROID_HOME/emulator/emulator -avd immich_test -no-window -accel off \
  -gpu swiftshader_indirect -no-audio -memory 4096 -partition-size 2048 \
  > /tmp/emulator.log 2>&1 < /dev/null &

# 2) 等开机
adb wait-for-device
# 轮询 sys.boot_completed=1（见第 3 节）

# 3) 装 APK（先 push 再 pm install，失败重试）
adb push /tmp/immich.apk /data/local/tmp/immich.apk
adb shell pm install -r /data/local/tmp/immich.apk

# 4) 端口转发（让 APK 通过 127.0.0.1:8081 访问宿主机 immich-go）
adb reverse tcp:8081 tcp:8081

# 5) 启动 APK 并按第 6 节用布局信息操作
adb shell am start -n app.alextran.immich/app.alextran.immich.MainActivity
```

---

## 9. 第三方产物登记（合规）

按仓库 `AGENTS.md` 的「依赖登记」硬规则，官方客户端 APK 属于运行时第三方制品，
已登记于 [`THIRD_PARTY.md`](./THIRD_PARTY.md)：

- 名称：immich official Android client
- 版本：v3.1.0
- 架构：x86_64（`app-x86_64-release.apk`，用于模拟器）/ arm64-v8a（`app-arm64-v8a-release.apk`，用于真机）
- 下载：`https://github.com/immich-app/immich/releases/download/v3.1.0/app-x86_64-release.apk`
- 许可：GPL-3.0（与 immich 服务端一致）
- 用途：本地 Android 模拟器端到端测试，验证 `immich-go` 与官方客户端的契约兼容性。
