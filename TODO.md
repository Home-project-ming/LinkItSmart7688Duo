<!-- Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan) -->

# TODO

> 專案的待辦與已知缺口,依價值排序。原本規劃的硬體覆蓋路線圖
> (GPIO/I2C → SPI/PWM → UART → I2S → ATmega 橋接 → ADC)已全部完成
> ——這裡列的是路線圖之外還掛著的事。

## ✅ 已完成:squashfs /opt 上線化(2026-07-14)

- [x] 開機自動 `losetup` + mount `/overlay/opt.sqfs`,把 `/opt` 指向唯讀
  映像——Entware 服務從此可以安全開機自啟。
- [x] 設計「改動 /opt 之後重建映像」的流程:`opkg` 裝/更新套件 → 重跑
  `provisioning/05-squashfs-opt/03_build_image.sh` → remount。日常唯讀,
  更新時才寫。
- [x] 重開機驗證自動掛載生效後,解除 dev-notes 裡「不要把 /opt 放進
  開機自啟服務」的禁令(`rc.unslung` 可以進 `/etc/rc.local` 了)。

**結果:成功,實機重開機驗證通過。** 新增 `provisioning/05-squashfs-opt/`
兩支腳本:`06_install_boot_hook.sh`(冪等安裝開機掛載鉤子到
`/etc/rc.local`,失敗不影響開機/不影響 SSH)、`07_opt_maintenance.sh`
(`unlock`/`rebuild` 兩個子命令,對應「解掛露出可寫目錄 → opkg 操作 →
重建映像+重新掛載」的維護流程)。實際對板子下 `reboot` 後確認:
`/opt` 無人值守自動掛載成 `squashfs(ro)`、寫入測試回報
`Read-only file system`、冷快取下 `grep --version`/`python3 --version`
皆正常執行,不再重現 03-diagnostics 的 SIGILL/SIGSEGV。設計與實作細節見
`docs/superpowers/specs/2026-07-14-squashfs-opt-boot-mount-design.md`
與 `docs/superpowers/plans/2026-07-14-squashfs-opt-boot-mount.md`。
dev-notes.html 的開機自啟禁令已同步解除。

## ✅ 已完成:squashfs 實驗(2026-07-12)——SD 執行檔隨機崩潰找到繞道

- [x] 把 `/opt` 打包成 squashfs 映像放 SD、loop-mount,驗證是否根絕 SIGILL/SIGSEGV。

**結果:成功,且數據乾淨利落。** 同一張 SD、同樣的檔案、同樣的
`drop_caches` 冷快取條件:ext4 直讀對照組 grep **0/10**、python3
**0/10**(完全重現歷史數據);squashfs loop-mount 實驗組 grep
**10/10**、python3 **10/10**;加碼 soak(grep/python3 各 20 次 +
`PYTHONHOME` 指進映像的真實 Python 運算 10 次)**70/70 全過**。
同時佐證 I-cache/DMA 不一致理論——squashfs 頁面由 CPU 解壓寫入,
不走 DMA 直填 mmap 頁面。

**細節**:kernel 的 squashfs 只編了 xz 解壓(gzip 映像會被拒),映像
27.4MB(原 67.5MB);需側載 `kmod-loop`+`losetup`(原廠 kernel 沒有
loop device);mksquashfs 要從 tmpfs 執行(它自己就是會冷快取崩潰的
SD 上 glibc 執行檔)。腳本與側載 ipk 都在
`provisioning/05-squashfs-opt/`。目前的 loop mount 重開機即消失——
上線化見上方新的第一名。

**根治備案**(不急):升級到 kernel ≥ 5.x 的現代 OpenWrt。

## 🎯 進行中:XCS20 FPGA slave-serial 下載(2026-07-30 設計完成)

設計文件:`docs/superpowers/specs/2026-08-08-xcs20-download-via-atmega.md`
(傳輸層;取代 2026-07-30 那份的 §5,其餘仍有效)
——經 74LS125 轉壓接目標板 DB25,slave serial,拔 EPROM 下載
(實機驗證過此模式可行,綠燈=成功)。

**軟體已完成(2026-08-08),硬體與 bring-up 未動。**

> **傳輸層換過一次。** 原訂由 7688 用 sysfs GPIO bit-bang,查證後發現
> Duo 只引出 P1/P8/P9 三支、且 P8/P9 是 console UART——湊不出四出一入。
> 改由 7688 經 Firmata bridge 把位元流餵給 ATmega32U4,由它的 D8–D12
> 打進轉接板,約 4.8 秒。`sysfsPort` 的程式碼與測試都還在,只是這塊板用不到。

- [x] 設計文件 §6.1 `fpga_configure` MCP 工具:`fpga.go`(.bit 容器解析器、
  slave-serial 時序狀態機、預開 fd 的 sysfs pin driver)+ `fpga_test.go`
  (33 個單元測試,含兩個真 `.bit` 當 testdata)。全部離線可跑,
  `go test ./...` 綠、`go vet` 乾淨、mipsle 交叉編譯過。
- [x] 設計文件 §6.3 host glue:姊妹 repo 的 `build.ps1 -Download`——
  scp `.bit` 到板子 `/tmp`,再用 SSH 上的 JSON-RPC 叫 `fpga_configure`。
  **`-Pins` 已移除**:腳位在 ATmega 韌體裡寫死,留一個永遠被忽略的必填參數
  只會害人查半天。舊命令列仍可跑,只印警告。
- [x] ATmega 韌體 `atmega-firmware/fpga-download/fpga-download.ino`:
  Firmata SysEx 0x7E,BEGIN/DATA/END/ABORT 四個子命令,整條 slave-serial
  時序在韌體裡跑。編譯過(9,528 bytes flash / 33%)。
- [x] Go 端 `fpga_bridge.go` + `_linux.go`/`_other.go` + 測試:8→7 密集打包、
  SysEx 分框、殘塊、丟包偵測、每條退出路徑都送 ABORT。
- [x] 設計文件 §6.2 bring-up 腳本 `provisioning/06-xcs20/01_manual_probe.sh`
  ——busybox-only,把 slave-serial 各信號拆成可單步下的子命令
  (`init`/`status`/`gate`/`set`/`clock`/`pulse-program`/`release`),
  讓人拿三用電表走完 §7 的第 3、4 階段。**已作廢**:它驅動的是 7688 sysfs
  GPIO,這塊板沒有那些腳。腳本頂端已加註,單步檢查改由韌體的 BEGIN 階段
  代勞(打 /PROGRAM 脈衝、DONE 沒掉就拒絕,不會白灌 22KB)。
- [ ] **實機 bring-up**:`fpga_configure` 從未對真板子跑過。
  前置:焊 74LS125 轉接板(BOM:1×LS125、1×10K、1×15K、DB25、0.1µF)、
  燒 `fpga-download.ino` 到 ATmega(燒錄時目標板要斷電)。
  順序:7688 先完全開機 → 目標板才上電。
  第一次務必先下 `-DryRun`(只解析,連 bridge 都不碰)。
  已驗證到 dry run 為止:`part s20pq208 / bits 178144`,再往後全未驗。

## 🔌 等硬體到位才能補的驗證缺口

- [ ] `i2c_write`:目前只有 code review + 單元測試——需要一顆可安全寫入的
  I2C 裝置(例如 BME280)實測。
- [ ] `uart_transact` 位元組級 round-trip:TX(pin 45)–RX(pin 46)接一條
  loopback 跳線即可測——等願意動硬體的時候順手做。
- [ ] `i2s_play` 實際聽感:只驗證過呼叫不報錯;接上喇叭確認聲音乾淨。

## 📋 Backlog(從未被需要,不欠)

- [ ] MIPS 原生 C 交叉編譯工具鏈——目前 Go 自帶的交叉編譯就夠;若有純 C
  需求再建,並回填 `CLAUDE.md` 的 TODO。
- [ ] ATmega 的 USB 有線上傳流程(`arduino-cli upload -p`)——網路 ISP
  燒錄已夠用,需要時再驗證。

## 🚫 已結案,不要再開

- **I2S 錄音**:驅動層死巷——capture 一啟動就卡死(SIGKILL 殺不掉、須
  重開機),codec 電源/路由確認過、buffer/period 加大也試過,同樣失敗。
  詳見 `docs/superpowers/specs/2026-07-11-i2s-design.md` 的 Investigation
  段落。
- **GPIO 黑名單 pin 3/38 之謎**:2026-07-12 已修正並實機驗證
  (pin 3 = I2S + ATmega ISP reset;pin 38 = Wi-Fi Reset 按鈕)。
